package metrics

import (
	"context"
	"log"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prafdin/simple-workflows/internal/domain"
)

// population is a gauge collector that asks the store how many workflows
// exist at scrape time, so the value survives process restarts.
type population struct {
	store domain.WorkflowStore
	desc  *prometheus.Desc
}

func newPopulation(store domain.WorkflowStore) population {
	return population{
		store: store,
		desc:  prometheus.NewDesc("simple_workflows_workflows", "Workflows currently stored.", nil, nil),
	}
}

func (p population) Describe(ch chan<- *prometheus.Desc) {
	ch <- p.desc
}

func (p population) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count, err := p.store.Count(ctx)
	if err != nil {
		log.Printf("could not count workflows for metrics: %v", err)
		return
	}
	ch <- prometheus.MustNewConstMetric(p.desc, prometheus.GaugeValue, float64(count))
}
