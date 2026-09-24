package metrics_test

import (
	"errors"
	"io"
	"log"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/prafdin/simple-workflows/internal/domain"
	"github.com/prafdin/simple-workflows/internal/metrics"
)

func scrape(t *testing.T, telemetry *metrics.Prometheus, expected string, names ...string) error {
	t.Helper()
	server := httptest.NewServer(telemetry.Handler())
	defer server.Close()
	return testutil.ScrapeAndCompare(server.URL, strings.NewReader(expected), names...)
}

func silence(t *testing.T) {
	t.Helper()
	previous := log.Writer()
	log.SetOutput(io.Discard)
	t.Cleanup(func() { log.SetOutput(previous) })
}

func TestWorkflowCreatedIncrementsCreatedCounter(t *testing.T) {
	telemetry := metrics.New(stubStore{count: 11})
	for range 3 {
		telemetry.WorkflowCreated()
	}

	err := scrape(t, telemetry, `
# HELP simple_workflows_workflows_created_total Workflow manifests accepted by POST /workflows.
# TYPE simple_workflows_workflows_created_total counter
simple_workflows_workflows_created_total 3
`, "simple_workflows_workflows_created_total")

	if err != nil {
		t.Fatalf("created counter does not match: %v", err)
	}
}

func TestRunStartedIncrementsStartedCounter(t *testing.T) {
	telemetry := metrics.New(stubStore{count: 4})
	for range 7 {
		telemetry.RunStarted()
	}

	err := scrape(t, telemetry, `
# HELP simple_workflows_runs_started_total Workflow runs started by GET /workflows/run.
# TYPE simple_workflows_runs_started_total counter
simple_workflows_runs_started_total 7
`, "simple_workflows_runs_started_total")

	if err != nil {
		t.Fatalf("started counter does not match: %v", err)
	}
}

func TestRunCompletedIncrementsCounterPerStatus(t *testing.T) {
	telemetry := metrics.New(stubStore{count: 2})
	telemetry.RunCompleted(domain.StatusSucceeded)
	telemetry.RunCompleted(domain.StatusFailed)
	telemetry.RunCompleted(domain.StatusSucceeded)

	err := scrape(t, telemetry, `
# HELP simple_workflows_runs_completed_total Workflow runs that reached a terminal state, by outcome.
# TYPE simple_workflows_runs_completed_total counter
simple_workflows_runs_completed_total{status="failed"} 1
simple_workflows_runs_completed_total{status="succeeded"} 2
`, "simple_workflows_runs_completed_total")

	if err != nil {
		t.Fatalf("completed counter does not match: %v", err)
	}
}

func TestRunCompletedSeriesExistBeforeAnyRun(t *testing.T) {
	telemetry := metrics.New(stubStore{count: 5})

	err := scrape(t, telemetry, `
# HELP simple_workflows_runs_completed_total Workflow runs that reached a terminal state, by outcome.
# TYPE simple_workflows_runs_completed_total counter
simple_workflows_runs_completed_total{status="failed"} 0
simple_workflows_runs_completed_total{status="succeeded"} 0
`, "simple_workflows_runs_completed_total")

	if err != nil {
		t.Fatalf("completed series are not pre-initialized: %v", err)
	}
}

func TestWorkflowsGaugeReportsStoreCount(t *testing.T) {
	telemetry := metrics.New(stubStore{count: 42})

	err := scrape(t, telemetry, `
# HELP simple_workflows_workflows Workflows currently stored.
# TYPE simple_workflows_workflows gauge
simple_workflows_workflows 42
`, "simple_workflows_workflows")

	if err != nil {
		t.Fatalf("workflows gauge does not match: %v", err)
	}
}

func TestWorkflowsGaugeIsOmittedWhenStoreFails(t *testing.T) {
	silence(t)
	telemetry := metrics.New(stubStore{err: errors.New("mongo unreachable")})
	telemetry.RunStarted()

	err := scrape(t, telemetry, `
# HELP simple_workflows_runs_started_total Workflow runs started by GET /workflows/run.
# TYPE simple_workflows_runs_started_total counter
simple_workflows_runs_started_total 1
`, "simple_workflows_runs_started_total", "simple_workflows_workflows")

	if err != nil {
		t.Fatalf("scrape with failing store does not serve counters alone: %v", err)
	}
}

func TestScrapeDoesNotWaitForStuckStoreBeyondTimeout(t *testing.T) {
	silence(t)
	telemetry := metrics.New(stuckStore{})
	done := make(chan error, 1)
	go func() {
		done <- scrape(t, telemetry, `
# HELP simple_workflows_runs_started_total Workflow runs started by GET /workflows/run.
# TYPE simple_workflows_runs_started_total counter
simple_workflows_runs_started_total 0
`, "simple_workflows_runs_started_total", "simple_workflows_workflows")
	}()
	var err error
	select {
	case err = <-done:
	case <-time.After(7 * time.Second):
		err = errors.New("scrape did not return within 7s")
	}

	if err != nil {
		t.Fatalf("scrape with stuck store does not return counters in time: %v", err)
	}
}
