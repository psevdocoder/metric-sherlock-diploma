package scraper

import (
	"context"
	"errors"

	"git.server.lan/maksim/metric-sherlock-diploma/internal/scrapetask"
	"git.server.lan/pkg/zaplogger/logger"
	dto "github.com/prometheus/client_model/go"
	"go.uber.org/zap"
)

type Processor struct {
	client *MetricsClient
	limits LimitsConfig
}

func NewProcessor(client *MetricsClient, limitsConfig LimitsConfig) *Processor {
	return &Processor{
		client: client,
		limits: limitsConfig,
	}
}

func (p *Processor) Process(ctx context.Context, task *scrapetask.ScrapeTask) (*Report, []*TargetGroup, error) {
	logger.Debug(
		"Processing target group",
		zap.String("target_group", task.TargetGroup),
		zap.Any("addresses", task.Addresses),
	)

	statistic := &Report{
		TargetGroup: task.TargetGroup,
		Env:         task.Env,
		Cluster:     task.Cluster,
		TeamName:    task.TeamName,
	}

	totalProcessed := 0

	for _, address := range task.Addresses {
		metrics, bytesWeight, err := p.client.GetTargetMetrics(ctx, address)
		if err != nil {
			logger.Error("Failed to get target metrics", zap.Error(err))
			continue
		}

		for _, metric := range metrics {
			p.handleMetricFamily(statistic, metric)
		}
		totalProcessed++

		if p.limits.MaxBytesWeight < bytesWeight && statistic.Details.ResponseWeight < bytesWeight {
			statistic.Details.ResponseWeight = bytesWeight
		}
	}

	if totalProcessed == 0 {
		return nil, nil, errors.New("failed to get target metrics")
	}

	tg := &TargetGroup{
		Name:     task.TargetGroup,
		Env:      task.Env,
		Cluster:  task.Cluster,
		Job:      task.Job,
		TeamName: task.TeamName,
	}

	return statistic, []*TargetGroup{tg}, nil
}

func (p *Processor) handleMetricFamily(stats *Report, mf *dto.MetricFamily) {
	if mf == nil {
		return
	}

	d := &stats.Details
	metricName := mf.GetName()

	if l := len(metricName); l > p.limits.MaxMetricNameLen {
		d.addMetricNameViolation(MetricNameViolation{
			MetricName: metricName,
			Length:     l,
		})
	}

	if cardinality := len(mf.Metric); cardinality > p.limits.MaxMetricCardinality {
		d.addCardinalityViolation(CardinalityViolation{
			MetricName: metricName,
			Value:      cardinality,
		})
	}

	for _, m := range mf.Metric {
		if m == nil {
			continue
		}

		for _, label := range m.Label {
			if label == nil {
				continue
			}

			labelName := label.GetName()
			labelValue := label.GetValue()

			if l := len(labelName); l > p.limits.MaxLabelNameLen {
				d.addLabelNameViolation(LabelNameViolation{
					MetricName: metricName,
					LabelName:  labelName,
					Length:     l,
				})
			}

			if l := len(labelValue); l > p.limits.MaxLabelValueLen {
				d.addLabelValueViolation(LabelValueViolation{
					MetricName: metricName,
					LabelName:  labelName,
					Value:      labelValue,
					Length:     l,
				})
			}
		}

		if h := m.GetHistogram(); h != nil {
			if buckets := len(h.Bucket); buckets > p.limits.MaxHistogramBuckets {
				d.addHistogramBucketsViolation(HistogramBucketsViolation{
					MetricName: metricName,
					Buckets:    buckets,
				})
			}
		}
	}
}
