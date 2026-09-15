package main

import "testing"

func TestGetenvReturnsFallbackWhenEnvVarUnset(t *testing.T) {
	got := getenv("SIMPLE_WORKFLOWS_UNSET_TEST_VAR", "fallback")

	if got != "fallback" {
		t.Fatalf("got %q, want %q", got, "fallback")
	}
}

func TestGetenvReturnsEnvValueWhenSet(t *testing.T) {
	t.Setenv("SIMPLE_WORKFLOWS_SET_TEST_VAR", "value")

	got := getenv("SIMPLE_WORKFLOWS_SET_TEST_VAR", "fallback")

	if got != "value" {
		t.Fatalf("got %q, want %q", got, "value")
	}
}
