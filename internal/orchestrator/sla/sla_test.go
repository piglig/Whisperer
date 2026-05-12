package sla

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/zhuzhenwu/whisperer/internal/agent"
)

func toolCall(name, output string, success *bool, winner string) agent.ToolCall {
	out := map[string]any{}
	_ = json.Unmarshal([]byte(output), &out)
	if success != nil {
		out["success"] = *success
	}
	if winner != "" {
		out["winner"] = winner
	}
	b, _ := json.Marshal(out)
	return agent.ToolCall{Name: name, Output: b}
}

func TestSLA1_RollMissing(t *testing.T) {
	v := New(Snapshot{InvestigatorActive: true, InvestigatorHP: 10, InvestigatorSAN: 60})
	r := v.Check(agent.TurnTrace{
		Narrative: "你成功推开了门，里面没人。",
	})
	assert.False(t, r.Passed)
	require := assert.New(t)
	require.Len(r.Violations, 1)
	require.Equal(CodeRollMissing, r.Violations[0].Code)
}

func TestSLA1_RollPresent_Passes(t *testing.T) {
	pass := true
	v := New(Snapshot{InvestigatorActive: true, InvestigatorHP: 10, InvestigatorSAN: 60})
	r := v.Check(agent.TurnTrace{
		Narrative: "你成功扭开了锁。",
		ToolCalls: []agent.ToolCall{toolCall("roll_skill", `{}`, &pass, "")},
	})
	assert.True(t, r.Passed)
}

func TestSLA1_NoConclusion_Passes(t *testing.T) {
	v := New(Snapshot{InvestigatorActive: true, InvestigatorHP: 10, InvestigatorSAN: 60})
	r := v.Check(agent.TurnTrace{
		Narrative: "雾气更浓了，远处灯光闪烁。",
	})
	assert.True(t, r.Passed)
}

func TestSLA4_ItemRevival(t *testing.T) {
	v := New(Snapshot{
		InvestigatorActive: true, InvestigatorHP: 10, InvestigatorSAN: 60,
		DestroyedItemNames: []string{"黄铜油灯"},
	})
	r := v.Check(agent.TurnTrace{
		Narrative: "你打开背包，找到一盏黄铜油灯。",
	})
	assert.False(t, r.Passed)
	assert.Equal(t, CodeDestroyedRevived, r.Violations[0].Code)
}

func TestSLA4_NoDestroyed_Passes(t *testing.T) {
	v := New(Snapshot{
		InvestigatorActive: true, InvestigatorHP: 10, InvestigatorSAN: 60,
		DestroyedItemNames: []string{"黄铜油灯"},
	})
	r := v.Check(agent.TurnTrace{Narrative: "你环顾房间，没有发现可用之物。"})
	assert.True(t, r.Passed)
}

func TestSLA4_EmptyName_Skipped(t *testing.T) {
	v := New(Snapshot{
		InvestigatorActive: true, InvestigatorHP: 10, InvestigatorSAN: 60,
		DestroyedItemNames: []string{""},
	})
	r := v.Check(agent.TurnTrace{Narrative: "雾气更浓了。"})
	assert.True(t, r.Passed)
}

func TestSLA6_FailureContradiction(t *testing.T) {
	fail := false
	v := New(Snapshot{InvestigatorActive: true, InvestigatorHP: 10, InvestigatorSAN: 60})
	r := v.Check(agent.TurnTrace{
		Narrative: "你成功说服了她。",
		ToolCalls: []agent.ToolCall{toolCall("roll_skill", `{}`, &fail, "")},
	})
	assert.False(t, r.Passed)
	codes := []Code{}
	for _, x := range r.Violations {
		codes = append(codes, x.Code)
	}
	assert.Contains(t, codes, CodeFailureContradict)
}

func TestSLA6_OpposedTargetWins(t *testing.T) {
	v := New(Snapshot{InvestigatorActive: true, InvestigatorHP: 10, InvestigatorSAN: 60})
	r := v.Check(agent.TurnTrace{
		Narrative: "你成功制服了对手。",
		ToolCalls: []agent.ToolCall{toolCall("opposed_roll", `{}`, nil, "target")},
	})
	codes := []Code{}
	for _, x := range r.Violations {
		codes = append(codes, x.Code)
	}
	assert.Contains(t, codes, CodeFailureContradict)
}

func TestSLA6_DamageRollNoSuccessField_Ignored(t *testing.T) {
	v := New(Snapshot{InvestigatorActive: true, InvestigatorHP: 10, InvestigatorSAN: 60})
	r := v.Check(agent.TurnTrace{
		Narrative: "雾涌过来。",
		ToolCalls: []agent.ToolCall{{Name: "roll_damage", Output: json.RawMessage(`{"total":3}`)}},
	})
	assert.True(t, r.Passed)
}

func TestSLA8_EndingForced_OnInactive(t *testing.T) {
	v := New(Snapshot{InvestigatorActive: false})
	r := v.Check(agent.TurnTrace{Narrative: "雾散。"})
	assert.True(t, r.EndingForced)
}

