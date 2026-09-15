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
