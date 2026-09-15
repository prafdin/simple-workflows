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
