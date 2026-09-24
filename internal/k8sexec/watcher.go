package k8sexec

import (
	"context"
	"errors"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/prafdin/simple-workflows/internal/domain"
)

// Watcher follows workflow Jobs through an informer and reports each run
// that reaches a terminal state exactly once while the process lives.
type Watcher struct {
	factory  informers.SharedInformerFactory
	informer cache.SharedIndexInformer
}

func NewWatcher(clientset kubernetes.Interface, namespace string, metrics domain.Metrics) (*Watcher, error) {
	factory := informers.NewSharedInformerFactoryWithOptions(
		clientset,
		0,
		informers.WithNamespace(namespace),
		informers.WithTweakListOptions(func(o *metav1.ListOptions) { o.LabelSelector = workflowLabel }),
	)
	informer := factory.Batch().V1().Jobs().Informer()
	_, err := informer.AddEventHandler(cache.ResourceEventHandlerDetailedFuncs{
		AddFunc: func(obj interface{}, initial bool) {
			if status, done := outcome(obj.(*batchv1.Job)); done && !initial {
				metrics.RunCompleted(status)
			}
		},
		UpdateFunc: func(previous, current interface{}) {
			_, was := outcome(previous.(*batchv1.Job))
			if status, done := outcome(current.(*batchv1.Job)); done && !was {
				metrics.RunCompleted(status)
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
