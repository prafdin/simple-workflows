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
