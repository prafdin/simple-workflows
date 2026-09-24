package api_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/trace/noop"

	"github.com/prafdin/simple-workflows/internal/api"
	"github.com/prafdin/simple-workflows/internal/domain"
)

func TestOutputReturnsRunnerLogsForActiveWorkflow(t *testing.T) {
	store := newFakeStore()
	if err := store.Save(context.Background(), domain.Workflow{Name: "example", Image: "img:v1", JobName: "example-1"}); err != nil {
		t.Fatalf("could not seed workflow: %v", err)
	}
	runner := &fakeRunner{logs: "hello from task"}
	server := httptest.NewServer(api.NewRouter(store, runner, &fakeMetrics{}, noop.NewTracerProvider()))
	defer server.Close()

	resp, err := http.Get(server.URL + "/workflows/output?name=example")
	if err != nil {
		t.Fatalf("could not call output endpoint: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("could not read response body: %v", err)
	}

	if string(body) != "hello from task" {
		t.Fatalf("got body %q, want %q", string(body), "hello from task")
	}
}

func TestOutputFailsWithNotFoundForUnknownWorkflow(t *testing.T) {
	store := newFakeStore()
	server := httptest.NewServer(api.NewRouter(store, &fakeRunner{}, &fakeMetrics{}, noop.NewTracerProvider()))
	defer server.Close()

	resp, err := http.Get(server.URL + "/workflows/output?name=missing")
	if err != nil {
		t.Fatalf("could not call output endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}
