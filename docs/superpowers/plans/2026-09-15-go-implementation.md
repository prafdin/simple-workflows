# Simple Workflows Go Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the Go service described in `README.md` — a REST API that stores workflow definitions and runs them as one-off Kubernetes Jobs, plus the deployment artifacts to install it.

**Architecture:** Hexagonal (ports & adapters). `internal/domain` defines `WorkflowStore` and `WorkflowRunner` interfaces; `internal/mongostore` and `internal/k8sexec` implement them; `internal/api` holds HTTP handlers depending only on the interfaces; `cmd/server` wires concrete adapters together.

**Tech Stack:** Go 1.22+, `net/http` `ServeMux` (method-pattern routing), `go.mongodb.org/mongo-driver` (v1 API), `k8s.io/client-go`, `gopkg.in/yaml.v3`, `github.com/testcontainers/testcontainers-go/modules/mongodb` for store tests, `k8s.io/client-go/kubernetes/fake` for runner tests.

**Spec:** `docs/superpowers/specs/2026-09-15-go-implementation-design.md`

## Global Constraints

- Module path: `github.com/prafdin/simple-workflows`
- Go directive: `go 1.22` or higher (required for `ServeMux` method patterns like `"POST /workflows"`)
- One assertion per test, as its last statement; error-checking guard clauses (`if err != nil { t.Fatalf(...) }`) on setup calls are not the assertion under test
- No mocking frameworks — hand-written fakes only, in `_test.go` files
- Tests never depend on the real Internet or a real Kubernetes cluster: k8s-facing tests use `client-go`'s `fake.Clientset`; Mongo-facing tests use a real ephemeral container via testcontainers-go
- Status/output are always read live from Kubernetes at request time — Mongo stores only the workflow definition and the last run's `JobName`/`SubmittedAt`
- Name lookups are case-insensitive via `domain.NormalizeName`

---

### Task 1: Domain model

**Files:**
- Create: `go.mod`
- Create: `internal/domain/workflow.go`
- Create: `internal/domain/store.go`
- Create: `internal/domain/runner.go`
- Test: `internal/domain/workflow_test.go`

**Interfaces:**
- Produces: `domain.Workflow{Name, Image, JobName string; SubmittedAt time.Time}`, `(Workflow) Validate() error`, `domain.NormalizeName(string) string`, `domain.ErrNameRequired`, `domain.ErrImageRequired`, `domain.ErrNotFound`, `domain.ErrRunInProgress`, `domain.WorkflowStore` interface, `domain.WorkflowRunner` interface, `domain.Status` (`StatusPending`, `StatusRunning`, `StatusSucceeded`, `StatusFailed`)

- [ ] **Step 1: Initialize the Go module**

Run: `go mod init github.com/prafdin/simple-workflows && go mod edit -go=1.22`

- [ ] **Step 2: Write the failing tests**

Create `internal/domain/workflow_test.go`:

```go
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
```

- [ ] **Step 3: Run the tests and verify they fail**

Run: `go test ./internal/domain/...`
Expected: FAIL — `internal/domain` package does not exist / `undefined: domain.Workflow`

- [ ] **Step 4: Implement the domain package**

Create `internal/domain/workflow.go`:

```go
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
```

Create `internal/domain/store.go`:

```go
package domain

import "context"

type WorkflowStore interface {
	Save(ctx context.Context, w Workflow) error
	List(ctx context.Context) ([]Workflow, error)
	Get(ctx context.Context, name string) (Workflow, error)
}
```

Create `internal/domain/runner.go`:

```go
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
```

- [ ] **Step 5: Run the tests and verify they pass**

Run: `go test ./internal/domain/...`
Expected: PASS (5 tests)

- [ ] **Step 6: Commit**

```bash
git add go.mod internal/domain
git commit -m "feat: add domain model and ports"
```

---

### Task 2: MongoDB workflow store

**Files:**
- Create: `internal/mongostore/store.go`
- Test: `internal/mongostore/store_test.go`

**Interfaces:**
- Consumes: `domain.Workflow`, `domain.NormalizeName`, `domain.ErrNotFound`, `domain.WorkflowStore`
- Produces: `mongostore.New(collection *mongo.Collection) *mongostore.Store` implementing `domain.WorkflowStore`

- [ ] **Step 1: Add dependencies**

Run: `go get go.mongodb.org/mongo-driver/mongo@latest && go get github.com/testcontainers/testcontainers-go@latest && go get github.com/testcontainers/testcontainers-go/modules/mongodb@latest`

- [ ] **Step 2: Write the failing tests**

Create `internal/mongostore/store_test.go`:

