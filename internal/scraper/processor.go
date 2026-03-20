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
}

func NewProcessor(client *MetricsClient) *Processor {
	return &Processor{
		client: client,
	}
}

func (p *Processor) Process(ctx context.Context, task *scrapetask.ScrapeTask) (*Report, error) {
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
		metrics, err := p.client.GetTargetMetrics(ctx, address)
		if err != nil {
			logger.Error("Failed to get target metrics", zap.Error(err))
			continue
		}

		for _, metric := range metrics {
			handleMetricFamily(statistic, metric)
		}
		totalProcessed++
	}

	if totalProcessed == 0 {
		return nil, errors.New("failed to get target metrics")
	}

	return statistic, nil
}

const (
	maxMetricNameLen    = 100
	maxLabelNameLen     = 100
	maxLabelValueLen    = 200
	maxLabelCardinality = 20
	maxHistogramBuckets = 50
)

func handleMetricFamily(stats *Report, mf *dto.MetricFamily) {
	if mf == nil {
		return
	}

	d := &stats.Details
	metricName := mf.GetName()

	// --- длина имени метрики
	if l := len(metricName); l > maxMetricNameLen {
		d.addMetricNameViolation(MetricNameViolation{
			MetricName: metricName,
			Length:     l,
		})
	}

	// --- кардинальность
	if cardinality := len(mf.Metric); cardinality > maxLabelCardinality {
		d.addCardinalityViolation(CardinalityViolation{
			MetricName: metricName,
			Value:      cardinality,
		})
	}

	// --- обход метрик
	for _, m := range mf.Metric {
		if m == nil {
			continue
		}

		// --- лейблы
		for _, label := range m.Label {
			if label == nil {
				continue
			}

			labelName := label.GetName()
			labelValue := label.GetValue()

			if l := len(labelName); l > maxLabelNameLen {
				d.addLabelNameViolation(LabelNameViolation{
					MetricName: metricName,
					LabelName:  labelName,
					Length:     l,
				})
			}

			if l := len(labelValue); l > maxLabelValueLen {
				d.addLabelValueViolation(LabelValueViolation{
					MetricName: metricName,
					LabelName:  labelName,
					Value:      labelValue,
					Length:     l,
				})
			}
		}

		// --- гистограмма
		if h := m.GetHistogram(); h != nil {
			if buckets := len(h.Bucket); buckets > maxHistogramBuckets {
				d.addHistogramBucketsViolation(HistogramBucketsViolation{
					MetricName: metricName,
					Buckets:    buckets,
				})
			}
		}
	}
}
