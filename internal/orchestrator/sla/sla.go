// Package sla implements the structural Service-Level-Assertion validator for
// each GM turn. Four SLAs are checked here (#1 #4 #6 #8 from spec §11);
// semantic SLAs (#3 NPC 一致性 / #7 角色知识闭环) are deferred to LLM-as-judge
// in a later milestone.
package sla

import (
	"encoding/json"
	"strings"

	"github.com/zhuzhenwu/whisperer/internal/agent"
)

// 关键词集合：检定结论 / 成功语义 / 失败语义。
//
// 这些都是中文文本，在 narrative 中常出现的成功/失败结论性短语。MVP 走最简
// 关键词扫描；之后可以接 jieba/分句器 + 否定窗口检测来减少误报。
var (
	checkConclusionKeywords = []string{
		"成功", "失败", "击中", "击退", "答应", "被说服",
		"通过检定", "未通过", "顺利完成", "失手",
	}
	successSemanticKeywords = []string{
		"成功", "答应", "信任你", "击中", "顺利", "通过检定",
	}
)

// Code 是违规码，便于上层做匹配 / 多语化。
type Code string

const (
	CodeRollMissing       Code = "roll_missing"
	CodeDestroyedRevived  Code = "destroyed_revived"
	CodeFailureContradict Code = "failure_contradicted"
	CodeEndingNotForced   Code = "ending_not_forced"
)

