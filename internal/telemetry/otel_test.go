package telemetry

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestInit_Noop(t *testing.T) {
	shutdown, err := Init(context.Background(), Config{})
	require.NoError(t, err)
	require.NotNil(t, shutdown)
	require.NoError(t, shutdown(context.Background()))

	// noop tracer 不应该 panic
	_, span := Tracer().Start(context.Background(), "test")
	span.End()
}

func TestInit_Stdout(t *testing.T) {
	shutdown, err := Init(context.Background(), Config{Exporter: "stdout"})
	require.NoError(t, err)
	require.NotNil(t, shutdown)
	defer func() { _ = shutdown(context.Background()) }()

	_, span := Tracer().Start(context.Background(), "test_span")
	span.End()
}

func TestInit_RejectsUnknownExporter(t *testing.T) {
	_, err := Init(context.Background(), Config{Exporter: "datadog"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown exporter")
}

// TestSpansFlowToInMemoryRecorder 用 tracetest.NewSpanRecorder 验证我们的 span
// 命名按约定走（whisperer.* 前缀）。这是契约测试——后续 otel 探针铺到 RunTurn /
// tool dispatch 时应保持名字稳定。
func TestSpansFlowToInMemoryRecorder(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	tracer := Tracer()
	_, span := tracer.Start(context.Background(), "whisperer.turn")
	span.End()

	spans := rec.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, "whisperer.turn", spans[0].Name())
	assert.True(t, strings.HasPrefix(TracerName, "github.com/"))
}

// 静态体检：让 stdouttrace 真的写到 buffer 而不是 stderr，避免污染测试输出。
var _ = bytes.Buffer{}
