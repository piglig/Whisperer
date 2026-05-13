// Package telemetry 给 Whisperer 接通 OpenTelemetry 追踪。
//
// 三种模式（按 Init 时的 Exporter 字段）：
//   - "" / "noop"   —— 无 exporter，trace 黑洞掉。生产 / 单测默认即此。
//   - "stdout"      —— 把 span 以 JSON 方式打到 stderr，本地开发 debug 用。
//   - "otlp"        —— 走 OTLP/HTTP 推到 OTEL Collector / Jaeger / Tempo 等。
//     端点由标准环境变量 OTEL_EXPORTER_OTLP_ENDPOINT 控制
//     （SDK 默认值，无需我方读取）。
//
// span 命名约定（语义层级，便于聚合）：
//   - whisperer.turn               —— RunTurn 整体
//   - whisperer.llm.message        —— 单次 anthropic.Messages.New
//   - whisperer.tool.dispatch      —— 单次 tool 调用
//   - whisperer.sla.check          —— SLA 校验
//   - whisperer.scenario.evaluate  —— 触发器/结局求值
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// Tracer 是 Whisperer 用到的所有包共用的 trace.Tracer。模块在 init 后调用
// otel.Tracer("whisperer/<pkg>") 即可获取。
const TracerName = "github.com/zhuzhenwu/whisperer"

// Config 配置 OTEL 初始化。零值 → noop。
type Config struct {
	// Exporter 选择导出器：noop（默认） / stdout / otlp
	Exporter string
	// ServiceVersion 出现在 resource attributes（"whisperer 0.4.x"）。可空。
	ServiceVersion string
}

// Init 按 cfg 装配 OTEL TracerProvider。返回的 shutdown 函数应在程序退出前调用，
// 让 batch 导出器把缓冲 span 刷出去。
//
// 任何 exporter 创建失败都返回错误；调用方决定是否降级到 noop。
func Init(ctx context.Context, cfg Config) (shutdown func(context.Context) error, err error) {
	mode := strings.ToLower(strings.TrimSpace(cfg.Exporter))

	if mode == "" || mode == "noop" {
		// 显式装上 noop tracer provider，让没显式注入 tracer 的代码不会崩。
		otel.SetTracerProvider(noop.NewTracerProvider())
		return func(context.Context) error { return nil }, nil
	}

	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithProcess(),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(
			semconv.ServiceName("whisperer"),
			semconv.ServiceVersion(orDefault(cfg.ServiceVersion, "dev")),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("telemetry: build resource: %w", err)
	}

	var exporter sdktrace.SpanExporter
	switch mode {
	case "stdout":
		exporter, err = stdouttrace.New(
			stdouttrace.WithWriter(os.Stderr),
			stdouttrace.WithPrettyPrint(),
		)
		if err != nil {
			return nil, fmt.Errorf("telemetry: stdout exporter: %w", err)
		}
	case "otlp":
		exporter, err = otlptracehttp.New(ctx) // 端点走标准环境变量
		if err != nil {
			return nil, fmt.Errorf("telemetry: otlp exporter: %w", err)
		}
	default:
		return nil, fmt.Errorf("telemetry: unknown exporter %q (want noop|stdout|otlp)", cfg.Exporter)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	return func(ctx context.Context) error {
		// Shutdown 已经包含 ForceFlush；不重复调以免错误掩盖。
		shutdownErr := tp.Shutdown(ctx)
		if shutdownErr != nil && !errors.Is(shutdownErr, context.Canceled) {
			return fmt.Errorf("telemetry: shutdown: %w", shutdownErr)
		}
		return nil
	}, nil
}

// Tracer 是 otel.Tracer(TracerName) 的薄封装，让调用方少 import 一行。
func Tracer() trace.Tracer {
	return otel.Tracer(TracerName)
}

func orDefault(v, fallback string) string {
	if v != "" {
		return v
	}
	return fallback
}
