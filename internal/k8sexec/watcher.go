package k8sexec

import (
	"context"
	"errors"
	"log"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"go.opentelemetry.io/otel/trace"

	"github.com/prafdin/simple-workflows/internal/domain"
)

// Watcher follows workflow Jobs through an informer and reports each run
// that reaches a terminal state exactly once while the process lives.
type Watcher struct {
	factory  informers.SharedInformerFactory
	informer cache.SharedIndexInformer
}

func NewWatcher(clientset kubernetes.Interface, namespace string, metrics domain.Metrics, provider trace.TracerProvider) (*Watcher, error) {
	factory := informers.NewSharedInformerFactoryWithOptions(
		clientset,
		0,
		informers.WithNamespace(namespace),
		informers.WithTweakListOptions(func(o *metav1.ListOptions) { o.LabelSelector = workflowLabel }),
	)
	informer := factory.Batch().V1().Jobs().Informer()
	history := chronicle{tracer: provider.Tracer("github.com/prafdin/simple-workflows/internal/k8sexec")}
	complete := func(job *batchv1.Job, status domain.Status) {
		metrics.RunCompleted(status)
		history.record(job, status, func() *corev1.Pod { return podOf(clientset, job) })
	}
	_, err := informer.AddEventHandler(cache.ResourceEventHandlerDetailedFuncs{
		AddFunc: func(obj interface{}, initial bool) {
			job := obj.(*batchv1.Job)
			if status, done := outcome(job); done && !initial {
				complete(job, status)
			}
		},
		UpdateFunc: func(previous, current interface{}) {
			_, was := outcome(previous.(*batchv1.Job))
			job := current.(*batchv1.Job)
			if status, done := outcome(job); done && !was {
				complete(job, status)
			}
		},
	})
	if err != nil {
		return nil, err
	}
	return &Watcher{factory: factory, informer: informer}, nil
}

func (w *Watcher) Run(ctx context.Context, timeout time.Duration) error {
	w.factory.Start(ctx.Done())
	syncCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if !cache.WaitForCacheSync(syncCtx.Done(), w.informer.HasSynced) {
		return errors.New("job informer cache did not sync in time")
	}
	return nil
}

func podOf(clientset kubernetes.Interface, job *batchv1.Job) *corev1.Pod {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pods, err := clientset.CoreV1().Pods(job.Namespace).List(ctx, metav1.ListOptions{LabelSelector: "job-name=" + job.Name})
	if err != nil {
		log.Printf("could not list pods of job %s for tracing: %v", job.Name, err)
		return nil
	}
	if len(pods.Items) == 0 {
		return nil
	}
	return &pods.Items[0]
}
