package middleware

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/miaosha/utils"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

var tracer trace.Tracer

// InitTracer 初始化 OpenTelemetry + Jaeger 链路追踪
// 使用 OTLP HTTP 协议上报到 Jaeger Collector（默认端口 4318）
func InitTracer(serviceName, endpoint string) (*sdktrace.TracerProvider, error) {
	if endpoint == "" {
		endpoint = "localhost:4318"
	}

	ctx := context.Background()

	// 创建 OTLP HTTP 导出器
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("创建OTLP导出器失败: %w", err)
	}

	// 创建资源信息
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion("1.0.0"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("创建资源信息失败: %w", err)
	}

	// 创建 TracerProvider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	tracer = tp.Tracer(serviceName)
	return tp, nil
}

// TracingMiddleware 全链路追踪中间件
// 自动为每个 HTTP 请求创建 Span，记录请求路径、方法、状态码、耗时
// 将 TraceID 注入到 Gin Context 和响应头中，方便关联日志
func TracingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if tracer == nil {
			c.Next()
			return
		}

		// 从请求头提取上游 Trace Context
		ctx := otel.GetTextMapPropagator().Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))

		// 创建 Span
		spanName := fmt.Sprintf("%s %s", c.Request.Method, c.FullPath())
		ctx, span := tracer.Start(ctx, spanName, trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()

		start := time.Now()

		// 设置 Span 属性
		span.SetAttributes(
			attribute.String("http.method", c.Request.Method),
			attribute.String("http.url", c.Request.URL.String()),
			attribute.String("http.target", c.Request.URL.Path),
			attribute.String("http.client_ip", c.ClientIP()),
			attribute.String("http.user_agent", c.GetHeader("User-Agent")),
		)

		// 将 TraceID 注入到 Context 和响应头
		traceID := span.SpanContext().TraceID().String()
		c.Set(utils.TraceIDKey, traceID)
		c.Header("X-Trace-ID", traceID)

		// 替换 Context 以传递 Span
		c.Request = c.Request.WithContext(ctx)

		c.Next()

		// 记录请求结果
		status := c.Writer.Status()
		span.SetAttributes(
			attribute.Int("http.status_code", status),
			attribute.Float64("http.duration_ms", float64(time.Since(start).Milliseconds())),
		)

		if status >= 400 {
			span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", status))
		}
	}
}

// GetTracer 获取全局 Tracer 实例
func GetTracer() trace.Tracer {
	return tracer
}