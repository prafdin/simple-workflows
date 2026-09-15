package api

import (
	"encoding/json"
	"net/http"
)

type workflowResponse struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	workflows, err := h.store.List(r.Context())
	if err != nil {
		http.Error(w, "could not list workflows", http.StatusBadGateway)
		return
	}

	response := make([]workflowResponse, 0, len(workflows))
	for _, wf := range workflows {
		response = append(response, workflowResponse{Name: wf.Name, Image: wf.Image})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
