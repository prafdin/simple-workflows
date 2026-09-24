package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prafdin/simple-workflows/internal/api"
	"github.com/prafdin/simple-workflows/internal/domain"
)

func TestRunStartsJobForKnownWorkflow(t *testing.T) {
	store := newFakeStore()
	if err := store.Save(context.Background(), domain.Workflow{Name: "example", Image: "img:v1"}); err != nil {
		t.Fatalf("could not seed workflow: %v", err)
	}
	runner := &fakeRunner{runJob: "example-123"}
	server := httptest.NewServer(api.NewRouter(store, runner, &fakeMetrics{}))
	defer server.Close()

	resp, err := http.Get(server.URL + "/workflows/run?name=example")
	if err != nil {
		t.Fatalf("could not call run endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusAccepted)
	}
}

func TestRunFailsWithNotFoundForUnknownWorkflow(t *testing.T) {
	store := newFakeStore()
	server := httptest.NewServer(api.NewRouter(store, &fakeRunner{}, &fakeMetrics{}))
	defer server.Close()

	resp, err := http.Get(server.URL + "/workflows/run?name=missing")
	if err != nil {
		t.Fatalf("could not call run endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestRunFailsWithConflictWhenPriorRunActive(t *testing.T) {
	store := newFakeStore()
	if err := store.Save(context.Background(), domain.Workflow{Name: "example", Image: "img:v1", JobName: "example-1"}); err != nil {
		t.Fatalf("could not seed workflow: %v", err)
	}
	runner := &fakeRunner{status: domain.StatusRunning}
	server := httptest.NewServer(api.NewRouter(store, runner, &fakeMetrics{}))
	defer server.Close()

	resp, err := http.Get(server.URL + "/workflows/run?name=example")
	if err != nil {
		t.Fatalf("could not call run endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusConflict)
	}
}

func TestRunCountsStartedRun(t *testing.T) {
	store := newFakeStore()
	if err := store.Save(context.Background(), domain.Workflow{Name: "kepler", Image: "img:v9"}); err != nil {
		t.Fatalf("could not seed workflow: %v", err)
	}
	telemetry := &fakeMetrics{}
	server := httptest.NewServer(api.NewRouter(store, &fakeRunner{runJob: "kepler-981"}, telemetry))
	defer server.Close()

	resp, err := http.Get(server.URL + "/workflows/run?name=kepler")
	if err != nil {
		t.Fatalf("could not call run endpoint: %v", err)
	}
	resp.Body.Close()

	if telemetry.started != 1 {
		t.Fatalf("got %d started runs counted, want 1", telemetry.started)
	}
}

func TestRunDoesNotCountRunInProgress(t *testing.T) {
	store := newFakeStore()
	if err := store.Save(context.Background(), domain.Workflow{Name: "kepler", Image: "img:v9", JobName: "kepler-1"}); err != nil {
		t.Fatalf("could not seed workflow: %v", err)
	}
	telemetry := &fakeMetrics{}
	server := httptest.NewServer(api.NewRouter(store, &fakeRunner{status: domain.StatusRunning}, telemetry))
	defer server.Close()

	resp, err := http.Get(server.URL + "/workflows/run?name=kepler")
	if err != nil {
		t.Fatalf("could not call run endpoint: %v", err)
	}
	resp.Body.Close()

	if telemetry.started != 0 {
		t.Fatalf("got %d started runs counted for conflicting run, want 0", telemetry.started)
	}
}
