package k8sexec

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/prafdin/simple-workflows/internal/domain"
)

// chronicle turns a finished workflow Job and its pod into the spans of one
// run, back-dated to the moments Kubernetes recorded for each stage.
type chronicle struct {
	tracer trace.Tracer
}

func (c chronicle) record(job *batchv1.Job, status domain.Status, lookup func() *corev1.Pod) {
	carrier := propagation.MapCarrier{}
	for _, key := range (propagation.TraceContext{}).Fields() {
		if value, ok := job.Annotations[annotationPrefix+key]; ok {
			carrier[key] = value
		}
	}
	ctx, run := c.tracer.Start(
		propagation.TraceContext{}.Extract(context.Background(), carrier),
		"workflow.run",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithTimestamp(job.CreationTimestamp.Time),
		trace.WithAttributes(
			attribute.String("workflow.name", job.Labels[workflowLabel]),
			attribute.String("workflow.image", image(job)),
			attribute.String("k8s.job.name", job.Name),
			attribute.String("workflow.status", string(status)),
		),
	)
	finished := ending(job)
	if run.IsRecording() {
		if pod := lookup(); pod != nil {
			finished = c.stages(ctx, job, pod, finished)
		}
	}
	if status == domain.StatusFailed {
		run.SetStatus(codes.Error, "workflow run failed")
	}
	run.End(trace.WithTimestamp(finished))
}

func (c chronicle) stages(ctx context.Context, job *batchv1.Job, pod *corev1.Pod, finished time.Time) time.Time {
	scheduled := scheduling(pod)
	latest := later(finished, c.stage(ctx, "schedule", job.CreationTimestamp.Time, scheduled))
	state := termination(pod)
	if state == nil {
		return latest
	}
	latest = later(latest, c.stage(ctx, "start", scheduled, state.StartedAt.Time))
	latest = later(latest, c.stage(ctx, "execute", state.StartedAt.Time, state.FinishedAt.Time,
		attribute.Int("container.exit_code", int(state.ExitCode)),
		attribute.String("container.reason", state.Reason),
	))
	return later(latest, c.stage(ctx, "complete", state.FinishedAt.Time, finished))
}

func (c chronicle) stage(ctx context.Context, name string, from, to time.Time, attrs ...attribute.KeyValue) time.Time {
	if from.IsZero() || to.IsZero() {
		return time.Time{}
	}
	to = later(to, from)
	_, span := c.tracer.Start(ctx, name, trace.WithTimestamp(from), trace.WithAttributes(attrs...))
	if name == "execute" && failed(attrs) {
		span.SetStatus(codes.Error, "task container exited with non-zero code")
	}
	span.End(trace.WithTimestamp(to))
	return to
}

func later(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func failed(attrs []attribute.KeyValue) bool {
	for _, attr := range attrs {
		if attr.Key == "container.exit_code" && attr.Value.AsInt64() != 0 {
			return true
		}
	}
	return false
}

func image(job *batchv1.Job) string {
	for _, container := range job.Spec.Template.Spec.Containers {
		if container.Name == "task" {
			return container.Image
		}
	}
	return ""
}

func ending(job *batchv1.Job) time.Time {
	if job.Status.CompletionTime != nil {
		return job.Status.CompletionTime.Time
	}
	for _, cond := range job.Status.Conditions {
		if (cond.Type == batchv1.JobComplete || cond.Type == batchv1.JobFailed) && cond.Status == corev1.ConditionTrue && !cond.LastTransitionTime.IsZero() {
			return cond.LastTransitionTime.Time
		}
	}
	return time.Now()
}

func scheduling(pod *corev1.Pod) time.Time {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionTrue {
			return cond.LastTransitionTime.Time
		}
	}
	return time.Time{}
}

func termination(pod *corev1.Pod) *corev1.ContainerStateTerminated {
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == "task" {
			return status.State.Terminated
		}
	}
	return nil
}
