package domain

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrNameRequired  = errors.New("workflow name is required")
	ErrImageRequired = errors.New("workflow image is required")
	ErrNotFound      = errors.New("workflow not found")
	ErrRunInProgress = errors.New("workflow run already in progress")
)

type Workflow struct {
	Name        string
	Image       string
	JobName     string
	SubmittedAt time.Time
}

func (w Workflow) Validate() error {
	if strings.TrimSpace(w.Name) == "" {
		return ErrNameRequired
	}
	if strings.TrimSpace(w.Image) == "" {
		return ErrImageRequired
	}
	return nil
}

func NormalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
