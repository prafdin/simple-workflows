# Simple Workflows — Prometheus Metrics Design

## Purpose

Expose application metrics for Prometheus (kube-prometheus-stack in the lab
cluster): how many workflows were created, how many runs were started, and how
many runs finished successfully, unsuccessfully and in total.

## Semantics

Events are Prometheus counters kept in process memory; they reset on restart,
which PromQL `increase()`/`rate()` handle. Current state is a gauge recomputed
from the source of truth on every scrape, so it survives restarts.

| Metric | Type | Changes when |
|---|---|---|
| `simple_workflows_workflows_created_total` | counter | `POST /workflows` returns 201 |
| `simple_workflows_runs_started_total` | counter | `GET /workflows/run` returns 202 |
| `simple_workflows_runs_completed_total{status}` | counter | a workflow Job turns `Complete` (`status="succeeded"`) or `Failed` (`status="failed"`) |
| `simple_workflows_workflows` | gauge | every scrape, `CountDocuments` on the workflows collection |

- All finished runs: `sum(simple_workflows_runs_completed_total)`; no separate metric.
- Re-submitting a manifest with an existing name (upsert) increments
  `workflows_created_total`; the gauge does not change.
- Failed handler calls (4xx/5xx) do not touch counters.
- If `Count` fails or exceeds 5s during a scrape, the gauge is omitted from that
  scrape and the error is logged; other metrics are served normally.
- Runs finished while the pod was down are not counted (counter semantics).
- Default `go_*` and `process_*` collectors are registered.

## Architecture

```
internal/domain/     + Metrics interface: WorkflowCreated(), RunStarted(),
                       RunCompleted(Status)
                     + WorkflowStore.Count(ctx) (int64, error)
internal/mongostore/ + Count via CountDocuments
internal/metrics/    new: Prometheus implementation of domain.Metrics
                       (three counters), a Collector for the workflows gauge
                       backed by WorkflowStore.Count, and a dedicated
                       prometheus.Registry (no global registry)
internal/api/        NewRouter(store, runner, metrics); submit and run call
                       metrics only after success
internal/k8sexec/    + Watcher: SharedInformer on Jobs in the namespace with
                       label selector `simple-workflows/workflow`;
                       Job→Status mapping extracted from Runner.Status and
                       shared by both
cmd/server/          start Watcher, wait for cache sync (fatal on timeout),
                       serve /metrics on METRICS_ADDR (default :9090) in a
                       second http.Server
```

### Watcher rules

- `UpdateFunc(old, new)`: if `old` was not finished and `new` is finished,
  call `RunCompleted(status of new)`.
- `AddFunc(obj, isInInitialList)`: if `isInInitialList` is false and `obj` is
  already finished, call `RunCompleted` (Job finished before the watch saw it
  unfinished). Jobs from the initial list are never counted.
- `DeleteFunc`: ignored.
- "Finished" means a `Complete` or `Failed` condition with status `True`, the
  same rule `Runner.Status` uses.

### Why a separate port

`/metrics` stays off the public HTTPRoute, which only targets port 8080.

## Deploy

- `deploy/base/deployment.yaml`: container ports named `http` (8080) and
  `metrics` (9090).
- `deploy/base/service.yaml`: add port `metrics` 9090.
- `deploy/base/configmap.yaml`: add `METRICS_ADDR: ":9090"`.
- `deploy/components/servicemonitor/`: kustomize Component with a
  ServiceMonitor (label `release: kube-prometheus-stack`, port `metrics`,
  path `/metrics`, interval 30s). Kept out of base so clusters without
  prometheus-operator still apply cleanly; both `sample` and
  `sample-with-mtls` overlays include it.
- RBAC: no change, the Role already grants `list` and `watch` on jobs.
- README: short Metrics section with the list and PromQL examples.

## Testing

TDD, in the style of existing tests.

- `internal/api`: a fake `domain.Metrics` records calls; submit and run call it
  on success and do not on failure.
- `internal/k8sexec`: Watcher over `fake.Clientset`; a Job updated to Complete
  counts as succeeded, to Failed as failed; a Job already finished before
  start is not counted. Every wait is bounded by a timeout.
- `internal/mongostore`: `Count` against testcontainers MongoDB.
- `internal/metrics`: gather from the real registry with `testutil`; counters
  reflect calls; gauge equals the stub store count; gauge absent when the store
  fails.
- End-to-end on the lab cluster (proxy variables unset): deploy, create and run
  `examples/hello`, confirm the four metrics in Prometheus.

## Out of scope

- `runs_running` gauge, per-workflow labels, histograms of run duration.
- Persisting counters across restarts.
- Fix for workflow names with spaces (issue #2).