// Violation 是 SLA 违规条目。
type Violation struct {
	Code       Code   `json:"code"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

// Report 是一次 Check 的汇总。
type Report struct {
	Violations   []Violation  `json:"violations,omitempty"`
	JudgeChecks  []JudgeCheck `json:"judge_checks,omitempty"`
	Passed       bool         `json:"passed"`
	EndingForced bool         `json:"ending_forced,omitempty"`
}

// JudgeCheck 是 LLM-as-judge 的可观测记录。结构化 SLA 仍由 Violations 决定是否
// 阻塞；JudgeCheck 让 trace/replay 能解释语义审查是否运行、判定了什么。
type JudgeCheck struct {
	Kind    string `json:"kind"`
	Target  string `json:"target,omitempty"`
	Passed  bool   `json:"passed"`
	Message string `json:"message,omitempty"`
}

// Snapshot 是 Validator 在 Check 时需要的上下文：销毁物品名（#4）、调查员状态（#8）。
type Snapshot struct {
	DestroyedItemNames []string
	InvestigatorActive bool
	InvestigatorHP     int
	InvestigatorSAN    int
}

// Validator 持有 Snapshot 并对每个 turn trace 做 4 条结构化检查。
type Validator struct {
	snap Snapshot
}

// New 构造 Validator。
func New(snap Snapshot) *Validator {
	return &Validator{snap: snap}
}

// Check 跑全部 4 条 SLA，返回 Report。
//
// 顺序（不影响结果，仅为可读性）：#1 → #4 → #6 → #8。
func (v *Validator) Check(trace agent.TurnTrace) Report {
	r := Report{}
	if vio := v.checkRollWhenConclusion(trace); vio != nil {
		r.Violations = append(r.Violations, *vio)
	}
	if vio := v.checkItemConservation(trace); vio != nil {
		r.Violations = append(r.Violations, *vio)
	}
	if vio := v.checkFailureContradiction(trace); vio != nil {
		r.Violations = append(r.Violations, *vio)
	}
	if v.shouldForceEnding() {
		r.EndingForced = true
	}
	r.Passed = len(r.Violations) == 0
	return r
}

// #1: 检定结论必须有 roll_* tool 调用支撑。
func (v *Validator) checkRollWhenConclusion(t agent.TurnTrace) *Violation {
	if !narrativeMentionsAny(t.Narrative, checkConclusionKeywords) {
		return nil
	}
	if hasRollTool(t.ToolCalls) {
		return nil
	}
	return &Violation{
		Code:       CodeRollMissing,
		Message:    "narrative 中出现检定结论关键词，但本回合未调用任何 roll_* 工具",
		Suggestion: "把检定动作改用 roll_skill / sanity_check / opposed_roll 等工具裁决，再叙述结果",
	}
}

// #4: 销毁过的物品名不得在 narrative 中以"现身"语义重新出现。
func (v *Validator) checkItemConservation(t agent.TurnTrace) *Violation {
	if len(v.snap.DestroyedItemNames) == 0 {
		return nil
	}
	for _, name := range v.snap.DestroyedItemNames {
		if name == "" {
			continue
		}
		if strings.Contains(t.Narrative, name) {
			return &Violation{
				Code:       CodeDestroyedRevived,
				Message:    "narrative 提到了已销毁的物品: " + name,
				Suggestion: "已销毁物品不应再出现在描述中；改用替代物或承认其不复存在",
			}
		}
	}
	return nil
}

// #6: 失败检定不得在 narrative 中以成功语义结尾。
//
// 关键："未成功 / 没击中 / 不答应 / 无法说服" 等否定形式 **不应** 算违规。否则
// GM 在 retry 反馈时被迫"绕开"成功词，但中文语境里 "未/没/不/无法 + 成功词"
// 才是表达失败的最自然方式（W7 真实 e2e 暴露的误伤问题）。
func (v *Validator) checkFailureContradiction(t agent.TurnTrace) *Violation {
	for _, tc := range t.ToolCalls {
		if !isRollTool(tc.Name) {
			continue
		}
		if !toolReportedFailure(tc.Output) {
			continue
		}
		if hasUnnegatedSuccessKeyword(t.Narrative, successSemanticKeywords) {
			return &Violation{
				Code:       CodeFailureContradict,
				Message:    "失败检定 " + tc.Name + " 后 narrative 出现未被否定的成功语义关键词",
				Suggestion: "把叙事改写为体现失败的结果（未能说服 / 没击中 / 通过等）；明确否定可以保留",
			}
		}
	}
	return nil
}

// #8: 投查员死亡 / SAN=0 → 必须强制路由到结局页。该方法仅返回 EndingForced 信号；
// orchestrator 在收到 Report.EndingForced=true 时切到 Ending 路由。
func (v *Validator) shouldForceEnding() bool {
	if !v.snap.InvestigatorActive {
		return true
	}
	if v.snap.InvestigatorHP <= 0 {
		return true
	}
	if v.snap.InvestigatorSAN <= 0 {
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func narrativeMentionsAny(n string, words []string) bool {
	if n == "" {
		return false
	}
	for _, w := range words {
		if strings.Contains(n, w) {
			return true
		}
	}
	return false
}

// 否定词字符。中文里这些字符出现在成功词紧前 ≤ negationWindow 个 rune 内，
// 我们视为该成功词被否定。
//
// 词表保守一点没事——L1 阶段宁可多放过几个边角，由 LLM-judge（启用时）兜底。
var negationRunes = map[rune]bool{
	'未': true, '没': true, '不': true, '无': true, '别': true, '別': true,
}

// negationWindow：成功词起点前 N 个 rune 内有否定字符即视为否定。
// 6 兼容 "她始终没有答应" 这种带连接词的句式。
const negationWindow = 6

// hasUnnegatedSuccessKeyword 是 SLA #6 的 L1 启发式判定。
//
// 算法：把 narrative 转成 []rune；对每个成功词，按 rune 滑窗找匹配；任一匹配
// 在前 negationWindow 个 rune 内未发现否定字 → 视为未被否定（违规）。
//
// 故意只覆盖最常见的"否定 + 成功词"模式。复合句（"虽然没成功，但你成功了"）
// 与反讽 / 隐喻交给 LLM-judge：见 Validator.CheckWithJudge。
func hasUnnegatedSuccessKeyword(n string, words []string) bool {
	if n == "" {
		return false
	}
	runes := []rune(n)
	for _, w := range words {
		wr := []rune(w)
		for i := 0; i+len(wr) <= len(runes); i++ {
			if !runeSliceEq(runes[i:i+len(wr)], wr) {
				continue
			}
			from := max(0, i-negationWindow)
			negated := false
			for j := from; j < i; j++ {
				if negationRunes[runes[j]] {
					negated = true
					break
				}
			}
			if !negated {
				return true
			}
		}
	}
	return false
}

func runeSliceEq(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func isRollTool(name string) bool {
	switch name {
	case "roll_skill", "roll_damage", "sanity_check", "opposed_roll":
		return true
	}
	return false
}

func hasRollTool(calls []agent.ToolCall) bool {
	for _, c := range calls {
		if isRollTool(c.Name) {
			return true
		}
	}
	return false
}

// toolReportedFailure 解析 roll_skill / sanity_check / opposed_roll 的输出，
// 判定是否为"失败"。MVP：寻找 `"success":false`（roll_skill / sanity_check）或
// winner=="target"（opposed_roll，actor 视角下为失败）。
func toolReportedFailure(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	// roll_damage 没有 success 字段；不视为失败检定。
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return false
	}
	if v, ok := generic["success"].(bool); ok {
		return !v
	}
	if w, ok := generic["winner"].(string); ok && w == "target" {
		return true
	}
	return false
}
