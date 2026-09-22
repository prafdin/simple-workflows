package api

import (
	"errors"
	"io"
	"net/http"

	"github.com/prafdin/simple-workflows/internal/domain"
)

func (h *handler) output(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	ctx := r.Context()

	wf, err := h.store.Get(ctx, name)
	if errors.Is(err, domain.ErrNotFound) {
		http.Error(w, "workflow not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "could not load workflow", http.StatusBadGateway)
		return
	}
	if wf.JobName == "" {
		http.Error(w, "workflow has not been run", http.StatusNotFound)
		return
	}

	logs, err := h.runner.Logs(ctx, wf.JobName)
	if err != nil {
		http.Error(w, "could not get workflow output", http.StatusBadGateway)
		return
	}
	defer logs.Close()

	w.Header().Set("Content-Type", "text/plain")
	io.Copy(w, logs)
}