```go
package mongostore_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/prafdin/simple-workflows/internal/domain"
	"github.com/prafdin/simple-workflows/internal/mongostore"
)

func newTestCollection(t *testing.T) *mongo.Collection {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	container, err := mongodb.Run(ctx, "mongo:7")
	if err != nil {
		t.Fatalf("could not start mongodb container: %v", err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Fatalf("could not terminate mongodb container: %v", err)
		}
	})

	uri, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("could not get mongodb connection string: %v", err)
	}

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("could not connect to mongodb: %v", err)
	}
	t.Cleanup(func() {
		if err := client.Disconnect(context.Background()); err != nil {
			t.Fatalf("could not disconnect from mongodb: %v", err)
		}
	})

	return client.Database("simple_workflows_test").Collection("workflows")
}

func TestGetReturnsWorkflowSavedWithSameName(t *testing.T) {
	store := mongostore.New(newTestCollection(t))
	ctx := context.Background()
	saved := domain.Workflow{Name: "My Workflow", Image: "example:v1", JobName: "job-1", SubmittedAt: time.Now().UTC().Truncate(time.Second)}

	if err := store.Save(ctx, saved); err != nil {
		t.Fatalf("could not save workflow: %v", err)
	}
	got, err := store.Get(ctx, "My Workflow")
	if err != nil {
		t.Fatalf("could not get saved workflow: %v", err)
	}

	if got != saved {
		t.Fatalf("got workflow %+v, does not equal saved workflow %+v", got, saved)
	}
}

func TestGetNormalizesNameCase(t *testing.T) {
	store := mongostore.New(newTestCollection(t))
	ctx := context.Background()
	if err := store.Save(ctx, domain.Workflow{Name: "Mixed Case", Image: "example:v1"}); err != nil {
		t.Fatalf("could not save workflow: %v", err)
	}

	got, err := store.Get(ctx, "mixed case")
	if err != nil {
		t.Fatalf("could not get workflow by lowercase name: %v", err)
	}

	if got.Name != "Mixed Case" {
		t.Fatalf("got name %q, want %q", got.Name, "Mixed Case")
	}
}

func TestSaveUpsertsExistingWorkflow(t *testing.T) {
	store := mongostore.New(newTestCollection(t))
	ctx := context.Background()
	if err := store.Save(ctx, domain.Workflow{Name: "dup", Image: "v1"}); err != nil {
		t.Fatalf("could not save first version: %v", err)
	}
	if err := store.Save(ctx, domain.Workflow{Name: "dup", Image: "v2"}); err != nil {
		t.Fatalf("could not save second version: %v", err)
	}

	got, err := store.Get(ctx, "dup")
	if err != nil {
		t.Fatalf("could not get workflow: %v", err)
	}

	if got.Image != "v2" {
		t.Fatalf("got image %q, want %q after upsert", got.Image, "v2")
	}
}

func TestListReturnsAllSavedWorkflows(t *testing.T) {
	store := mongostore.New(newTestCollection(t))
	ctx := context.Background()
	if err := store.Save(ctx, domain.Workflow{Name: "one", Image: "v1"}); err != nil {
		t.Fatalf("could not save workflow one: %v", err)
	}
	if err := store.Save(ctx, domain.Workflow{Name: "two", Image: "v1"}); err != nil {
		t.Fatalf("could not save workflow two: %v", err)
	}

	got, err := store.List(ctx)
	if err != nil {
		t.Fatalf("could not list workflows: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("got %d workflows, want 2", len(got))
	}
}

func TestGetOfUnknownNameFailsWithNotFound(t *testing.T) {
	store := mongostore.New(newTestCollection(t))

	_, err := store.Get(context.Background(), "missing")

	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("got error %v, want domain.ErrNotFound", err)
	}
}
```

Note: each test starts its own ephemeral Mongo container (no shared container/state between tests), matching the "isolate tests" preference; this costs a few seconds per test, which is fine for a study project.

- [ ] **Step 3: Run the tests and verify they fail**

Run: `go test ./internal/mongostore/...`
Expected: FAIL — `internal/mongostore` package does not exist / `undefined: mongostore.New`

- [ ] **Step 4: Implement the store**

Create `internal/mongostore/store.go`:

```go
package mongostore

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/prafdin/simple-workflows/internal/domain"
)

type Store struct {
	collection *mongo.Collection
}

func New(collection *mongo.Collection) *Store {
	return &Store{collection: collection}
}

type document struct {
	ID          string    `bson:"_id"`
	Name        string    `bson:"name"`
	Image       string    `bson:"image"`
	JobName     string    `bson:"jobName"`
	SubmittedAt time.Time `bson:"submittedAt"`
}

func (s *Store) Save(ctx context.Context, w domain.Workflow) error {
	id := domain.NormalizeName(w.Name)
	doc := document{
		ID:          id,
		Name:        w.Name,
		Image:       w.Image,
		JobName:     w.JobName,
		SubmittedAt: w.SubmittedAt,
	}
	opts := options.Replace().SetUpsert(true)
	_, err := s.collection.ReplaceOne(ctx, bson.M{"_id": id}, doc, opts)
	return err
}

func (s *Store) List(ctx context.Context) ([]domain.Workflow, error) {
	cursor, err := s.collection.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var workflows []domain.Workflow
	for cursor.Next(ctx) {
		var doc document
		if err := cursor.Decode(&doc); err != nil {
			return nil, err
		}
		workflows = append(workflows, toDomain(doc))
	}
	return workflows, cursor.Err()
}

func (s *Store) Get(ctx context.Context, name string) (domain.Workflow, error) {
	id := domain.NormalizeName(name)
	var doc document
	err := s.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return domain.Workflow{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Workflow{}, err
	}
	return toDomain(doc), nil
}

func toDomain(doc document) domain.Workflow {
	return domain.Workflow{
		Name:        doc.Name,
		Image:       doc.Image,
		JobName:     doc.JobName,
		SubmittedAt: doc.SubmittedAt,
	}
}
```

- [ ] **Step 5: Run the tests and verify they pass**

Run: `go test ./internal/mongostore/...`
Expected: PASS (5 tests) — requires Docker available for testcontainers

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/mongostore
git commit -m "feat: add MongoDB workflow store"
```

---

### Task 3: Kubernetes runner — Run

**Files:**
- Create: `internal/k8sexec/runner.go`
- Test: `internal/k8sexec/run_test.go`

**Interfaces:**
- Consumes: `domain.Workflow`, `domain.NormalizeName`
- Produces: `k8sexec.New(clientset kubernetes.Interface, namespace string) *k8sexec.Runner` with `(*Runner) Run(ctx, domain.Workflow) (string, error)` (partial implementation of `domain.WorkflowRunner`, completed in Tasks 4-5)

- [ ] **Step 1: Add dependencies**

Run: `go get k8s.io/client-go@latest && go get k8s.io/api@latest && go get k8s.io/apimachinery@latest`

- [ ] **Step 2: Write the failing test**

Create `internal/k8sexec/run_test.go`:

```go
package k8sexec_test

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/prafdin/simple-workflows/internal/domain"
	"github.com/prafdin/simple-workflows/internal/k8sexec"
)