func TestSLA8_EndingForced_OnZeroHP(t *testing.T) {
	v := New(Snapshot{InvestigatorActive: true, InvestigatorHP: 0, InvestigatorSAN: 60})
	r := v.Check(agent.TurnTrace{Narrative: "雾散。"})
	assert.True(t, r.EndingForced)
}

func TestSLA8_EndingForced_OnZeroSAN(t *testing.T) {
	v := New(Snapshot{InvestigatorActive: true, InvestigatorHP: 10, InvestigatorSAN: 0})
	r := v.Check(agent.TurnTrace{Narrative: "雾散。"})
	assert.True(t, r.EndingForced)
}

func TestSLA8_NotForced_WhenAlive(t *testing.T) {
	v := New(Snapshot{InvestigatorActive: true, InvestigatorHP: 10, InvestigatorSAN: 50})
	r := v.Check(agent.TurnTrace{Narrative: "雾散。"})
	assert.False(t, r.EndingForced)
}

// 否定窗口（W7 polish）：失败检定后 narrative 用否定形式表达失败不应被误判。

func TestSLA6_NegatedSuccessNotViolation(t *testing.T) {
	fail := false
	v := New(Snapshot{InvestigatorActive: true, InvestigatorHP: 10, InvestigatorSAN: 60})
	cases := map[string]string{
		"未能成功":      "你拼尽全力，但未能成功打开锁。",
		"没能成功":      "她转身就走，你没能成功拦下她。",
		"没击中":       "你瞄准对方下盘，没击中。",
		"不会答应":      "她明显不会答应你的请求。",
		"无法说服":      "你试了半天，但无法说服他开口。",
		"未答应":       "她未答应任何要求。",
		"远点的不":      "你不会成功的。", // "不" 距 "成功" 2 rune
		"始终不答应":     "他始终不答应你的请求。",
		"没有答应":      "她没有答应你。",
	}
	for name, narrative := range cases {
		t.Run(name, func(t *testing.T) {
			r := v.Check(agent.TurnTrace{
				Narrative: narrative,
				ToolCalls: []agent.ToolCall{toolCall("roll_skill", `{}`, &fail, "")},
			})
			for _, vio := range r.Violations {
				assert.NotEqual(t, CodeFailureContradict, vio.Code,
					"否定形式 %q 不应触发 #6: %s", name, vio.Message)
			}
		})
	}
}

func TestSLA6_UnnegatedSuccessIsViolation(t *testing.T) {
	fail := false
	v := New(Snapshot{InvestigatorActive: true, InvestigatorHP: 10, InvestigatorSAN: 60})
	// 真正违规：句中含未被否定的成功语义
	cases := map[string]string{
		"成功":         "你成功扭开了锁。",
		"答应":         "她答应了。",
		"信任你":        "她现在信任你了。",
		"先否定后肯定":     "她先没答应，但很快就答应了。", // 第二次"答应"未被否定 → 违规
	}
	for name, narrative := range cases {
		t.Run(name, func(t *testing.T) {
			r := v.Check(agent.TurnTrace{
				Narrative: narrative,
				ToolCalls: []agent.ToolCall{toolCall("roll_skill", `{}`, &fail, "")},
			})
			codes := []Code{}
			for _, x := range r.Violations {
				codes = append(codes, x.Code)
			}
			assert.Contains(t, codes, CodeFailureContradict,
				"%q 应当触发 #6 (narrative=%s)", name, narrative)
		})
	}
}

func TestSLA6_NegationOutsideWindowStillViolates(t *testing.T) {
	// "不" 距 "成功" 太远（> 6 rune），不算否定。
	fail := false
	v := New(Snapshot{InvestigatorActive: true, InvestigatorHP: 10, InvestigatorSAN: 60})
	r := v.Check(agent.TurnTrace{
		Narrative: "她不喜欢这种喧闹的酒馆，但你成功扭开了她身后那扇门。",
		ToolCalls: []agent.ToolCall{toolCall("roll_skill", `{}`, &fail, "")},
	})
	codes := []Code{}
	for _, x := range r.Violations {
		codes = append(codes, x.Code)
	}
	assert.Contains(t, codes, CodeFailureContradict)
}

func TestSLA_BadOutputJSON_NotFailure(t *testing.T) {
	// roll_skill 返回不可解析 JSON → 不视为失败 → 不触发 SLA #6
	v := New(Snapshot{InvestigatorActive: true, InvestigatorHP: 10, InvestigatorSAN: 60})
	r := v.Check(agent.TurnTrace{
		Narrative: "你成功扭开了锁。",
		ToolCalls: []agent.ToolCall{{Name: "roll_skill", Output: json.RawMessage(`{garbage`)}},
	})
	for _, x := range r.Violations {
		assert.NotEqual(t, CodeFailureContradict, x.Code)
	}
}
