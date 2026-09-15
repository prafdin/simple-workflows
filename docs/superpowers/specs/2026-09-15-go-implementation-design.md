# Simple Workflows — Go Implementation Design

## Purpose

Implement the service described in `README.md`: a REST API that stores workflow
definitions (name + container image) and runs them as one-off Kubernetes Jobs,
reporting status and output back to the caller. This is a study project — scope
is the full feature set already documented in the README, nothing beyond it.

## Module

Go module path: `github.com/prafdin/simple-workflows`.

## Architecture

Hexagonal (ports & adapters). The domain defines two interfaces; two adapter
packages implement them against real infrastructure; the HTTP layer depends
only on the interfaces.

```
cmd/server/               main.go — load config, build Mongo client + k8s
                           clientset, wire adapters, start HTTP server
internal/domain/          Workflow entity, WorkflowStore + WorkflowRunner
                           interfaces, validation
internal/mongostore/      WorkflowStore implementation (MongoDB)
internal/k8sexec/         WorkflowRunner implementation (client-go: create
                           Job, read Job/Pod status, fetch Pod logs)
internal/api/             HTTP handlers (net/http ServeMux), request/response
                           mapping; depends only on domain interfaces
deploy/base/               Dockerfile, Deployment, Service, ServiceAccount +
                           Role + RoleBinding, Kustomize base
deploy/overlays/sample/    ConfigMap patch (Mongo connection), HTTPRoute
                           patch, sample Kustomize overlay
```

### Domain model

`Workflow` is the only entity:

- `Name string` — user-supplied, unique identifier
- `Image string` — container image reference
- `JobName string` — set once a run has been started; empty otherwise
- `SubmittedAt time.Time` — set when a run is started

Name lookups are case-insensitive: the store normalizes `Name` to lowercase
for comparison and storage key, per the README's documented behavior on the
`run`/`status`/`output` endpoints.

```go
type WorkflowStore interface {
    Save(ctx context.Context, w Workflow) error   // upsert by lowercase name
    List(ctx context.Context) ([]Workflow, error)
    Get(ctx context.Context, name string) (Workflow, error) // ErrNotFound if absent
}

type WorkflowRunner interface {
    Run(ctx context.Context, w Workflow) (jobName string, err error)
    Status(ctx context.Context, jobName string) (Status, error)
    Logs(ctx context.Context, jobName string) (io.ReadCloser, error)
}
```

`Status` is one of `pending`, `running`, `succeeded`, `failed`, derived from
the Job's conditions and its Pod's phase.

## Endpoints & data flow

| Endpoint | Flow |
|---|---|
| `POST /workflows` | Parse multipart YAML body → validate (`name`, `image` required) → `WorkflowStore.Save` (upsert by lowercase name) |
| `GET /workflows` | `WorkflowStore.List` → JSON array |
| `GET /workflows/run?name=` | `Get` workflow → reject if a Job is already active for it → `WorkflowRunner.Run` creates a Kubernetes Job → `Save` the returned `JobName`/`SubmittedAt` against the record |
| `GET /workflows/status?name=` | `Get` workflow's `JobName` → `WorkflowRunner.Status` queries the Job live from Kubernetes |
| `GET /workflows/output?name=` | `Get` workflow's `JobName` → `WorkflowRunner.Logs` streams the Pod's container logs live from Kubernetes |

Status and logs are always read live from the Kubernetes API — Mongo stores
only the workflow definition and the last run's `JobName`/`SubmittedAt`, never
a copy of status or output.

## Error handling

- Domain validation errors (missing `name`/`image`, malformed YAML) → `400`
- Unknown workflow name on `run`/`status`/`output` → `404`
- `POST` of an existing name → upsert (overwrite), not an error
- `run` requested while a prior Job for that name is still active → `409 Conflict` — one in-flight run per workflow name at a time
- Adapter failures (Mongo/k8s timeouts, connection errors) → `502`, logged with context; response body carries no internal details
- No retries inside the request path — a failed `run` call returns the error; the caller may retry

## Testing strategy

- `internal/domain` — pure unit tests (no fakes): validation rules, name normalization
- `internal/mongostore` — tests against a real ephemeral MongoDB (testcontainers-go); one assertion per test; covers Save/List/Get, upsert, and case-insensitive lookup
- `internal/k8sexec` — tests against client-go's `fake.Clientset`: Job creation, status mapping from Job/Pod conditions, log retrieval
- `internal/api` — handler tests against small hand-written fakes of `WorkflowStore`/`WorkflowRunner` (no mocking framework); verify status codes and JSON shape, including the 400/404/409/502 cases above
- `cmd/server` — no unit tests; a thin smoke test (start server, hit `GET /workflows`) is sufficient
- All tests are bound by timeouts; the ephemeral Mongo container and the k8s fake clientset both avoid any dependency on the real Internet or a real cluster

## Deployment artifacts

In scope, so the README's existing Installation section works as written:

- `Dockerfile` — multi-stage build producing a static Go binary
- `deploy/base/` — Deployment, Service, ServiceAccount, Role (create/get Jobs, get Pods, get Pod logs), RoleBinding, Kustomize `kustomization.yaml`
- `deploy/overlays/sample/` — `configmap-patch.yaml` (Mongo connection parameters), `httproute-patch.yaml` (Gateway API route), sample `kustomization.yaml`

## Out of scope

- Multi-step / DAG workflows (README's example is a single container per workflow)
- Authentication/authorization
- Workflow run history beyond the single most recent `JobName`
