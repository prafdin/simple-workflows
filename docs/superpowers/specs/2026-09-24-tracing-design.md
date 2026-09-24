# Simple Workflows — OpenTelemetry Tracing Design

## Purpose

Let an engineer follow one workflow run end to end in Grafana/Tempo: the HTTP
request that started it, the MongoDB and Kubernetes API calls it made, and the
stages the Kubernetes Job went through until it finished. Sub-project 2 of 2;
the Tempo backend (sub-project 1) lives in the gitops repo.

Agreed scope with the user: level 1 (request tracing via auto-instrumentation)
plus level 2 (workflow run lifecycle trace). Level 3 (propagating context into
task containers) is out of scope.

## Traces

### Request trace (level 1)

```
GET /workflows/run                       server   otelhttp, span name = route pattern
 ├─ workflows.find                       client   otelmongo
 ├─ GET  jobs/<prev>                     client   otelhttp transport on client-go
 ├─ POST jobs                            client
 └─ workflows.replace                    client
```

All API routes are traced. `/metrics` on the metrics port is not.

### Run trace (level 2)

`workflow.run` is a child of the `GET /workflows/run` server span, so one trace
holds the request and the run. The request span ends in milliseconds; the run
span ends when the Job finishes.

```
GET /workflows/run
 └─ workflow.run            Job created → Job finished        internal
     ├─ schedule            Job created → pod PodScheduled
     ├─ start               PodScheduled → container startedAt (image pull, create)
     ├─ execute             container startedAt → finishedAt
     └─ complete            container finishedAt → Job finished
```

- Attributes on `workflow.run`: `workflow.name`, `workflow.image`,
  `k8s.job.name`, `workflow.status` (`succeeded`/`failed`).
- `execute` carries `container.exit_code` and `container.reason` when the
  container terminated.
- Failed run: `workflow.run` and `execute` get status Error.
- "Job finished" time: `status.completionTime` when set, otherwise the
  `lastTransitionTime` of the terminal condition.
- A stage whose timestamps are missing (e.g. pod never scheduled, pod already
  gone) is skipped; `workflow.run` is still emitted.

### Context hand-off

- `Runner.Run(ctx, w)` injects the W3C trace context of `ctx` into the Job's
  annotation `simple-workflows/traceparent` (plus `tracestate` if present).
- The watcher, on the unfinished → finished transition it already detects for
  metrics, extracts that context and emits the run spans with explicit start and
  end timestamps. Spans are therefore created retroactively, once, at the end.
- A Job without the annotation (created by an older version) gets a root
  `workflow.run` span.
- Jobs already finished at startup are not traced (same rule as metrics).

### Sampling

Parent-based. For root spans: sample `server` and `internal` spans, drop
`client` spans. This keeps the informer's own list/watch calls and the Mongo
`count` done at every metrics scrape from creating a stream of lone root traces,
while every API request and every run is kept.

## Architecture

```
internal/tracing/      new: Setup(ctx) (TracerProvider, shutdown, error) —
                       OTLP gRPC exporter from standard OTEL_* env,
                       resource with service.name (default "simple-workflows"),
                       W3C propagator, the sampler above; no endpoint env →
                       no-op provider
internal/api/          NewRouter(store, runner, metrics, provider): each route
                       wrapped in otelhttp with the route pattern as span name
internal/k8sexec/      runner.go: inject trace context into Job annotations;
                       chronicle.go: builds run spans from Job + Pod;
                       watcher.go: on completion, loads the Job's pod and hands
                       Job + Pod to the chronicle; NewWatcher gains a provider
cmd/server/            Setup tracing; Mongo client monitor otelmongo; client-go
                       rest.Config.Wrap(otelhttp.NewTransport); pass provider
```

Everything that creates spans takes a `trace.TracerProvider`; there is no use
of the global provider except as propagator registration.

## Deploy

- Kustomize component `deploy/components/tracing`: patches the Deployment with
  `HOST_IP` (from `status.hostIP`), `OTEL_EXPORTER_OTLP_ENDPOINT=http://$(HOST_IP):4317`,
  `OTEL_SERVICE_NAME=simple-workflows`. Included by both sample overlays.
- Base stays tracing-free, so a deployment without a collector runs with the
  no-op provider.
- README: Tracing section.
- Lab rollout: tag `v0.4.0`, gitops overlay `ref`/`newTag` → `v0.4.0` plus the
  tracing component.

## Testing

- `internal/tracing`: sampler keeps root server/internal spans, drops root
  client spans, follows a sampled parent.
- `internal/api`: a request produces a server span named after the route
  pattern (in-memory `tracetest.SpanRecorder`).
- `internal/k8sexec`: `Run` writes the traceparent annotation of the span in
  `ctx`; the watcher, given a Job with annotation and a Pod with conditions and
  container state, records `workflow.run` with the four stages, the right
  parent trace id, timestamps and error status on failure; a missing pod still
  yields `workflow.run`.
- End to end in the lab: run `examples/hello`, find in Tempo one trace holding
  `GET /workflows/run`, its Mongo/k8s client spans and `workflow.run` with its
  stages; k8s attributes present via the collector.

## Out of scope

- Context propagation into task containers (level 3).
- Traces ↔ logs (issue #6).
- Graceful shutdown flushing on SIGTERM beyond what the batch processor does on
  exit; spans of runs finishing during a restart may be lost.
