package api

import (
	"io"
	"net/http"

	"gopkg.in/yaml.v3"

	"github.com/prafdin/simple-workflows/internal/domain"
)

type workflowYAML struct {
	Name  string `yaml:"name"`
	Image string `yaml:"image"`
}

func (h *handler) submit(w http.ResponseWriter, r *http.Request) {
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	body, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "could not read uploaded file", http.StatusBadRequest)
		return
	}

	var parsed workflowYAML
	if err := yaml.Unmarshal(body, &parsed); err != nil {
		http.Error(w, "invalid YAML", http.StatusBadRequest)
		return
	}

	wf := domain.Workflow{Name: parsed.Name, Image: parsed.Image}
	if err := wf.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.store.Save(r.Context(), wf); err != nil {
		http.Error(w, "could not save workflow", http.StatusBadGateway)
		return
	}

	w.WriteHeader(http.StatusCreated)
}
