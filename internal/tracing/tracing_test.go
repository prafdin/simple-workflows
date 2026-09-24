package tracing_test

import (
	"context"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/prafdin/simple-workflows/internal/tracing"
)

func recorded(kinds ...trace.SpanKind) int {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(tracing.Sampler()), sdktrace.WithSpanProcessor(recorder))
	ctx := context.Background()
	for _, kind := range kinds {
		var span trace.Span
		ctx, span = provider.Tracer("probe").Start(ctx, "step", trace.WithSpanKind(kind))
		span.End()
	}
	return len(recorder.Ended())
}

func TestSamplerDropsRootClientSpan(t *testing.T) {
	if got := recorded(trace.SpanKindClient); got != 0 {
		t.Fatalf("got %d spans recorded for a root client span, want 0", got)
	}
}

func TestSamplerKeepsRootServerSpan(t *testing.T) {
	if got := recorded(trace.SpanKindServer); got != 1 {
		t.Fatalf("got %d spans recorded for a root server span, want 1", got)
	}
}

func TestSamplerKeepsRootInternalSpan(t *testing.T) {
	if got := recorded(trace.SpanKindInternal); got != 1 {
		t.Fatalf("got %d spans recorded for a root internal span, want 1", got)
	}
}

func TestSamplerKeepsClientSpanUnderSampledParent(t *testing.T) {
	if got := recorded(trace.SpanKindServer, trace.SpanKindClient); got != 2 {
		t.Fatalf("got %d spans recorded for server with client child, want 2", got)
	}
}

func TestSetupReturnsNonRecordingProviderWithoutEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	provider, shutdown, err := tracing.Setup(context.Background())
	if err != nil {
		t.Fatalf("could not set up tracing: %v", err)
	}
	defer shutdown(context.Background())
	_, span := provider.Tracer("probe").Start(context.Background(), "idle", trace.WithSpanKind(trace.SpanKindServer))

	if span.IsRecording() {
		t.Fatalf("span is recording without an OTLP endpoint, want no-op")
	}
}
