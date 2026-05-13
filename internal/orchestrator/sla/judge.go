package sla

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/zhuzhenwu/whisperer/internal/agent"
)

// Judge 是 LLM-as-judge 的扩展接口。orchestrator 可注入实现；
// nil 表示仅运行结构化校验（与 W6 行为兼容）。
//
// 三个方法对应不同 SLA：
//   - JudgeNPCConsistency: SLA #3。本回合存在 NPC 台词时，比对 persona / 历史 / 当前台词
//   - JudgeKnowledgeProjection: SLA #7。narrative 出现"你想起 / 你认出"等知识投射句时
//   - JudgeFailureContradiction: SLA #6 的 L2 二判。L1 字符级否定窗口判定为"未被否定"
//     时调用此方法做语义复核，避免误伤复合句 / 转折 / 反讽
//
// 实现应在判定通过时返回 nil，违规时返回非 nil Violation。
type Judge interface {
	JudgeNPCConsistency(ctx context.Context, in JudgeNPCInput) (*Violation, error)
	JudgeKnowledgeProjection(ctx context.Context, in JudgeKnowledgeInput) (*Violation, error)
	JudgeFailureContradiction(ctx context.Context, in JudgeFailureInput) (*Violation, error)
}

// JudgeFailureInput 是 #6 二判的输入。
type JudgeFailureInput struct {
	Narrative string
	ToolName  string // 触发判定的失败 roll_* 工具名
}

// JudgeNPCInput 是 #3 校验的输入。
type JudgeNPCInput struct {
	NPCID       string
	Persona     string
	RecentLines []string
	CurrentLine string
}

// JudgeKnowledgeInput 是 #7 校验的输入。
type JudgeKnowledgeInput struct {
	Narrative      string
	SkillsJSON     string // 调查员当前技能 JSON
	OccupationHint string
}

// CodeNPCInconsistent / CodeKnowledgeOverreach 是 LLM-judge 专属的违规码。
const (
	CodeNPCInconsistent    Code = "npc_inconsistent"
	CodeKnowledgeOverreach Code = "knowledge_overreach"
)

// CheckWithJudge 在结构化 Check 之上叠加 LLM 语义判定。
//
// 流程：
//  1. 先跑 Check（L1 字符级 4 条结构化 + #8 EndingForced）。
//  2. 若 Judge != nil：
//     a. #6 假阳过滤：L1 把 narrative 判为 "含未被否定的成功词"，调 Judge 二判；
//     若 Judge 判定 narrative 实际表达的是失败 / 否定 / 中性，移除 L1 的违规。
//     b. #3 NPC 一致性：trace 含 npc_speak 时按 NPC 调一次。
//     c. #7 角色知识投射：narrative 含 "你想起 / 你认出" 时调一次。
//  3. 重新计算 Passed。
//
// Judge 失败（API 错误 / 解析失败）一律保守：不修改 Report，让 L1 决定。
func (v *Validator) CheckWithJudge(ctx context.Context, trace agent.TurnTrace, judge Judge, judgeCtx JudgeContext) Report {
	r := v.Check(trace)
	if judge == nil {
		return r
	}

	// 2a. #6 二判：清理 L1 假阳。
	r.Violations = filterFailureContradictionsByJudge(ctx, r.Violations, trace, judge)

	// 2b. #3 NPC 一致性
	for _, tc := range trace.ToolCalls {
		if tc.Name != "npc_speak" || tc.IsError {
			continue
		}
		var out struct {
			Dialogue string `json:"dialogue"`
			NPCID    string `json:"npc_id"`
		}
		if err := json.Unmarshal(tc.Output, &out); err != nil || out.Dialogue == "" {
			continue
		}
		input := JudgeNPCInput{
			NPCID:       out.NPCID,
			Persona:     judgeCtx.NPCPersonas[out.NPCID],
			RecentLines: judgeCtx.NPCRecentLines[out.NPCID],
			CurrentLine: out.Dialogue,
		}
		if vio, err := judge.JudgeNPCConsistency(ctx, input); err == nil && vio != nil {
			r.Violations = append(r.Violations, *vio)
		}
	}

	// 2c. #7 角色知识投射
	if narrativeMentionsAny(trace.Narrative, knowledgeProjectionKeywords) {
		if vio, err := judge.JudgeKnowledgeProjection(ctx, JudgeKnowledgeInput{
			Narrative:      trace.Narrative,
			SkillsJSON:     judgeCtx.InvestigatorSkillsJSON,
			OccupationHint: judgeCtx.InvestigatorOccupation,
		}); err == nil && vio != nil {
			r.Violations = append(r.Violations, *vio)
		}
	}

	r.Passed = len(r.Violations) == 0
	return r
}

// filterFailureContradictionsByJudge 把 L1 标记的 #6 违规交给 Judge 复核：
// Judge 明确判定为"通过"则移除该违规；其余情况（违规 / 错误）保留。
func filterFailureContradictionsByJudge(ctx context.Context, vios []Violation, trace agent.TurnTrace, judge Judge) []Violation {
	out := make([]Violation, 0, len(vios))
	for _, vio := range vios {
		if vio.Code != CodeFailureContradict {
			out = append(out, vio)
			continue
		}
		// 找触发本条 violation 的失败 roll_* tool 名（取第一个失败的即可）。
		toolName := ""
		for _, tc := range trace.ToolCalls {
			if isRollTool(tc.Name) && toolReportedFailure(tc.Output) {
				toolName = tc.Name
				break
			}
		}
		judgeRes, err := judge.JudgeFailureContradiction(ctx, JudgeFailureInput{
			Narrative: trace.Narrative,
			ToolName:  toolName,
		})
		if err != nil {
			// Judge 失败 → 保守保留 L1 判定
			out = append(out, vio)
			continue
		}
		if judgeRes != nil {
			// Judge 也判违规：用 Judge 的 message（更精准）
			out = append(out, *judgeRes)
		}
		// judgeRes == nil 且 err == nil → Judge 明确判通过，丢弃 L1 假阳
	}
	return out
}

