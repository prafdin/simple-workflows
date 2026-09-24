package k8sexec_test

import (
	"context"
	"strings"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
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

func TestRunStoresTraceparentOfCallerSpanOnJob(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	runner := k8sexec.New(clientset, "lab")
	ctx, span := sdktrace.NewTracerProvider().Tracer("probe").Start(context.Background(), "request")
	defer span.End()

	name, err := runner.Run(ctx, domain.Workflow{Name: "quasar", Image: "img:q7"})
	if err != nil {
		t.Fatalf("could not run workflow: %v", err)
	}
	job, err := clientset.BatchV1().Jobs("lab").Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("could not get job: %v", err)
	}

	if got := job.Annotations["simple-workflows/traceparent"]; !strings.Contains(got, span.SpanContext().TraceID().String()) {
		t.Fatalf("got traceparent %q, want it to carry trace id %s", got, span.SpanContext().TraceID())
	}
}
