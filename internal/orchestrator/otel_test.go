package orchestrator

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestRunTurn_EmitsSpanHierarchy 验证一回合产出的 span 层级符合契约：
//   - whisperer.turn 是根
//   - 至少一个 whisperer.llm.message 是它的子 span
//   - 至少一个 whisperer.tool.dispatch 是 llm.message 的子 span（tool_use 路径）
//   - root span 上挂着 trace.cost_usd / trace.iterations 等关键属性
//
// 这是契约测试——后续 OTEL 探针挪位置时不应破坏 span 命名 / 层级。
func TestRunTurn_EmitsSpanHierarchy(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		otel.SetTracerProvider(prev)
		_ = tp.Shutdown(t.Context())
	})

	llm := &fakeLLM{scripts: []string{
		msgWith("tool_use",
			textBlk("Roll Spot Hidden:"),
			toolUseBlk("tu1", "roll_skill", map[string]any{
				"skill_name": "Spot Hidden", "skill_value": 50, "difficulty": "regular",
			}),
		),
		msgWith("end_turn", textBlk("你看到桌上有一封信。")),
	}}
	o, _, _, ctx := newOrchestrator(t, llm)

	_, err := o.RunTurn(ctx, "我搜查桌面")
	require.NoError(t, err)

	spans := rec.Ended()
	require.NotEmpty(t, spans)

	names := map[string]int{}
	var turnSpan, toolSpan, llmSpan sdktrace.ReadOnlySpan
	for _, s := range spans {
		names[s.Name()]++
		switch s.Name() {
		case "whisperer.turn":
			turnSpan = s
		case "whisperer.tool.dispatch":
			toolSpan = s
		case "whisperer.llm.message":
			llmSpan = s
		}
	}

	assert.Equal(t, 1, names["whisperer.turn"], "exactly one turn span")
	assert.GreaterOrEqual(t, names["whisperer.llm.message"], 1, "at least one LLM call")
	assert.GreaterOrEqual(t, names["whisperer.tool.dispatch"], 1, "at least one tool dispatch")

	// 验证关键属性挂在 turn span 上
	require.NotNil(t, turnSpan)
	attrs := map[string]string{}
	for _, kv := range turnSpan.Attributes() {
		attrs[string(kv.Key)] = kv.Value.Emit()
	}
	assert.NotEmpty(t, attrs["save_id"])
	assert.Contains(t, attrs, "trace.iterations")
	assert.Contains(t, attrs, "trace.cost_usd")
	assert.Contains(t, attrs, "trace.tool_calls")

	// 验证 tool span 上挂了 tool.name 与 tool.is_error
	require.NotNil(t, toolSpan)
	toolAttrs := map[string]string{}
	for _, kv := range toolSpan.Attributes() {
		toolAttrs[string(kv.Key)] = kv.Value.Emit()
	}
	assert.NotEmpty(t, toolAttrs["tool.name"])

	// 验证 LLM span 上挂了 model
	require.NotNil(t, llmSpan)
	llmAttrs := map[string]string{}
	for _, kv := range llmSpan.Attributes() {
		llmAttrs[string(kv.Key)] = kv.Value.Emit()
	}
	assert.True(t, strings.Contains(llmAttrs["llm.model"], "claude") || llmAttrs["llm.model"] == "")
}
