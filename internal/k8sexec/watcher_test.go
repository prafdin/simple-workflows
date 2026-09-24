package k8sexec_test

import (
	"context"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/prafdin/simple-workflows/internal/domain"
	"github.com/prafdin/simple-workflows/internal/k8sexec"
)

type recorder struct {
	outcome chan domain.Status
}

func (r recorder) WorkflowCreated() {}

func (r recorder) RunStarted() {}

func (r recorder) RunCompleted(status domain.Status) {
	r.outcome <- status
}

func workflowJob(name string, conditions ...batchv1.JobCondition) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "lab",
			Labels:    map[string]string{"simple-workflows/workflow": "nebula"},
		},
		Status: batchv1.JobStatus{Conditions: conditions},
	}
}

func condition(kind batchv1.JobConditionType) batchv1.JobCondition {
	return batchv1.JobCondition{Type: kind, Status: corev1.ConditionTrue}
}

func watch(t *testing.T, clientset *fake.Clientset) recorder {
	t.Helper()
	rec := recorder{outcome: make(chan domain.Status, 8)}
	watcher, err := k8sexec.NewWatcher(clientset, "lab", rec)
	if err != nil {
		t.Fatalf("could not create watcher: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := watcher.Run(ctx, 5*time.Second); err != nil {
		t.Fatalf("could not run watcher: %v", err)
	}
	return rec
}

func seed(t *testing.T, clientset *fake.Clientset, job *batchv1.Job) {
	t.Helper()
	if _, err := clientset.BatchV1().Jobs("lab").Create(context.Background(), job, metav1.CreateOptions{}); err != nil {
		t.Fatalf("could not create job %q: %v", job.Name, err)
	}
}

func finish(t *testing.T, clientset *fake.Clientset, job *batchv1.Job) {
	t.Helper()
	if _, err := clientset.BatchV1().Jobs("lab").UpdateStatus(context.Background(), job, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("could not update job %q: %v", job.Name, err)
	}
}

func first(t *testing.T, rec recorder) domain.Status {
	t.Helper()
	select {
	case status := <-rec.outcome:
		return status
	case <-time.After(5 * time.Second):
		t.Fatalf("no completed run was reported within 5s")
		return ""
	}
}

func TestWatcherReportsSucceededWhenJobCompletes(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	seed(t, clientset, workflowJob("nebula-501"))
	rec := watch(t, clientset)

	finish(t, clientset, workflowJob("nebula-501", condition(batchv1.JobComplete)))

	if got := first(t, rec); got != domain.StatusSucceeded {
		t.Fatalf("got status %q, want %q", got, domain.StatusSucceeded)
	}
}

func TestWatcherReportsFailedWhenJobFails(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	seed(t, clientset, workflowJob("nebula-502"))
	rec := watch(t, clientset)

	finish(t, clientset, workflowJob("nebula-502", condition(batchv1.JobFailed)))

	if got := first(t, rec); got != domain.StatusFailed {
		t.Fatalf("got status %q, want %q", got, domain.StatusFailed)
	}
}

func TestWatcherReportsJobCreatedAlreadyFinished(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	rec := watch(t, clientset)

	seed(t, clientset, workflowJob("nebula-503", condition(batchv1.JobFailed)))

	if got := first(t, rec); got != domain.StatusFailed {
		t.Fatalf("got status %q, want %q", got, domain.StatusFailed)
	}
}

func TestWatcherIgnoresJobFinishedBeforeStart(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	seed(t, clientset, workflowJob("nebula-504", condition(batchv1.JobComplete)))
	rec := watch(t, clientset)

	seed(t, clientset, workflowJob("nebula-505", condition(batchv1.JobFailed)))

	if got := first(t, rec); got != domain.StatusFailed {
		t.Fatalf("got status %q first, want sentinel %q; job finished before start was counted", got, domain.StatusFailed)
	}
}

func TestWatcherIgnoresUpdateOfFinishedJob(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	seed(t, clientset, workflowJob("nebula-506", condition(batchv1.JobComplete)))
	rec := watch(t, clientset)
	touched := workflowJob("nebula-506", condition(batchv1.JobComplete))
	touched.Annotations = map[string]string{"touched": "yes"}
	if _, err := clientset.BatchV1().Jobs("lab").Update(context.Background(), touched, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("could not touch job: %v", err)
	}

	seed(t, clientset, workflowJob("nebula-507", condition(batchv1.JobFailed)))

	if got := first(t, rec); got != domain.StatusFailed {
		t.Fatalf("got status %q first, want sentinel %q; update of finished job was counted", got, domain.StatusFailed)
	}
}
