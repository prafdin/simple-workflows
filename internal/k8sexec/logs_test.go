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
