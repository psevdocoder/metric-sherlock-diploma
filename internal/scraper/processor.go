package scraper

import (
	"context"

	"git.server.lan/maksim/metric-sherlock-diploma/internal/scrapetask"
)

type Processor struct {
	client *MetricsClient
}

func NewProcessor(client *MetricsClient) *Processor {
	return &Processor{
		client: client,
	}
}

func (p *Processor) Process(ctx context.Context, task *scrapetask.ScrapeTask) (*Statistic, error) {
	metrics, err := p.client.GetTargetMetrics(ctx, task.Address)
	if err != nil {
		return nil, err
	}

	for _, metric := range metrics {
		_ = metric
	}

	stats := &Statistic{}

	return stats, nil
}
