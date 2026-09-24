package api

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/prafdin/simple-workflows/internal/domain"
)

type handler struct {
	store   domain.WorkflowStore
	runner  domain.WorkflowRunner
	metrics domain.Metrics
}

func NewRouter(store domain.WorkflowStore, runner domain.WorkflowRunner, metrics domain.Metrics, provider trace.TracerProvider) http.Handler {
	h := &handler{store: store, runner: runner, metrics: metrics}
	mux := http.NewServeMux()
	route := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, otelhttp.NewHandler(fn, pattern,
			otelhttp.WithTracerProvider(provider),
			otelhttp.WithPropagators(propagation.TraceContext{}),
			otelhttp.WithSpanNameFormatter(func(string, *http.Request) string { return pattern }),
		))
	}
	route("POST /workflows", h.submit)
	route("GET /workflows", h.list)
	route("GET /workflows/run", h.run)
	route("GET /workflows/status", h.status)
	route("GET /workflows/output", h.output)
	return mux
}
