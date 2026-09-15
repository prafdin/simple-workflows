package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/prafdin/simple-workflows/internal/domain"
)

type statusResponse struct {
	Status string `json:"status"`
}

func (h *handler) status(w http.ResponseWriter, r *http.Request) {
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

	status, err := h.runner.Status(ctx, wf.JobName)
	if err != nil {
		http.Error(w, "could not get workflow status", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(statusResponse{Status: string(status)})
}
