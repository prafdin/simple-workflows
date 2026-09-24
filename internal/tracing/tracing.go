package tracing

import (
	"context"
	"os"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

func Setup(ctx context.Context) (trace.TracerProvider, func(context.Context) error, error) {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" && os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") == "" {
		return noop.NewTracerProvider(), func(context.Context) error { return nil }, nil
	}
	exporter, err := otlptracegrpc.New(ctx)
	if err != nil {
		return nil, nil, err
	}
	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName("simple-workflows")),
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return nil, nil, err
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(Sampler()),
	)
	return provider, provider.Shutdown, nil
}

func Sampler() sdktrace.Sampler {
	return sdktrace.ParentBased(root{})
}

// root keeps root spans that begin a unit of work, such as an API request or
// a workflow run, and drops root client spans from background calls like the
// informer's list/watch or the scrape-time workflow count.
type root struct{}

func (root) ShouldSample(p sdktrace.SamplingParameters) sdktrace.SamplingResult {
	decision := sdktrace.RecordAndSample
	if p.Kind == trace.SpanKindClient {
		decision = sdktrace.Drop
	}
	return sdktrace.SamplingResult{Decision: decision, Tracestate: trace.SpanContextFromContext(p.ParentContext).TraceState()}
}

func (root) Description() string {
	return "root"
}
