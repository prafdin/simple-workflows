package k8sexec_test

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/prafdin/simple-workflows/internal/domain"
	"github.com/prafdin/simple-workflows/internal/k8sexec"
)

var origin = time.Date(2026, 3, 14, 9, 26, 53, 0, time.UTC)

func moment(seconds int) metav1.Time {
	return metav1.NewTime(origin.Add(time.Duration(seconds) * time.Second))
}

func tracedJob(name, traceparent string, kind batchv1.JobConditionType) *batchv1.Job {
	job := workflowJob(name)
	job.CreationTimestamp = moment(0)
	job.Annotations = map[string]string{"simple-workflows/traceparent": traceparent}
	job.Spec.Template.Spec.Containers = []corev1.Container{{Name: "task", Image: "registry.local/pulsar:2.4"}}
	if kind != "" {
		job.Status.Conditions = []batchv1.JobCondition{{Type: kind, Status: corev1.ConditionTrue, LastTransitionTime: moment(19)}}
	}
	return job
}

func taskPod(job string, exit int32) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: job + "-x7k2p", Namespace: "lab", Labels: map[string]string{"job-name": job}},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionTrue, LastTransitionTime: moment(1)}},
			ContainerStatuses: []corev1.ContainerStatus{{Name: "task", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
				ExitCode: exit, Reason: "Completed", StartedAt: moment(7), FinishedAt: moment(18),
			}}}},
		},
	}
}

