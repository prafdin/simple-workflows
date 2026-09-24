package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/trace/noop"

	"github.com/prafdin/simple-workflows/internal/api"
	"github.com/prafdin/simple-workflows/internal/domain"
)

func TestStatusReturnsRunnerStatusForActiveWorkflow(t *testing.T) {
	store := newFakeStore()
	if err := store.Save(context.Background(), domain.Workflow{Name: "example", Image: "img:v1", JobName: "example-1"}); err != nil {
		t.Fatalf("could not seed workflow: %v", err)
	}
	runner := &fakeRunner{status: domain.StatusRunning}
	server := httptest.NewServer(api.NewRouter(store, runner, &fakeMetrics{}, noop.NewTracerProvider()))
	defer server.Close()

	resp, err := http.Get(server.URL + "/workflows/status?name=example")
	if err != nil {
		t.Fatalf("could not call status endpoint: %v", err)
	}
	defer resp.Body.Close()

	var got struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("could not decode response: %v", err)
	}

	if got.Status != "running" {
		t.Fatalf("got status %q, want %q", got.Status, "running")
	}
}

func TestStatusFailsWithNotFoundForUnknownWorkflow(t *testing.T) {
	store := newFakeStore()
	server := httptest.NewServer(api.NewRouter(store, &fakeRunner{}, &fakeMetrics{}, noop.NewTracerProvider()))
	defer server.Close()

	resp, err := http.Get(server.URL + "/workflows/status?name=missing")
	if err != nil {
		t.Fatalf("could not call status endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestStatusFailsWithNotFoundForWorkflowNeverRun(t *testing.T) {
	store := newFakeStore()
	if err := store.Save(context.Background(), domain.Workflow{Name: "example", Image: "img:v1"}); err != nil {
		t.Fatalf("could not seed workflow: %v", err)
	}
	server := httptest.NewServer(api.NewRouter(store, &fakeRunner{}, &fakeMetrics{}, noop.NewTracerProvider()))
	defer server.Close()

	resp, err := http.Get(server.URL + "/workflows/status?name=example")
	if err != nil {
		t.Fatalf("could not call status endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}
