package main

import (
	"testing"

	"go.opentelemetry.io/otel/trace/noop"
)

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

func TestBuildClientOptionsHasNoAuthWhenUsernameEmpty(t *testing.T) {
	opts := buildClientOptions("mongodb://localhost:27017", "", "", noop.NewTracerProvider())

	if opts.Auth != nil {
		t.Fatalf("got auth %+v, want nil", opts.Auth)
	}
}

func TestBuildClientOptionsSetsAuthUsernameWhenUsernameProvided(t *testing.T) {
	opts := buildClientOptions("mongodb://localhost:27017", "user", "pass", noop.NewTracerProvider())

	if opts.Auth.Username != "user" {
		t.Fatalf("got username %q, want %q", opts.Auth.Username, "user")
	}
}

func TestBuildClientOptionsSetsAuthPasswordWhenUsernameProvided(t *testing.T) {
	opts := buildClientOptions("mongodb://localhost:27017", "user", "pass", noop.NewTracerProvider())

	if opts.Auth.Password != "pass" {
		t.Fatalf("got password %q, want %q", opts.Auth.Password, "pass")
	}
}

func TestBuildClientOptionsInstallsCommandMonitor(t *testing.T) {
	opts := buildClientOptions("mongodb://localhost:27017", "", "", noop.NewTracerProvider())

	if opts.Monitor == nil {
		t.Fatalf("client options have no command monitor, want otelmongo monitor")
	}
}
