package k8sexec

import (
	"context"
	"fmt"
	"io"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"go.opentelemetry.io/otel/propagation"

	"github.com/prafdin/simple-workflows/internal/domain"
)

const (
	workflowLabel    = "simple-workflows/workflow"
	annotationPrefix = "simple-workflows/"
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
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	annotations := map[string]string{}
	for key, value := range carrier {
		annotations[annotationPrefix+key] = value
	}
	backoffLimit := int32(0)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:        jobName,
			Namespace:   r.namespace,
			Labels:      map[string]string{workflowLabel: domain.NormalizeName(w.Name)},
			Annotations: annotations,
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

func (r *Runner) Status(ctx context.Context, jobName string) (domain.Status, error) {
	job, err := r.clientset.BatchV1().Jobs(r.namespace).Get(ctx, jobName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	if status, done := outcome(job); done {
		return status, nil
	}
	if job.Status.Active > 0 {
		return domain.StatusRunning, nil
	}
	return domain.StatusPending, nil
}

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

func outcome(job *batchv1.Job) (domain.Status, bool) {
	for _, cond := range job.Status.Conditions {
		if cond.Type == batchv1.JobFailed && cond.Status == corev1.ConditionTrue {
			return domain.StatusFailed, true
		}
		if cond.Type == batchv1.JobComplete && cond.Status == corev1.ConditionTrue {
			return domain.StatusSucceeded, true
		}
	}
	return "", false
}