func TestRunCreatesJobWithWorkflowImage(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	runner := k8sexec.New(clientset, "default")

	jobName, err := runner.Run(context.Background(), domain.Workflow{Name: "example", Image: "docker.io/prafdin/example:v1"})
	if err != nil {
		t.Fatalf("could not run workflow: %v", err)
	}

	job, err := clientset.BatchV1().Jobs("default").Get(context.Background(), jobName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("could not get created job: %v", err)
	}

	if job.Spec.Template.Spec.Containers[0].Image != "docker.io/prafdin/example:v1" {
		t.Fatalf("got image %q, want %q", job.Spec.Template.Spec.Containers[0].Image, "docker.io/prafdin/example:v1")
	}
}
```

- [ ] **Step 3: Run the test and verify it fails**

Run: `go test ./internal/k8sexec/...`
Expected: FAIL — `internal/k8sexec` package does not exist / `undefined: k8sexec.New`

- [ ] **Step 4: Implement Run**

Create `internal/k8sexec/runner.go`:

```go
package k8sexec

import (
	"context"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/prafdin/simple-workflows/internal/domain"
)

type Runner struct {
	clientset kubernetes.Interface
	namespace string
}

func New(clientset kubernetes.Interface, namespace string) *Runner {
	return &Runner{clientset: clientset, namespace: namespace}
}

func (r *Runner) Run(ctx context.Context, w domain.Workflow) (string, error) {
	jobName := fmt.Sprintf("%s-%d", domain.NormalizeName(w.Name), time.Now().UnixNano())
	backoffLimit := int32(0)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: r.namespace,
			Labels:    map[string]string{"simple-workflows/workflow": domain.NormalizeName(w.Name)},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoffLimit,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:  "task",
							Image: w.Image,
						},
					},
				},
			},
		},
	}

	created, err := r.clientset.BatchV1().Jobs(r.namespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return "", err
	}
	return created.Name, nil
}
```

- [ ] **Step 5: Run the test and verify it passes**

Run: `go test ./internal/k8sexec/...`
Expected: PASS (1 test)

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/k8sexec
git commit -m "feat: create kubernetes job on workflow run"
```

---

### Task 4: Kubernetes runner — Status

**Files:**
- Modify: `internal/k8sexec/runner.go`
- Test: `internal/k8sexec/status_test.go`

**Interfaces:**
- Consumes: `domain.Status`, `domain.StatusPending/Running/Succeeded/Failed`
- Produces: `(*Runner) Status(ctx, jobName string) (domain.Status, error)`

- [ ] **Step 1: Write the failing tests**

Create `internal/k8sexec/status_test.go`:

```go
package k8sexec_test

import (
	"context"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/prafdin/simple-workflows/internal/domain"
	"github.com/prafdin/simple-workflows/internal/k8sexec"
)

func createJob(t *testing.T, clientset *fake.Clientset, name string, conditions []batchv1.JobCondition, active int32) {
	t.Helper()
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Status:     batchv1.JobStatus{Conditions: conditions, Active: active},
	}
	if _, err := clientset.BatchV1().Jobs("default").Create(context.Background(), job, metav1.CreateOptions{}); err != nil {
		t.Fatalf("could not seed job: %v", err)
	}
}

func TestStatusReturnsFailedWhenJobHasFailedCondition(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	createJob(t, clientset, "job-failed", []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue}}, 0)
	runner := k8sexec.New(clientset, "default")

	got, err := runner.Status(context.Background(), "job-failed")
	if err != nil {
		t.Fatalf("could not get status: %v", err)
	}

	if got != domain.StatusFailed {
		t.Fatalf("got status %q, want %q", got, domain.StatusFailed)
	}
}

func TestStatusReturnsSucceededWhenJobHasCompleteCondition(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	createJob(t, clientset, "job-complete", []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}, 0)
	runner := k8sexec.New(clientset, "default")

	got, err := runner.Status(context.Background(), "job-complete")
	if err != nil {
		t.Fatalf("could not get status: %v", err)
	}

	if got != domain.StatusSucceeded {
		t.Fatalf("got status %q, want %q", got, domain.StatusSucceeded)
	}
}

func TestStatusReturnsRunningWhenJobHasActivePods(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	createJob(t, clientset, "job-active", nil, 1)
	runner := k8sexec.New(clientset, "default")

	got, err := runner.Status(context.Background(), "job-active")
	if err != nil {
		t.Fatalf("could not get status: %v", err)
	}

	if got != domain.StatusRunning {
		t.Fatalf("got status %q, want %q", got, domain.StatusRunning)
	}
}

func TestStatusReturnsPendingWhenJobHasNoConditionsOrActivePods(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	createJob(t, clientset, "job-new", nil, 0)
	runner := k8sexec.New(clientset, "default")

	got, err := runner.Status(context.Background(), "job-new")
	if err != nil {
		t.Fatalf("could not get status: %v", err)
	}

	if got != domain.StatusPending {
		t.Fatalf("got status %q, want %q", got, domain.StatusPending)
	}
}
```

- [ ] **Step 2: Run the tests and verify they fail**

Run: `go test ./internal/k8sexec/...`
Expected: FAIL — `(*k8sexec.Runner) has no field or method Status`

- [ ] **Step 3: Implement Status**

Append to `internal/k8sexec/runner.go`:

```go
func (r *Runner) Status(ctx context.Context, jobName string) (domain.Status, error) {
	job, err := r.clientset.BatchV1().Jobs(r.namespace).Get(ctx, jobName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	for _, cond := range job.Status.Conditions {
		if cond.Type == batchv1.JobFailed && cond.Status == corev1.ConditionTrue {
			return domain.StatusFailed, nil
		}
		if cond.Type == batchv1.JobComplete && cond.Status == corev1.ConditionTrue {
			return domain.StatusSucceeded, nil
		}
	}
	if job.Status.Active > 0 {
		return domain.StatusRunning, nil
	}
	return domain.StatusPending, nil
}
```

