package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/prafdin/simple-workflows/internal/api"
)

func TestRouterNamesRequestSpanAfterRoutePattern(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	server := httptest.NewServer(api.NewRouter(newFakeStore(), &fakeRunner{}, &fakeMetrics{}, provider))
	defer server.Close()

	resp, err := http.Get(server.URL + "/workflows/status?name=nova-3")
	if err != nil {
		t.Fatalf("could not call status endpoint: %v", err)
	}
	resp.Body.Close()

	ended := recorder.Ended()
	if len(ended) != 1 || ended[0].Name() != "GET /workflows/status" {
		t.Fatalf("got %d spans %v, want one span named %q", len(ended), ended, "GET /workflows/status")
	}
}
