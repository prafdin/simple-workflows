package api_test

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prafdin/simple-workflows/internal/api"
)

func postWorkflowYAML(t *testing.T, serverURL, yamlBody string) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", "workflow.yaml")
	if err != nil {
		t.Fatalf("could not create form file: %v", err)
	}
	if _, err := part.Write([]byte(yamlBody)); err != nil {
		t.Fatalf("could not write yaml body: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("could not close multipart writer: %v", err)
	}

	resp, err := http.Post(serverURL+"/workflows", mw.FormDataContentType(), &buf)
	if err != nil {
		t.Fatalf("could not post workflow: %v", err)
	}
	return resp
}

func TestSubmitStoresValidWorkflow(t *testing.T) {
	store := newFakeStore()
	server := httptest.NewServer(api.NewRouter(store, &fakeRunner{}))
	defer server.Close()

	resp := postWorkflowYAML(t, server.URL, "name: example\nimage: docker.io/prafdin/example:v1\n")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusCreated)
	}
}

func TestSubmitFailsWithBadRequestWhenNameMissing(t *testing.T) {
	store := newFakeStore()
	server := httptest.NewServer(api.NewRouter(store, &fakeRunner{}))
	defer server.Close()

	resp := postWorkflowYAML(t, server.URL, "image: docker.io/prafdin/example:v1\n")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestSubmitPersistsWorkflowRetrievableByStore(t *testing.T) {
	store := newFakeStore()
	server := httptest.NewServer(api.NewRouter(store, &fakeRunner{}))
	defer server.Close()

	resp := postWorkflowYAML(t, server.URL, "name: example\nimage: docker.io/prafdin/example:v1\n")
	resp.Body.Close()

	got, err := store.Get(context.Background(), "example")
	if err != nil {
		t.Fatalf("could not get saved workflow: %v", err)
	}

	if got.Image != "docker.io/prafdin/example:v1" {
		t.Fatalf("got image %q, want %q", got.Image, "docker.io/prafdin/example:v1")
	}
}