// JudgeContext 把 orchestrator 抓的快照按需传给 Judge。
type JudgeContext struct {
	NPCPersonas            map[string]string   // npc_id → persona text
	NPCRecentLines         map[string][]string // npc_id → recent dialogues
	InvestigatorSkillsJSON string
	InvestigatorOccupation string
}

var knowledgeProjectionKeywords = []string{
	"你想起", "你认出", "你回忆起", "你意识到", "你忽然明白",
}

// HaikuJudge 是基于 anthropic Haiku 的轻量 Judge 实现。
//
// 单次回合最多两次额外 LLM 调用（NPC + Knowledge 各一）；显式启用方可承担成本。
type HaikuJudge struct {
	llm   agent.LLM
	model anthropic.Model
}

// NewHaikuJudge 构造 HaikuJudge。model 留空时默认 Haiku 4.5。
func NewHaikuJudge(llm agent.LLM, model anthropic.Model) *HaikuJudge {
	if model == "" {
		model = agent.ModelHelper
	}
	return &HaikuJudge{llm: llm, model: model}
}

const judgeRubric = `你是 CoC 7e 跑团的合规审查员。仅判断给定输出是否违反规则，输出严格 JSON：
{"violation": true|false, "reason": "..."}。不要输出任何额外文本。`

func (j *HaikuJudge) JudgeNPCConsistency(ctx context.Context, in JudgeNPCInput) (*Violation, error) {
	prompt := strings.Join([]string{
		"NPC persona:", in.Persona,
		"Recent lines:", strings.Join(in.RecentLines, "\n"),
		"Current line:", in.CurrentLine,
		"任务：判断 'Current line' 是否与 persona 与历史口吻明显不一致（如换人称 / 换语气 / 知识超出 persona）。",
	}, "\n")
	violated, reason, err := j.askJudge(ctx, prompt)
	if err != nil || !violated {
		return nil, err
	}
	return &Violation{
		Code:       CodeNPCInconsistent,
		Message:    "NPC " + in.NPCID + " 当前台词与历史口吻不一致：" + reason,
		Suggestion: "重写 NPC 台词以符合既有 persona 与口吻",
	}, nil
}

func (j *HaikuJudge) JudgeFailureContradiction(ctx context.Context, in JudgeFailureInput) (*Violation, error) {
	prompt := strings.Join([]string{
		"Failed tool:", in.ToolName,
		"Narrative:", in.Narrative,
		"任务：narrative 是否实际表达了 'tool 检定失败' 的语义？",
		"  - 表达失败 / 否定 / 中性 → violation=false",
		"  - 表达成功（包括反讽 / 表面失败但实际成功） → violation=true",
		"中文复合句、转折（虽然...但...）、'未/没/不/无' 等否定要正确识别。",
	}, "\n")
	violated, reason, err := j.askJudge(ctx, prompt)
	if err != nil || !violated {
		return nil, err
	}
	return &Violation{
		Code:       CodeFailureContradict,
		Message:    "失败检定 " + in.ToolName + " 的 narrative 实际表达成功语义：" + reason,
		Suggestion: "改写为体现失败的结果",
	}, nil
}

func (j *HaikuJudge) JudgeKnowledgeProjection(ctx context.Context, in JudgeKnowledgeInput) (*Violation, error) {
	prompt := strings.Join([]string{
		"Investigator skills (JSON):", in.SkillsJSON,
		"Occupation:", in.OccupationHint,
		"Narrative:", in.Narrative,
		"任务：narrative 中如果出现 '你想起 / 你认出 / 你回忆起 / 你意识到' 等知识投射句，判断该认知是否能被 skills 支撑。能 → violation=false；不能 → violation=true。",
	}, "\n")
	violated, reason, err := j.askJudge(ctx, prompt)
	if err != nil || !violated {
		return nil, err
	}
	return &Violation{
		Code:       CodeKnowledgeOverreach,
		Message:    "调查员知识投射超出技能支撑：" + reason,
		Suggestion: "改写为符合调查员能力的暗示性描述，或直接删除该投射句",
	}, nil
}

func (j *HaikuJudge) askJudge(ctx context.Context, userPrompt string) (bool, string, error) {
	msg, err := j.llm.NewMessage(ctx, anthropic.MessageNewParams{
		Model:     j.model,
		MaxTokens: 256,
		System:    []anthropic.TextBlockParam{{Text: judgeRubric}},
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(userPrompt))},
	})
	if err != nil {
		return false, "", err
	}
	var text strings.Builder
	for _, b := range msg.Content {
		if t, ok := b.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(t.Text)
		}
	}
	var parsed struct {
		Violation bool   `json:"violation"`
		Reason    string `json:"reason"`
	}
	raw := strings.TrimSpace(text.String())
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		// LLM 输出不规范 → 视为通过，避免误伤；记录在 reason 中以便上层日志。
		//nolint:nilerr // fail-open by design: judge unparseable should not block the turn
		return false, "judge output unparseable", nil
	}
	return parsed.Violation, parsed.Reason, nil
}
