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

func TestListReturnsAllSubmittedWorkflows(t *testing.T) {
	store := newFakeStore()
	if err := store.Save(context.Background(), domain.Workflow{Name: "example", Image: "img:v1"}); err != nil {
		t.Fatalf("could not seed workflow: %v", err)
	}
	server := httptest.NewServer(api.NewRouter(store, &fakeRunner{}, &fakeMetrics{}, noop.NewTracerProvider()))
	defer server.Close()

	resp, err := http.Get(server.URL + "/workflows")
	if err != nil {
		t.Fatalf("could not call list endpoint: %v", err)
	}
	defer resp.Body.Close()

	var got []struct {
		Name  string `json:"name"`
		Image string `json:"image"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("could not decode response: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("got %d workflows, want 1", len(got))
	}
}