- [ ] **Step 4: Run the tests and verify they pass**

Run: `go test ./internal/k8sexec/...`
Expected: PASS (5 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/k8sexec
git commit -m "feat: derive workflow status from kubernetes job conditions"
```

---

### Task 5: Kubernetes runner — Logs

**Files:**
- Modify: `internal/k8sexec/runner.go`
- Test: `internal/k8sexec/logs_test.go`

**Interfaces:**
- Produces: `(*Runner) Logs(ctx, jobName string) (io.ReadCloser, error)` — completes `domain.WorkflowRunner`

- [ ] **Step 1: Write the failing tests**

Create `internal/k8sexec/logs_test.go`:

```go
package k8sexec_test

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/prafdin/simple-workflows/internal/domain"
	"github.com/prafdin/simple-workflows/internal/k8sexec"
)

func TestLogsFailsWithNotFoundWhenNoPodExistsForJob(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	runner := k8sexec.New(clientset, "default")

	_, err := runner.Logs(context.Background(), "job-without-pod")

	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("got error %v, want domain.ErrNotFound", err)
	}
}

func TestLogsSucceedsWhenPodExistsForJob(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "job-with-pod-abcde",
			Namespace: "default",
			Labels:    map[string]string{"job-name": "job-with-pod"},
		},
	}
	if _, err := clientset.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{}); err != nil {
		t.Fatalf("could not seed pod: %v", err)
	}
	runner := k8sexec.New(clientset, "default")

	_, err := runner.Logs(context.Background(), "job-with-pod")

	if err != nil {
		t.Fatalf("got error %v, want no error", err)
	}
}
```

Note: `client-go`'s fake `GetLogs` always returns an empty, error-free stream — it doesn't simulate real log content — so the second test only asserts that a Pod matching the `job-name` label is found and no error occurs; real log content is exercised manually against a real cluster.

- [ ] **Step 2: Run the tests and verify they fail**

Run: `go test ./internal/k8sexec/...`
Expected: FAIL — `(*k8sexec.Runner) has no field or method Logs`

- [ ] **Step 3: Implement Logs**

Append to `internal/k8sexec/runner.go` (add `"fmt"`... already imported; add `"io"` to the import block):

```go
func (r *Runner) Logs(ctx context.Context, jobName string) (io.ReadCloser, error) {
	pods, err := r.clientset.CoreV1().Pods(r.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("job-name=%s", jobName),
	})
	if err != nil {
		return nil, err
	}
	if len(pods.Items) == 0 {
		return nil, domain.ErrNotFound
	}
	req := r.clientset.CoreV1().Pods(r.namespace).GetLogs(pods.Items[0].Name, &corev1.PodLogOptions{Container: "task"})
	return req.Stream(ctx)
}
```

Update the `import` block at the top of `internal/k8sexec/runner.go` to include `"io"`:

```go
import (
	"context"
	"fmt"
	"io"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/prafdin/simple-workflows/internal/domain"
)
```

- [ ] **Step 4: Run the tests and verify they pass**

Run: `go test ./internal/k8sexec/...`
Expected: PASS (7 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/k8sexec
git commit -m "feat: stream pod logs for workflow output"
```

---

### Task 6: HTTP API — submit workflow

**Files:**
- Create: `internal/api/router.go`
- Create: `internal/api/submit.go`
- Create: `internal/api/fakes_test.go`
- Test: `internal/api/submit_test.go`

**Interfaces:**
- Consumes: `domain.WorkflowStore`, `domain.WorkflowRunner`, `domain.Workflow`, `(Workflow) Validate() error`, `domain.NormalizeName`, `domain.ErrNotFound`
- Produces: `api.NewRouter(store domain.WorkflowStore, runner domain.WorkflowRunner) http.Handler`; test fakes `fakeStore` and `fakeRunner` (both used by every later task in this package)

- [ ] **Step 1: Add dependency**

Run: `go get gopkg.in/yaml.v3@latest`

- [ ] **Step 2: Write the failing tests**

Create `internal/api/fakes_test.go` (shared test doubles for the whole package):

```go
package api_test

import (
	"context"
	"io"
	"strings"
	"sync"

	"github.com/prafdin/simple-workflows/internal/domain"
)

type fakeStore struct {
	mu        sync.Mutex
	workflows map[string]domain.Workflow
}

func newFakeStore() *fakeStore {
	return &fakeStore{workflows: map[string]domain.Workflow{}}
}

func (s *fakeStore) Save(_ context.Context, w domain.Workflow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workflows[domain.NormalizeName(w.Name)] = w
	return nil
}

func (s *fakeStore) List(_ context.Context) ([]domain.Workflow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := make([]domain.Workflow, 0, len(s.workflows))
	for _, w := range s.workflows {
		list = append(list, w)
	}
	return list, nil
}

func (s *fakeStore) Get(_ context.Context, name string) (domain.Workflow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.workflows[domain.NormalizeName(name)]
	if !ok {
		return domain.Workflow{}, domain.ErrNotFound
	}
	return w, nil
}

type fakeRunner struct {
	runErr    error
	runJob    string
	status    domain.Status
	statusErr error
	logs      string
	logsErr   error
}

func (r *fakeRunner) Run(_ context.Context, _ domain.Workflow) (string, error) {
	if r.runErr != nil {
		return "", r.runErr
	}
	return r.runJob, nil
}

func (r *fakeRunner) Status(_ context.Context, _ string) (domain.Status, error) {
	return r.status, r.statusErr
}

func (r *fakeRunner) Logs(_ context.Context, _ string) (io.ReadCloser, error) {
	if r.logsErr != nil {
		return nil, r.logsErr
	}
	return io.NopCloser(strings.NewReader(r.logs)), nil
}
```

Create `internal/api/submit_test.go`:

