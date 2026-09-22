package domain

import "context"

type WorkflowStore interface {
	Save(ctx context.Context, w Workflow) error
	List(ctx context.Context) ([]Workflow, error)
	Get(ctx context.Context, name string) (Workflow, error)
}
