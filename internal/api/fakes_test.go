package api_test

import (
	"context"
	"io"
	"strings"
	"sync"

	"github.com/prafdin/simple-workflows/internal/domain"
)

type fakeStore struct {
	mu        sync.Mutex
	workflows map[string]domain.Workflow
}

func newFakeStore() *fakeStore {
	return &fakeStore{workflows: map[string]domain.Workflow{}}
}

func (s *fakeStore) Save(_ context.Context, w domain.Workflow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workflows[domain.NormalizeName(w.Name)] = w
	return nil
}

func (s *fakeStore) List(_ context.Context) ([]domain.Workflow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := make([]domain.Workflow, 0, len(s.workflows))
	for _, w := range s.workflows {
		list = append(list, w)
	}
	return list, nil
}

func (s *fakeStore) Get(_ context.Context, name string) (domain.Workflow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.workflows[domain.NormalizeName(name)]
	if !ok {
		return domain.Workflow{}, domain.ErrNotFound
	}
	return w, nil
}

func (s *fakeStore) Count(_ context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return int64(len(s.workflows)), nil
}

type fakeRunner struct {
	runErr    error
	runJob    string
	status    domain.Status
	statusErr error
	logs      string
	logsErr   error
}

func (r *fakeRunner) Run(_ context.Context, _ domain.Workflow) (string, error) {
	if r.runErr != nil {
		return "", r.runErr
	}
	return r.runJob, nil
}

func (r *fakeRunner) Status(_ context.Context, _ string) (domain.Status, error) {
	return r.status, r.statusErr
}

func (r *fakeRunner) Logs(_ context.Context, _ string) (io.ReadCloser, error) {
	if r.logsErr != nil {
		return nil, r.logsErr
	}
	return io.NopCloser(strings.NewReader(r.logs)), nil
}
