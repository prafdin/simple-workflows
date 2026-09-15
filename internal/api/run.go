package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/prafdin/simple-workflows/internal/domain"
)

func (h *handler) run(w http.ResponseWriter, r *http.Request) {
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

	if wf.JobName != "" {
		status, err := h.runner.Status(ctx, wf.JobName)
		if err != nil {
			http.Error(w, "could not check previous run status", http.StatusBadGateway)
			return
		}
		if status == domain.StatusPending || status == domain.StatusRunning {
			http.Error(w, "workflow run already in progress", http.StatusConflict)
			return
		}
	}

	jobName, err := h.runner.Run(ctx, wf)
	if err != nil {
		http.Error(w, "could not start workflow run", http.StatusBadGateway)
		return
	}

	wf.JobName = jobName
	wf.SubmittedAt = time.Now().UTC()
	if err := h.store.Save(ctx, wf); err != nil {
		http.Error(w, "could not save run state", http.StatusBadGateway)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}
