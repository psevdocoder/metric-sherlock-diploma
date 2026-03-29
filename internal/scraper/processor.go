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
	client            *MetricsClient
	limits            LimitsConfig
	whitelistProvider WhitelistProvider
}

func NewProcessor(client *MetricsClient, limitsConfig LimitsConfig, whitelistProvider WhitelistProvider) *Processor {
	return &Processor{
		client:            client,
		limits:            limitsConfig,
		whitelistProvider: whitelistProvider,
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

	whitelists := NewEffectiveWhitelists()
	if p.whitelistProvider != nil {
		loadedWhitelists, err := p.whitelistProvider.GetEffectiveWhitelists(ctx, task.TargetGroup, task.Env)
		if err != nil {
			logger.Error(
				"Failed to load whitelists",
				zap.String("target_group", task.TargetGroup),
				zap.String("env", task.Env),
				zap.Error(err),
			)
		} else if loadedWhitelists != nil {
			whitelists = loadedWhitelists
		}
	}

	totalProcessed := 0

	for _, address := range task.Addresses {
		metrics, bytesWeight, err := p.client.GetTargetMetrics(ctx, address)
		if err != nil {
			logger.Error("Failed to get target metrics", zap.Error(err))
			continue
		}

		for _, metric := range metrics {
			p.handleMetricFamily(statistic, metric, whitelists)
		}
		totalProcessed++

		if !whitelists.IsCheckDisabled(CheckTypeResponseWeight) {
			if bytesWeight > statistic.maxResponseWeight {
				statistic.maxResponseWeight = bytesWeight
			}

			if p.limits.MaxBytesWeight < bytesWeight && statistic.Details.ResponseWeight < bytesWeight {
				statistic.Details.ResponseWeight = bytesWeight
			}
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

	checks := buildCheckResults(statistic, p.limits, whitelists)
	statistic.Checks = checks
	statistic.Details.Checks = checks
	statistic.Details.Limits = buildCheckLimits(checks)
	statistic.Details.Current = buildCheckCurrent(checks)

	return statistic, []*TargetGroup{tg}, nil
}

func (p *Processor) handleMetricFamily(stats *Report, mf *dto.MetricFamily, whitelists *EffectiveWhitelists) {
	if mf == nil {
		return
	}

	d := &stats.Details
	metricName := mf.GetName()

	if !whitelists.IsCheckDisabled(CheckTypeMetricNameLength) {
		if l := len(metricName); l > stats.maxMetricNameLen {
			stats.maxMetricNameLen = l
		}

		if _, whitelisted, violated := p.metricCheckLimit(metricName, CheckTypeMetricNameLength, int64(p.limits.MaxMetricNameLen), int64(len(metricName)), whitelists); violated {
			d.addMetricNameViolation(MetricNameViolation{
				MetricName:  metricName,
				Length:      len(metricName),
				Whitelisted: whitelisted,
			})
		}
	}

	if !whitelists.IsCheckDisabled(CheckTypeCardinality) {
		if cardinality := len(mf.Metric); cardinality > stats.maxMetricCardinality {
			stats.maxMetricCardinality = cardinality
		}

		if _, whitelisted, violated := p.metricCheckLimit(metricName, CheckTypeCardinality, int64(p.limits.MaxMetricCardinality), int64(len(mf.Metric)), whitelists); violated {
			d.addCardinalityViolation(CardinalityViolation{
				MetricName:  metricName,
				Value:       len(mf.Metric),
				Whitelisted: whitelisted,
			})
		}
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

			if !whitelists.IsCheckDisabled(CheckTypeLabelNameLength) {
				if l := len(labelName); l > stats.maxLabelNameLen {
					stats.maxLabelNameLen = l
				}

				if _, whitelisted, violated := p.metricCheckLimit(metricName, CheckTypeLabelNameLength, int64(p.limits.MaxLabelNameLen), int64(len(labelName)), whitelists); violated {
					d.addLabelNameViolation(LabelNameViolation{
						MetricName:  metricName,
						LabelName:   labelName,
						Length:      len(labelName),
						Whitelisted: whitelisted,
					})
				}
			}

			if !whitelists.IsCheckDisabled(CheckTypeLabelValueLength) {
				if l := len(labelValue); l > stats.maxLabelValueLen {
					stats.maxLabelValueLen = l
				}

				if _, whitelisted, violated := p.metricCheckLimit(metricName, CheckTypeLabelValueLength, int64(p.limits.MaxLabelValueLen), int64(len(labelValue)), whitelists); violated {
					d.addLabelValueViolation(LabelValueViolation{
						MetricName:  metricName,
						LabelName:   labelName,
						Value:       labelValue,
						Length:      len(labelValue),
						Whitelisted: whitelisted,
					})
				}
			}
		}

		if !whitelists.IsCheckDisabled(CheckTypeHistogramBuckets) {
			if h := m.GetHistogram(); h != nil {
				if buckets := len(h.Bucket); buckets > stats.maxHistogramBuckets {
					stats.maxHistogramBuckets = buckets
				}

				if _, whitelisted, violated := p.metricCheckLimit(metricName, CheckTypeHistogramBuckets, int64(p.limits.MaxHistogramBuckets), int64(len(h.Bucket)), whitelists); violated {
					d.addHistogramBucketsViolation(HistogramBucketsViolation{
						MetricName:  metricName,
						Buckets:     len(h.Bucket),
						Whitelisted: whitelisted,
					})
				}
			}
		}
	}
}

func (p *Processor) metricCheckLimit(
	metricName string,
	checkType CheckType,
	defaultLimit int64,
	current int64,
	whitelists *EffectiveWhitelists,
) (limit int64, isWhitelisted bool, violated bool) {
	limit = defaultLimit
	if customLimit, ok := whitelists.MetricCustomLimit(metricName, checkType); ok {
		limit = customLimit
		isWhitelisted = true
	}
	return limit, isWhitelisted, current > limit
}

func buildCheckResults(report *Report, limits LimitsConfig, whitelists *EffectiveWhitelists) []CheckResult {
	checks := make([]CheckResult, 0, 6)

	if !whitelists.IsCheckDisabled(CheckTypeMetricNameLength) {
		hasViolation, allWhitelisted := report.Details.metricNameWhitelistStatus()
		checks = append(checks, CheckResult{
			Type:        CheckTypeMetricNameLength,
			Limit:       int64(limits.MaxMetricNameLen),
			Current:     int64(report.maxMetricNameLen),
			Violated:    hasViolation,
			Whitelisted: hasViolation && allWhitelisted,
		})
	}

	if !whitelists.IsCheckDisabled(CheckTypeLabelNameLength) {
		hasViolation, allWhitelisted := report.Details.labelNameWhitelistStatus()
		checks = append(checks, CheckResult{
			Type:        CheckTypeLabelNameLength,
			Limit:       int64(limits.MaxLabelNameLen),
			Current:     int64(report.maxLabelNameLen),
			Violated:    hasViolation,
			Whitelisted: hasViolation && allWhitelisted,
		})
	}

	if !whitelists.IsCheckDisabled(CheckTypeLabelValueLength) {
		hasViolation, allWhitelisted := report.Details.labelValueWhitelistStatus()
		checks = append(checks, CheckResult{
			Type:        CheckTypeLabelValueLength,
			Limit:       int64(limits.MaxLabelValueLen),
			Current:     int64(report.maxLabelValueLen),
			Violated:    hasViolation,
			Whitelisted: hasViolation && allWhitelisted,
		})
	}

	if !whitelists.IsCheckDisabled(CheckTypeCardinality) {
		hasViolation, allWhitelisted := report.Details.cardinalityWhitelistStatus()
		checks = append(checks, CheckResult{
			Type:        CheckTypeCardinality,
			Limit:       int64(limits.MaxMetricCardinality),
			Current:     int64(report.maxMetricCardinality),
			Violated:    hasViolation,
			Whitelisted: hasViolation && allWhitelisted,
		})
	}

	if !whitelists.IsCheckDisabled(CheckTypeHistogramBuckets) {
		hasViolation, allWhitelisted := report.Details.histogramBucketsWhitelistStatus()
		checks = append(checks, CheckResult{
			Type:        CheckTypeHistogramBuckets,
			Limit:       int64(limits.MaxHistogramBuckets),
			Current:     int64(report.maxHistogramBuckets),
			Violated:    hasViolation,
			Whitelisted: hasViolation && allWhitelisted,
		})
	}

	if !whitelists.IsCheckDisabled(CheckTypeResponseWeight) {
		checks = append(checks, CheckResult{
			Type:        CheckTypeResponseWeight,
			Limit:       limits.MaxBytesWeight,
			Current:     report.maxResponseWeight,
			Violated:    report.maxResponseWeight > limits.MaxBytesWeight,
			Whitelisted: false,
		})
	}

	return checks
}

func buildCheckLimits(checks []CheckResult) *CheckLimits {
	if len(checks) == 0 {
		return nil
	}

	limits := &CheckLimits{}
	for _, check := range checks {
		switch check.Type {
		case CheckTypeMetricNameLength:
			limits.MetricNameLength = check.Limit
		case CheckTypeLabelNameLength:
			limits.LabelNameLength = check.Limit
		case CheckTypeLabelValueLength:
			limits.LabelValueLength = check.Limit
		case CheckTypeCardinality:
			limits.Cardinality = check.Limit
		case CheckTypeHistogramBuckets:
			limits.HistogramBuckets = check.Limit
		case CheckTypeResponseWeight:
			limits.ResponseWeight = check.Limit
		}
	}

	return limits
}

func buildCheckCurrent(checks []CheckResult) *CheckCurrent {
	if len(checks) == 0 {
		return nil
	}

	current := &CheckCurrent{}
	for _, check := range checks {
		switch check.Type {
		case CheckTypeMetricNameLength:
			current.MetricNameLength = check.Current
		case CheckTypeLabelNameLength:
			current.LabelNameLength = check.Current
		case CheckTypeLabelValueLength:
			current.LabelValueLength = check.Current
		case CheckTypeCardinality:
			current.Cardinality = check.Current
		case CheckTypeHistogramBuckets:
			current.HistogramBuckets = check.Current
		case CheckTypeResponseWeight:
			current.ResponseWeight = check.Current
		}
	}

	return current
}
