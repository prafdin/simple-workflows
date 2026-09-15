package domain

import (
	"context"
	"io"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

type WorkflowRunner interface {
	Run(ctx context.Context, w Workflow) (jobName string, err error)
	Status(ctx context.Context, jobName string) (Status, error)
	Logs(ctx context.Context, jobName string) (io.ReadCloser, error)
}
