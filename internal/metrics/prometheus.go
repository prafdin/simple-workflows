package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/prafdin/simple-workflows/internal/domain"
)

// Prometheus counts workflow lifecycle events and exposes them, together with
// the number of stored workflows, on a private Prometheus registry.
type Prometheus struct {
	created   prometheus.Counter
	started   prometheus.Counter
	completed *prometheus.CounterVec
	registry  *prometheus.Registry
}

func New(store domain.WorkflowStore) *Prometheus {
	p := &Prometheus{
		created: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "simple_workflows_workflows_created_total",
			Help: "Workflow manifests accepted by POST /workflows.",
		}),
		started: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "simple_workflows_runs_started_total",
			Help: "Workflow runs started by GET /workflows/run.",
		}),
		completed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "simple_workflows_runs_completed_total",
			Help: "Workflow runs that reached a terminal state, by outcome.",
		}, []string{"status"}),
		registry: prometheus.NewRegistry(),
	}
	p.completed.WithLabelValues(string(domain.StatusSucceeded))
	p.completed.WithLabelValues(string(domain.StatusFailed))
	p.registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		p.created,
		p.started,
		p.completed,
		newPopulation(store),
	)
	return p
}

func (p *Prometheus) WorkflowCreated() {
	p.created.Inc()
}

func (p *Prometheus) RunStarted() {
	p.started.Inc()
}

func (p *Prometheus) RunCompleted(status domain.Status) {
	p.completed.WithLabelValues(string(status)).Inc()
}

func (p *Prometheus) Handler() http.Handler {
	return promhttp.HandlerFor(p.registry, promhttp.HandlerOpts{})
}
