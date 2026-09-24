package metrics_test

import (
	"context"

	"github.com/prafdin/simple-workflows/internal/domain"
)

type stubStore struct {
	count int64
	err   error
}

func (s stubStore) Save(_ context.Context, _ domain.Workflow) error { return nil }

func (s stubStore) List(_ context.Context) ([]domain.Workflow, error) { return nil, nil }

func (s stubStore) Get(_ context.Context, _ string) (domain.Workflow, error) {
	return domain.Workflow{}, domain.ErrNotFound
}

func (s stubStore) Count(_ context.Context) (int64, error) { return s.count, s.err }
