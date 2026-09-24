package api

import (
	"net/http"

	"github.com/prafdin/simple-workflows/internal/domain"
)

type handler struct {
	store   domain.WorkflowStore
	runner  domain.WorkflowRunner
	metrics domain.Metrics
}

func NewRouter(store domain.WorkflowStore, runner domain.WorkflowRunner, metrics domain.Metrics) http.Handler {
	h := &handler{store: store, runner: runner, metrics: metrics}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /workflows", h.submit)
	mux.HandleFunc("GET /workflows", h.list)
	mux.HandleFunc("GET /workflows/run", h.run)
	mux.HandleFunc("GET /workflows/status", h.status)
	mux.HandleFunc("GET /workflows/output", h.output)
	return mux
}