```go
package api_test

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prafdin/simple-workflows/internal/api"
)

func postWorkflowYAML(t *testing.T, serverURL, yamlBody string) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", "workflow.yaml")
	if err != nil {
		t.Fatalf("could not create form file: %v", err)
	}
	if _, err := part.Write([]byte(yamlBody)); err != nil {
		t.Fatalf("could not write yaml body: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("could not close multipart writer: %v", err)
	}

	resp, err := http.Post(serverURL+"/workflows", mw.FormDataContentType(), &buf)
	if err != nil {
		t.Fatalf("could not post workflow: %v", err)
	}
	return resp
}

func TestSubmitStoresValidWorkflow(t *testing.T) {
	store := newFakeStore()
	server := httptest.NewServer(api.NewRouter(store, &fakeRunner{}))
	defer server.Close()

	resp := postWorkflowYAML(t, server.URL, "name: example\nimage: docker.io/prafdin/example:v1\n")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusCreated)
	}
}

func TestSubmitFailsWithBadRequestWhenNameMissing(t *testing.T) {
	store := newFakeStore()
	server := httptest.NewServer(api.NewRouter(store, &fakeRunner{}))
	defer server.Close()

	resp := postWorkflowYAML(t, server.URL, "image: docker.io/prafdin/example:v1\n")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestSubmitPersistsWorkflowRetrievableByStore(t *testing.T) {
	store := newFakeStore()
	server := httptest.NewServer(api.NewRouter(store, &fakeRunner{}))
	defer server.Close()

	resp := postWorkflowYAML(t, server.URL, "name: example\nimage: docker.io/prafdin/example:v1\n")
	resp.Body.Close()

	got, err := store.Get(context.Background(), "example")
	if err != nil {
		t.Fatalf("could not get saved workflow: %v", err)
	}

	if got.Image != "docker.io/prafdin/example:v1" {
		t.Fatalf("got image %q, want %q", got.Image, "docker.io/prafdin/example:v1")
	}
}
```

- [ ] **Step 3: Run the tests and verify they fail**

Run: `go test ./internal/api/...`
Expected: FAIL — `internal/api` package does not exist / `undefined: api.NewRouter`

- [ ] **Step 4: Implement the router and submit handler**

Create `internal/api/router.go`:

```go
package api

import (
	"net/http"

	"github.com/prafdin/simple-workflows/internal/domain"
)

type handler struct {
	store  domain.WorkflowStore
	runner domain.WorkflowRunner
}

func NewRouter(store domain.WorkflowStore, runner domain.WorkflowRunner) http.Handler {
	h := &handler{store: store, runner: runner}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /workflows", h.submit)
	return mux
}
```

Create `internal/api/submit.go`:

```go
package api

import (
	"io"
	"net/http"

	"gopkg.in/yaml.v3"

	"github.com/prafdin/simple-workflows/internal/domain"
)

type workflowYAML struct {
	Name  string `yaml:"name"`
	Image string `yaml:"image"`
}

func (h *handler) submit(w http.ResponseWriter, r *http.Request) {
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	body, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "could not read uploaded file", http.StatusBadRequest)
		return
	}

	var parsed workflowYAML
	if err := yaml.Unmarshal(body, &parsed); err != nil {
		http.Error(w, "invalid YAML", http.StatusBadRequest)
		return
	}

	wf := domain.Workflow{Name: parsed.Name, Image: parsed.Image}
	if err := wf.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.store.Save(r.Context(), wf); err != nil {
		http.Error(w, "could not save workflow", http.StatusBadGateway)
		return
	}

	w.WriteHeader(http.StatusCreated)
}
```

- [ ] **Step 5: Run the tests and verify they pass**

Run: `go test ./internal/api/...`
Expected: PASS (3 tests)

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/api
git commit -m "feat: add POST /workflows submit endpoint"
```

---

### Task 7: HTTP API — list workflows

**Files:**
- Create: `internal/api/list.go`
- Modify: `internal/api/router.go`
- Test: `internal/api/list_test.go`

**Interfaces:**
- Consumes: `fakeStore`, `fakeRunner` (from Task 6)

- [ ] **Step 1: Write the failing test**

Create `internal/api/list_test.go`:

```go
package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prafdin/simple-workflows/internal/api"
	"github.com/prafdin/simple-workflows/internal/domain"
)

