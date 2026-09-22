package domain_test

import (
	"errors"
	"testing"

	"github.com/prafdin/simple-workflows/internal/domain"
)

func TestValidateFailsWhenNameIsEmpty(t *testing.T) {
	w := domain.Workflow{Name: "", Image: "img:v1"}

	err := w.Validate()

	if !errors.Is(err, domain.ErrNameRequired) {
		t.Fatalf("got error %v, want domain.ErrNameRequired", err)
	}
}

func TestValidateFailsWhenImageIsEmpty(t *testing.T) {
	w := domain.Workflow{Name: "example", Image: ""}

	err := w.Validate()

	if !errors.Is(err, domain.ErrImageRequired) {
		t.Fatalf("got error %v, want domain.ErrImageRequired", err)
	}
}

func TestValidateSucceedsWhenNameAndImageArePresent(t *testing.T) {
	w := domain.Workflow{Name: "example", Image: "img:v1"}

	err := w.Validate()

	if err != nil {
		t.Fatalf("got error %v, want no error", err)
	}
}

func TestNormalizeNameLowercasesInput(t *testing.T) {
	got := domain.NormalizeName("My Workflow")

	if got != "my workflow" {
		t.Fatalf("got %q, want %q", got, "my workflow")
	}
}

func TestNormalizeNameTrimsWhitespace(t *testing.T) {
	got := domain.NormalizeName("  my workflow  ")

	if got != "my workflow" {
		t.Fatalf("got %q, want %q", got, "my workflow")
	}
}
