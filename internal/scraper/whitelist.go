package scraper

import "context"

type MetricWhitelistRule struct {
	TargetGroup string              `json:"target_group"`
	Env         string              `json:"env"`
	MetricName  string              `json:"metric_name"`
	Checks      map[CheckType]int64 `json:"checks,omitempty"`
}

type TargetWhitelistRule struct {
	TargetGroup string      `json:"target_group"`
	Env         string      `json:"env"`
	Checks      []CheckType `json:"checks,omitempty"`
}

type EffectiveWhitelists struct {
	MetricChecks   map[string]map[CheckType]int64
	DisabledChecks map[CheckType]struct{}
}

func NewEffectiveWhitelists() *EffectiveWhitelists {
	return &EffectiveWhitelists{
		MetricChecks:   make(map[string]map[CheckType]int64),
		DisabledChecks: make(map[CheckType]struct{}),
	}
}

func (w *EffectiveWhitelists) IsCheckDisabled(checkType CheckType) bool {
	if w == nil {
		return false
	}
	_, ok := w.DisabledChecks[checkType]
	return ok
}

func (w *EffectiveWhitelists) MetricCustomLimit(metricName string, checkType CheckType) (int64, bool) {
	if w == nil {
		return 0, false
	}

	metricChecks, ok := w.MetricChecks[metricName]
	if !ok {
		return 0, false
	}

	limit, ok := metricChecks[checkType]
	if !ok {
		return 0, false
	}

	return limit, true
}

type WhitelistProvider interface {
	GetEffectiveWhitelists(ctx context.Context, targetGroup, env string) (*EffectiveWhitelists, error)
}