func TestListReturnsAllSubmittedWorkflows(t *testing.T) {
	store := newFakeStore()
	if err := store.Save(context.Background(), domain.Workflow{Name: "example", Image: "img:v1"}); err != nil {
		t.Fatalf("could not seed workflow: %v", err)
	}
	server := httptest.NewServer(api.NewRouter(store, &fakeRunner{}))
	defer server.Close()

	resp, err := http.Get(server.URL + "/workflows")
	if err != nil {
		t.Fatalf("could not call list endpoint: %v", err)
	}
	defer resp.Body.Close()

	var got []struct {
		Name  string `json:"name"`
		Image string `json:"image"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("could not decode response: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("got %d workflows, want 1", len(got))
	}
}
```

- [ ] **Step 2: Run the test and verify it fails**

Run: `go test ./internal/api/...`
Expected: FAIL — `GET /workflows` returns 404 (route not registered)

- [ ] **Step 3: Implement the list handler**

Create `internal/api/list.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
)

type workflowResponse struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	workflows, err := h.store.List(r.Context())
	if err != nil {
		http.Error(w, "could not list workflows", http.StatusBadGateway)
		return
	}

	response := make([]workflowResponse, 0, len(workflows))
	for _, wf := range workflows {
		response = append(response, workflowResponse{Name: wf.Name, Image: wf.Image})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
```

Modify `internal/api/router.go` to register the route:

```go
	mux.HandleFunc("POST /workflows", h.submit)
	mux.HandleFunc("GET /workflows", h.list)
```

- [ ] **Step 4: Run the test and verify it passes**

Run: `go test ./internal/api/...`
Expected: PASS (4 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/api
git commit -m "feat: add GET /workflows list endpoint"
```

---

### Task 8: HTTP API — run workflow

**Files:**
- Create: `internal/api/run.go`
- Modify: `internal/api/router.go`
- Test: `internal/api/run_test.go`

**Interfaces:**
- Consumes: `domain.StatusPending`, `domain.StatusRunning`, `fakeStore`, `fakeRunner{runJob, status}`
- Produces: `(*handler) run(http.ResponseWriter, *http.Request)`

- [ ] **Step 1: Write the failing tests**

Create `internal/api/run_test.go`:

```go
package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prafdin/simple-workflows/internal/api"
	"github.com/prafdin/simple-workflows/internal/domain"
)

func TestRunStartsJobForKnownWorkflow(t *testing.T) {
	store := newFakeStore()
	if err := store.Save(context.Background(), domain.Workflow{Name: "example", Image: "img:v1"}); err != nil {
		t.Fatalf("could not seed workflow: %v", err)
	}
	runner := &fakeRunner{runJob: "example-123"}
	server := httptest.NewServer(api.NewRouter(store, runner))
	defer server.Close()

	resp, err := http.Get(server.URL + "/workflows/run?name=example")
	if err != nil {
		t.Fatalf("could not call run endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusAccepted)
	}
}

func TestRunFailsWithNotFoundForUnknownWorkflow(t *testing.T) {
	store := newFakeStore()
	server := httptest.NewServer(api.NewRouter(store, &fakeRunner{}))
	defer server.Close()

	resp, err := http.Get(server.URL + "/workflows/run?name=missing")
	if err != nil {
		t.Fatalf("could not call run endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestRunFailsWithConflictWhenPriorRunActive(t *testing.T) {
	store := newFakeStore()
	if err := store.Save(context.Background(), domain.Workflow{Name: "example", Image: "img:v1", JobName: "example-1"}); err != nil {
		t.Fatalf("could not seed workflow: %v", err)
	}
	runner := &fakeRunner{status: domain.StatusRunning}
	server := httptest.NewServer(api.NewRouter(store, runner))
	defer server.Close()

	resp, err := http.Get(server.URL + "/workflows/run?name=example")
	if err != nil {
		t.Fatalf("could not call run endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusConflict)
	}
}
```

- [ ] **Step 2: Run the tests and verify they fail**

Run: `go test ./internal/api/...`
Expected: FAIL — `GET /workflows/run` returns 404 (route not registered)

- [ ] **Step 3: Implement the run handler**

Create `internal/api/run.go`:

```go
package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/prafdin/simple-workflows/internal/domain"
)

func (h *handler) run(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	ctx := r.Context()

	wf, err := h.store.Get(ctx, name)
	if errors.Is(err, domain.ErrNotFound) {
		http.Error(w, "workflow not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "could not load workflow", http.StatusBadGateway)
		return
	}

	if wf.JobName != "" {
		status, err := h.runner.Status(ctx, wf.JobName)
		if err != nil {
			http.Error(w, "could not check previous run status", http.StatusBadGateway)
			return
		}
		if status == domain.StatusPending || status == domain.StatusRunning {
			http.Error(w, "workflow run already in progress", http.StatusConflict)
			return
		}
	}

	jobName, err := h.runner.Run(ctx, wf)
	if err != nil {
		http.Error(w, "could not start workflow run", http.StatusBadGateway)
		return
	}

	wf.JobName = jobName
	wf.SubmittedAt = time.Now().UTC()
	if err := h.store.Save(ctx, wf); err != nil {
		http.Error(w, "could not save run state", http.StatusBadGateway)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}
```

Modify `internal/api/router.go` to register the route:

```go
	mux.HandleFunc("GET /workflows", h.list)
	mux.HandleFunc("GET /workflows/run", h.run)
```

- [ ] **Step 4: Run the tests and verify they pass**

Run: `go test ./internal/api/...`
Expected: PASS (7 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/api
git commit -m "feat: add GET /workflows/run endpoint"
```

---

### Task 9: HTTP API — workflow status

**Files:**
- Create: `internal/api/status.go`
- Modify: `internal/api/router.go`
- Test: `internal/api/status_test.go`

**Interfaces:**
- Consumes: `fakeStore`, `fakeRunner{status, statusErr}`, `domain.Status`

- [ ] **Step 1: Write the failing tests**

Create `internal/api/status_test.go`:

```go
package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prafdin/simple-workflows/internal/api"
	"github.com/prafdin/simple-workflows/internal/domain"
)

func TestStatusReturnsRunnerStatusForActiveWorkflow(t *testing.T) {
	store := newFakeStore()
	if err := store.Save(context.Background(), domain.Workflow{Name: "example", Image: "img:v1", JobName: "example-1"}); err != nil {
		t.Fatalf("could not seed workflow: %v", err)
	}
	runner := &fakeRunner{status: domain.StatusRunning}
	server := httptest.NewServer(api.NewRouter(store, runner))
	defer server.Close()

	resp, err := http.Get(server.URL + "/workflows/status?name=example")
	if err != nil {
		t.Fatalf("could not call status endpoint: %v", err)
	}
	defer resp.Body.Close()

	var got struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("could not decode response: %v", err)
	}

	if got.Status != "running" {
		t.Fatalf("got status %q, want %q", got.Status, "running")
	}
}

func TestStatusFailsWithNotFoundForUnknownWorkflow(t *testing.T) {
	store := newFakeStore()
	server := httptest.NewServer(api.NewRouter(store, &fakeRunner{}))
	defer server.Close()

	resp, err := http.Get(server.URL + "/workflows/status?name=missing")
	if err != nil {
		t.Fatalf("could not call status endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestStatusFailsWithNotFoundForWorkflowNeverRun(t *testing.T) {
	store := newFakeStore()
	if err := store.Save(context.Background(), domain.Workflow{Name: "example", Image: "img:v1"}); err != nil {
		t.Fatalf("could not seed workflow: %v", err)
	}
	server := httptest.NewServer(api.NewRouter(store, &fakeRunner{}))
	defer server.Close()

	resp, err := http.Get(server.URL + "/workflows/status?name=example")
	if err != nil {
		t.Fatalf("could not call status endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}
```

- [ ] **Step 2: Run the tests and verify they fail**

Run: `go test ./internal/api/...`
Expected: FAIL — `GET /workflows/status` returns 404 (route not registered)

- [ ] **Step 3: Implement the status handler**

Create `internal/api/status.go`:

```go
package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/prafdin/simple-workflows/internal/domain"
)

type statusResponse struct {
	Status string `json:"status"`
}

func (h *handler) status(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	ctx := r.Context()

	wf, err := h.store.Get(ctx, name)
	if errors.Is(err, domain.ErrNotFound) {
		http.Error(w, "workflow not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "could not load workflow", http.StatusBadGateway)
		return
	}
	if wf.JobName == "" {
		http.Error(w, "workflow has not been run", http.StatusNotFound)
		return
	}

	status, err := h.runner.Status(ctx, wf.JobName)
	if err != nil {
		http.Error(w, "could not get workflow status", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(statusResponse{Status: string(status)})
}
```

Modify `internal/api/router.go` to register the route:

```go
	mux.HandleFunc("GET /workflows/run", h.run)
	mux.HandleFunc("GET /workflows/status", h.status)
```

- [ ] **Step 4: Run the tests and verify they pass**

Run: `go test ./internal/api/...`
Expected: PASS (10 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/api
git commit -m "feat: add GET /workflows/status endpoint"
```

---

### Task 10: HTTP API — workflow output

**Files:**
- Create: `internal/api/output.go`
- Modify: `internal/api/router.go`
- Test: `internal/api/output_test.go`

**Interfaces:**
- Consumes: `fakeStore`, `fakeRunner{logs, logsErr}`

- [ ] **Step 1: Write the failing tests**

Create `internal/api/output_test.go`:

```go
package api_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prafdin/simple-workflows/internal/api"
	"github.com/prafdin/simple-workflows/internal/domain"
)

func TestOutputReturnsRunnerLogsForActiveWorkflow(t *testing.T) {
	store := newFakeStore()
	if err := store.Save(context.Background(), domain.Workflow{Name: "example", Image: "img:v1", JobName: "example-1"}); err != nil {
		t.Fatalf("could not seed workflow: %v", err)
	}
	runner := &fakeRunner{logs: "hello from task"}
	server := httptest.NewServer(api.NewRouter(store, runner))
	defer server.Close()

	resp, err := http.Get(server.URL + "/workflows/output?name=example")
	if err != nil {
		t.Fatalf("could not call output endpoint: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("could not read response body: %v", err)
	}

	if string(body) != "hello from task" {
		t.Fatalf("got body %q, want %q", string(body), "hello from task")
	}
}

func TestOutputFailsWithNotFoundForUnknownWorkflow(t *testing.T) {
	store := newFakeStore()
	server := httptest.NewServer(api.NewRouter(store, &fakeRunner{}))
	defer server.Close()

	resp, err := http.Get(server.URL + "/workflows/output?name=missing")
	if err != nil {
		t.Fatalf("could not call output endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}
```

- [ ] **Step 2: Run the tests and verify they fail**

Run: `go test ./internal/api/...`
Expected: FAIL — `GET /workflows/output` returns 404 (route not registered)

- [ ] **Step 3: Implement the output handler**

Create `internal/api/output.go`:

```go
package api

import (
	"errors"
	"io"
	"net/http"

	"github.com/prafdin/simple-workflows/internal/domain"
)

func (h *handler) output(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	ctx := r.Context()

	wf, err := h.store.Get(ctx, name)
	if errors.Is(err, domain.ErrNotFound) {
		http.Error(w, "workflow not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "could not load workflow", http.StatusBadGateway)
		return
	}
	if wf.JobName == "" {
		http.Error(w, "workflow has not been run", http.StatusNotFound)
		return
	}

	logs, err := h.runner.Logs(ctx, wf.JobName)
	if err != nil {
		http.Error(w, "could not get workflow output", http.StatusBadGateway)
		return
	}
	defer logs.Close()

	w.Header().Set("Content-Type", "text/plain")
	io.Copy(w, logs)
}
```

Modify `internal/api/router.go` to register the route:

```go
	mux.HandleFunc("GET /workflows/status", h.status)
	mux.HandleFunc("GET /workflows/output", h.output)
```

- [ ] **Step 4: Run the tests and verify they pass**

Run: `go test ./internal/api/...`
Expected: PASS (12 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/api
git commit -m "feat: add GET /workflows/output endpoint"
```

---

### Task 11: Server entrypoint

**Files:**
- Create: `cmd/server/main.go`
- Test: `cmd/server/main_test.go`

**Interfaces:**
- Consumes: `mongostore.New`, `k8sexec.New`, `api.NewRouter`

Note: `main` connects to a real MongoDB and a real (or in-cluster) Kubernetes API at startup, so it can't be exercised by an automated test without those running. The only custom logic worth a unit test is the env-var fallback helper; everything else is direct wiring of already-tested packages.

- [ ] **Step 1: Add dependency**

Run: `go get k8s.io/client-go/tools/clientcmd@latest`

- [ ] **Step 2: Write the failing tests**

Create `cmd/server/main_test.go`:

```go
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
```

- [ ] **Step 3: Run the tests and verify they fail**

Run: `go test ./cmd/server/...`
Expected: FAIL — `cmd/server` package does not exist / `undefined: getenv`

- [ ] **Step 4: Implement the entrypoint**

Create `cmd/server/main.go`:

```go
package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/prafdin/simple-workflows/internal/api"
	"github.com/prafdin/simple-workflows/internal/k8sexec"
	"github.com/prafdin/simple-workflows/internal/mongostore"
)

func main() {
	ctx := context.Background()

	mongoURI := getenv("MONGO_URI", "mongodb://localhost:27017")
	mongoDatabase := getenv("MONGO_DATABASE", "simple_workflows")
	namespace := getenv("K8S_NAMESPACE", "default")
	listenAddr := getenv("LISTEN_ADDR", ":8080")

	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatalf("could not connect to mongodb: %v", err)
	}
	collection := mongoClient.Database(mongoDatabase).Collection("workflows")
	store := mongostore.New(collection)

	k8sConfig, err := buildKubeConfig()
	if err != nil {
		log.Fatalf("could not build kubernetes config: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		log.Fatalf("could not build kubernetes clientset: %v", err)
	}
	runner := k8sexec.New(clientset, namespace)

	router := api.NewRouter(store, runner)

	log.Printf("listening on %s", listenAddr)
	if err := http.ListenAndServe(listenAddr, router); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

func buildKubeConfig() (*rest.Config, error) {
	if config, err := rest.InClusterConfig(); err == nil {
		return config, nil
	}
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{}).ClientConfig()
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
```

- [ ] **Step 5: Run the tests and verify they pass**

Run: `go test ./cmd/server/...`
Expected: PASS (2 tests)

- [ ] **Step 6: Build and tidy**

Run: `go build ./... && go mod tidy`
Expected: builds cleanly with no errors

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum cmd/server
git commit -m "feat: wire server entrypoint"
```

---

### Task 12: Dockerfile

**Files:**
- Create: `Dockerfile`

- [ ] **Step 1: Write the Dockerfile**

Create `Dockerfile`:

```dockerfile
FROM golang:1.23 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/server /server
ENTRYPOINT ["/server"]
```

- [ ] **Step 2: Verify the image builds**

Run: `docker build -t simple-workflows:test .`
Expected: build completes successfully (exit code 0)

- [ ] **Step 3: Commit**

```bash
git add Dockerfile
git commit -m "build: add Dockerfile"
```

---

### Task 13: Base Kustomize manifests

**Files:**
- Create: `deploy/base/deployment.yaml`
- Create: `deploy/base/service.yaml`
- Create: `deploy/base/serviceaccount.yaml`
- Create: `deploy/base/role.yaml`
- Create: `deploy/base/rolebinding.yaml`
- Create: `deploy/base/configmap.yaml`
- Create: `deploy/base/httproute.yaml`
- Create: `deploy/base/kustomization.yaml`

- [ ] **Step 1: Write the base manifests**

Create `deploy/base/serviceaccount.yaml`:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: simple-workflows
```

Create `deploy/base/role.yaml`:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: simple-workflows
rules:
  - apiGroups: ["batch"]
    resources: ["jobs"]
    verbs: ["create", "get", "list", "watch"]
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list"]
  - apiGroups: [""]
    resources: ["pods/log"]
    verbs: ["get"]
```

Create `deploy/base/rolebinding.yaml`:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: simple-workflows
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: simple-workflows
subjects:
  - kind: ServiceAccount
    name: simple-workflows
```

Create `deploy/base/configmap.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: simple-workflows-config
data:
  MONGO_URI: "mongodb://localhost:27017"
  MONGO_DATABASE: "simple_workflows"
```

Create `deploy/base/deployment.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: simple-workflows
spec:
  replicas: 1
  selector:
    matchLabels:
      app: simple-workflows
  template:
    metadata:
      labels:
        app: simple-workflows
    spec:
      serviceAccountName: simple-workflows
      containers:
        - name: simple-workflows
          image: simple-workflows:latest
          ports:
            - containerPort: 8080
          envFrom:
            - configMapRef:
                name: simple-workflows-config
          env:
            - name: K8S_NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
```

Create `deploy/base/service.yaml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: simple-workflows
spec:
  selector:
    app: simple-workflows
  ports:
    - port: 8080
      targetPort: 8080
```

Create `deploy/base/httproute.yaml`:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: simple-workflows
spec:
  parentRefs:
    - name: placeholder-gateway
  rules:
    - backendRefs:
        - name: simple-workflows
          port: 8080
```

Create `deploy/base/kustomization.yaml`:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - serviceaccount.yaml
  - role.yaml
  - rolebinding.yaml
  - configmap.yaml
  - deployment.yaml
  - service.yaml
  - httproute.yaml
```

- [ ] **Step 2: Verify the base builds**

Run: `kubectl kustomize deploy/base`
Expected: renders valid YAML with no errors

- [ ] **Step 3: Commit**

```bash
git add deploy/base
git commit -m "deploy: add base kustomize manifests"
```

---

### Task 14: Sample overlay

**Files:**
- Create: `deploy/overlays/sample/configmap-patch.yaml`
- Create: `deploy/overlays/sample/httproute-patch.yaml`
- Create: `deploy/overlays/sample/kustomization.yaml`

- [ ] **Step 1: Write the overlay**

Create `deploy/overlays/sample/configmap-patch.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: simple-workflows-config
data:
  MONGO_URI: "mongodb://mongo.default.svc.cluster.local:27017"
  MONGO_DATABASE: "simple_workflows"
```

Create `deploy/overlays/sample/httproute-patch.yaml`:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: simple-workflows
spec:
  parentRefs:
    - name: my-gateway
```

Create `deploy/overlays/sample/kustomization.yaml`:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - ../../base
patches:
  - path: configmap-patch.yaml
  - path: httproute-patch.yaml
```

- [ ] **Step 2: Verify the overlay builds**

Run: `kubectl kustomize deploy/overlays/sample`
Expected: renders valid YAML with the patched `MONGO_URI` and `parentRefs.name`, no errors

- [ ] **Step 3: Commit**

```bash
git add deploy/overlays
git commit -m "deploy: add sample kustomize overlay"
```