func traced(t *testing.T, clientset *fake.Clientset) *tracetest.SpanRecorder {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	watcher, err := k8sexec.NewWatcher(clientset, "lab", recorder2metrics{}, provider)
	if err != nil {
		t.Fatalf("could not create watcher: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := watcher.Run(ctx, 5*time.Second); err != nil {
		t.Fatalf("could not run watcher: %v", err)
	}
	return recorder
}

type recorder2metrics struct{}

func (recorder2metrics) WorkflowCreated()             {}
func (recorder2metrics) RunStarted()                  {}
func (recorder2metrics) RunCompleted(_ domain.Status) {}

func spans(t *testing.T, recorder *tracetest.SpanRecorder) map[string]sdktrace.ReadOnlySpan {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		named := map[string]sdktrace.ReadOnlySpan{}
		for _, span := range recorder.Ended() {
			named[span.Name()] = span
		}
		if _, ok := named["workflow.run"]; ok {
			return named
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no workflow.run span was recorded within 5s")
	return nil
}

const parent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

func finishRun(t *testing.T, clientset *fake.Clientset, name string, kind batchv1.JobConditionType, pod *corev1.Pod) *tracetest.SpanRecorder {
	t.Helper()
	seed(t, clientset, tracedJob(name, parent, ""))
	if pod != nil {
		if _, err := clientset.CoreV1().Pods("lab").Create(context.Background(), pod, metav1.CreateOptions{}); err != nil {
			t.Fatalf("could not create pod: %v", err)
		}
	}
	recorder := traced(t, clientset)
	finish(t, clientset, tracedJob(name, parent, kind))
	return recorder
}

func TestWatcherRecordsRunUnderCallerTrace(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	recorder := finishRun(t, clientset, "pulsar-11", batchv1.JobComplete, taskPod("pulsar-11", 0))

	if got := spans(t, recorder)["workflow.run"].SpanContext().TraceID().String(); got != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("got trace id %s, want the caller's 4bf92f3577b34da6a3ce929d0e0e4736", got)
	}
}

func TestWatcherBackdatesRunToJobLifetime(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	recorder := finishRun(t, clientset, "pulsar-12", batchv1.JobComplete, taskPod("pulsar-12", 0))
	run := spans(t, recorder)["workflow.run"]

	if got := run.EndTime().Sub(run.StartTime()); got != 19*time.Second {
		t.Fatalf("got run duration %s, want 19s from creation to completion", got)
	}
}

func TestWatcherRecordsEveryStageOfRun(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	recorder := finishRun(t, clientset, "pulsar-13", batchv1.JobComplete, taskPod("pulsar-13", 0))
	named := spans(t, recorder)

	for _, stage := range []string{"schedule", "start", "execute", "complete"} {
		if _, ok := named[stage]; !ok {
			t.Fatalf("stage %q was not recorded, got %d spans", stage, len(named))
		}
	}
}

func TestWatcherTimesExecuteStageByContainer(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	recorder := finishRun(t, clientset, "pulsar-14", batchv1.JobComplete, taskPod("pulsar-14", 0))
	execute := spans(t, recorder)["execute"]

	if got := execute.EndTime().Sub(execute.StartTime()); got != 11*time.Second {
		t.Fatalf("got execute duration %s, want 11s of container runtime", got)
	}
}

func TestWatcherMarksFailedRunAsError(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	recorder := finishRun(t, clientset, "pulsar-15", batchv1.JobFailed, taskPod("pulsar-15", 3))

	if got := spans(t, recorder)["workflow.run"].Status().Code; got != codes.Error {
		t.Fatalf("got run status %v, want Error for a failed job", got)
	}
}

func TestWatcherRecordsRunWithoutPod(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	recorder := finishRun(t, clientset, "pulsar-16", batchv1.JobComplete, nil)

	if got := len(spans(t, recorder)); got != 1 {
		t.Fatalf("got %d spans without a pod, want only workflow.run", got)
	}
}

func TestWatcherStartsRootRunForJobWithoutTraceparent(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	seed(t, clientset, tracedJob("pulsar-17", "", ""))
	recorder := traced(t, clientset)
	finish(t, clientset, tracedJob("pulsar-17", "", batchv1.JobComplete))

	if got := spans(t, recorder)["workflow.run"].Parent().IsValid(); got {
		t.Fatalf("run without traceparent has a parent, want a root span")
	}
}

func skewedRun(t *testing.T, name string) map[string]sdktrace.ReadOnlySpan {
	t.Helper()
	clientset := fake.NewSimpleClientset()
	seed(t, clientset, tracedJob(name, parent, ""))
	if _, err := clientset.CoreV1().Pods("lab").Create(context.Background(), taskPod(name, 0), metav1.CreateOptions{}); err != nil {
		t.Fatalf("could not create pod: %v", err)
	}
	recorder := traced(t, clientset)
	done := tracedJob(name, parent, batchv1.JobComplete)
	done.Status.Conditions[0].LastTransitionTime = moment(16)
	finish(t, clientset, done)
	return spans(t, recorder)
}

func TestWatcherNeverEndsStageBeforeItStarts(t *testing.T) {
	complete := skewedRun(t, "pulsar-18")["complete"]

	if complete.EndTime().Before(complete.StartTime()) {
		t.Fatalf("complete stage ends at %s before it starts at %s", complete.EndTime(), complete.StartTime())
	}
}

func TestWatcherKeepsStagesInsideRun(t *testing.T) {
	named := skewedRun(t, "pulsar-19")
	run := named["workflow.run"]

	for name, span := range named {
		if span.EndTime().After(run.EndTime()) {
			t.Fatalf("stage %q ends at %s after run ends at %s", name, span.EndTime(), run.EndTime())
		}
	}
}

func TestWatcherSkipsPodLookupWhenRunIsNotTraced(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	seed(t, clientset, workflowJob("pulsar-20"))
	rec := watch(t, clientset)
	finish(t, clientset, workflowJob("pulsar-20", condition(batchv1.JobComplete)))
	first(t, rec)
	seed(t, clientset, workflowJob("pulsar-21", condition(batchv1.JobFailed)))
	first(t, rec)
	lookups := 0
	for _, action := range clientset.Actions() {
		if action.GetVerb() == "list" && action.GetResource().Resource == "pods" {
			lookups++
		}
	}

	if lookups != 0 {
		t.Fatalf("got %d pod lookups with a no-op tracer, want 0", lookups)
	}
}
