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
