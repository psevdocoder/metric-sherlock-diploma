package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"git.server.lan/maksim/metric-sherlock-diploma/internal/scraper"
)

const (
	listMetricWhitelistSQL = `
SELECT target_group, env, metric_name, checks
FROM metrics_whitelist
WHERE ($1 = '' OR target_group = $1)
  AND ($2 = '' OR env = $2)
ORDER BY target_group, env, metric_name;
`
	upsertMetricWhitelistSQL = `
INSERT INTO metrics_whitelist (target_group, env, metric_name, checks)
VALUES ($1, $2, $3, $4)
ON CONFLICT (target_group, env, metric_name)
DO UPDATE SET checks = EXCLUDED.checks;
`
	deleteMetricWhitelistSQL = `
DELETE FROM metrics_whitelist
WHERE target_group = $1
  AND env = $2
  AND metric_name = $3;
`
	listTargetWhitelistSQL = `
SELECT target_group, env, checks
FROM targets_whitelist
WHERE ($1 = '' OR target_group = $1)
  AND ($2 = '' OR env = $2)
ORDER BY target_group, env;
`
	upsertTargetWhitelistSQL = `
INSERT INTO targets_whitelist (target_group, env, checks)
VALUES ($1, $2, $3)
ON CONFLICT (target_group, env)
DO UPDATE SET checks = EXCLUDED.checks;
`
	deleteTargetWhitelistSQL = `
DELETE FROM targets_whitelist
WHERE target_group = $1
  AND env = $2;
`
)

var (
	errEmptyTargetGroup = errors.New("target_group is required")
	errEmptyEnv         = errors.New("env is required")
	errEmptyMetricName  = errors.New("metric_name is required")
	errEmptyChecks      = errors.New("checks are required")
)

func (s *Storage) ListMetricWhitelist(ctx context.Context, targetGroup, env string) ([]scraper.MetricWhitelistRule, error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	rows, err := conn.Query(ctx, listMetricWhitelistSQL, targetGroup, env)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]scraper.MetricWhitelistRule, 0)
	for rows.Next() {
		var (
			item      scraper.MetricWhitelistRule
			checksRaw []byte
		)

		if err = rows.Scan(&item.TargetGroup, &item.Env, &item.MetricName, &checksRaw); err != nil {
			return nil, err
		}

		item.Checks, err = parseMetricChecksJSON(checksRaw)
		if err != nil {
			return nil, err
		}

		items = append(items, item)
	}

	return items, rows.Err()
}

func (s *Storage) UpsertMetricWhitelist(ctx context.Context, item scraper.MetricWhitelistRule) error {
	if item.TargetGroup == "" {
		return errEmptyTargetGroup
	}
	if item.Env == "" {
		return errEmptyEnv
	}
	if item.MetricName == "" {
		return errEmptyMetricName
	}
	if len(item.Checks) == 0 {
		return errEmptyChecks
	}

	checksJSON, err := marshalMetricChecks(item.Checks)
	if err != nil {
		return err
	}

	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	_, err = conn.Exec(
		ctx,
		upsertMetricWhitelistSQL,
		item.TargetGroup,
		item.Env,
		item.MetricName,
		checksJSON,
	)
	return err
}

func (s *Storage) DeleteMetricWhitelist(ctx context.Context, targetGroup, env, metricName string) error {
	if targetGroup == "" {
		return errEmptyTargetGroup
	}
	if env == "" {
		return errEmptyEnv
	}
	if metricName == "" {
		return errEmptyMetricName
	}

	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	_, err = conn.Exec(ctx, deleteMetricWhitelistSQL, targetGroup, env, metricName)
	return err
}

func (s *Storage) ListTargetWhitelist(ctx context.Context, targetGroup, env string) ([]scraper.TargetWhitelistRule, error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	rows, err := conn.Query(ctx, listTargetWhitelistSQL, targetGroup, env)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]scraper.TargetWhitelistRule, 0)
	for rows.Next() {
		var (
			item      scraper.TargetWhitelistRule
			checksRaw []byte
		)

		if err = rows.Scan(&item.TargetGroup, &item.Env, &checksRaw); err != nil {
			return nil, err
		}

		item.Checks, err = parseTargetChecksJSON(checksRaw)
		if err != nil {
			return nil, err
		}

		items = append(items, item)
	}

	return items, rows.Err()
}

func (s *Storage) UpsertTargetWhitelist(ctx context.Context, item scraper.TargetWhitelistRule) error {
	if item.TargetGroup == "" {
		return errEmptyTargetGroup
	}
	if item.Env == "" {
		return errEmptyEnv
	}
	if len(item.Checks) == 0 {
		return errEmptyChecks
	}

	checksJSON, err := marshalTargetChecks(item.Checks)
	if err != nil {
		return err
	}

	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	_, err = conn.Exec(
		ctx,
		upsertTargetWhitelistSQL,
		item.TargetGroup,
		item.Env,
		checksJSON,
	)
	return err
}

func (s *Storage) DeleteTargetWhitelist(ctx context.Context, targetGroup, env string) error {
	if targetGroup == "" {
		return errEmptyTargetGroup
	}
	if env == "" {
		return errEmptyEnv
	}

	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	_, err = conn.Exec(ctx, deleteTargetWhitelistSQL, targetGroup, env)
	return err
}

func (s *Storage) GetEffectiveWhitelists(ctx context.Context, targetGroup, env string) (*scraper.EffectiveWhitelists, error) {
	metricRules, err := s.ListMetricWhitelist(ctx, targetGroup, env)
	if err != nil {
		return nil, err
	}

	targetRules, err := s.ListTargetWhitelist(ctx, targetGroup, env)
	if err != nil {
		return nil, err
	}

	result := scraper.NewEffectiveWhitelists()

	for _, rule := range metricRules {
		if len(rule.Checks) == 0 {
			continue
		}
		checksCopy := make(map[scraper.CheckType]int64, len(rule.Checks))
		for checkType, limit := range rule.Checks {
			checksCopy[checkType] = limit
		}
		result.MetricChecks[rule.MetricName] = checksCopy
	}

	for _, rule := range targetRules {
		for _, checkType := range rule.Checks {
			result.DisabledChecks[checkType] = struct{}{}
		}
	}

	return result, nil
}

func parseMetricChecksJSON(raw []byte) (map[scraper.CheckType]int64, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	var decoded map[string]int64
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("invalid metrics whitelist checks JSON: %w", err)
	}

	result := make(map[scraper.CheckType]int64, len(decoded))
	for key, value := range decoded {
		checkType := scraper.CheckType(key)
		if !isMetricWhitelistCheckType(checkType) {
			return nil, fmt.Errorf("unsupported metric whitelist check: %s", key)
		}
		if value <= 0 {
			return nil, fmt.Errorf("metric whitelist limit must be positive for check: %s", key)
		}
		result[checkType] = value
	}

	return result, nil
}

func marshalMetricChecks(checks map[scraper.CheckType]int64) ([]byte, error) {
	if len(checks) == 0 {
		return nil, errEmptyChecks
	}

	normalized := make(map[string]int64, len(checks))
	for checkType, limit := range checks {
		if !isMetricWhitelistCheckType(checkType) {
			return nil, fmt.Errorf("unsupported metric whitelist check: %s", checkType)
		}
		if limit <= 0 {
			return nil, fmt.Errorf("metric whitelist limit must be positive for check: %s", checkType)
		}
		normalized[string(checkType)] = limit
	}

	return json.Marshal(normalized)
}

func parseTargetChecksJSON(raw []byte) ([]scraper.CheckType, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	var decodedArray []string
	if err := json.Unmarshal(raw, &decodedArray); err == nil {
		return normalizeTargetChecks(decodedArray)
	}

	var decodedObject map[string]bool
	if err := json.Unmarshal(raw, &decodedObject); err == nil {
		checks := make([]string, 0, len(decodedObject))
		for checkType, enabled := range decodedObject {
			if enabled {
				checks = append(checks, checkType)
			}
		}
		return normalizeTargetChecks(checks)
	}

	return nil, errors.New("invalid target whitelist checks JSON")
}

func normalizeTargetChecks(rawChecks []string) ([]scraper.CheckType, error) {
	if len(rawChecks) == 0 {
		return nil, nil
	}

	seen := make(map[scraper.CheckType]struct{}, len(rawChecks))
	result := make([]scraper.CheckType, 0, len(rawChecks))
	for _, rawCheck := range rawChecks {
		checkType := scraper.CheckType(rawCheck)
		if !isTargetWhitelistCheckType(checkType) {
			return nil, fmt.Errorf("unsupported target whitelist check: %s", rawCheck)
		}
		if _, ok := seen[checkType]; ok {
			continue
		}
		seen[checkType] = struct{}{}
		result = append(result, checkType)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i] < result[j]
	})

	return result, nil
}

func marshalTargetChecks(checks []scraper.CheckType) ([]byte, error) {
	if len(checks) == 0 {
		return nil, errEmptyChecks
	}

	normalized := make([]string, 0, len(checks))
	seen := make(map[scraper.CheckType]struct{}, len(checks))
	for _, checkType := range checks {
		if !isTargetWhitelistCheckType(checkType) {
			return nil, fmt.Errorf("unsupported target whitelist check: %s", checkType)
		}
		if _, ok := seen[checkType]; ok {
			continue
		}
		seen[checkType] = struct{}{}
		normalized = append(normalized, string(checkType))
	}

	sort.Strings(normalized)
	return json.Marshal(normalized)
}

func isMetricWhitelistCheckType(checkType scraper.CheckType) bool {
	switch checkType {
	case scraper.CheckTypeMetricNameLength,
		scraper.CheckTypeLabelNameLength,
		scraper.CheckTypeLabelValueLength,
		scraper.CheckTypeCardinality,
		scraper.CheckTypeHistogramBuckets:
		return true
	default:
		return false
	}
}

func isTargetWhitelistCheckType(checkType scraper.CheckType) bool {
	switch checkType {
	case scraper.CheckTypeMetricNameLength,
		scraper.CheckTypeLabelNameLength,
		scraper.CheckTypeLabelValueLength,
		scraper.CheckTypeCardinality,
		scraper.CheckTypeHistogramBuckets,
		scraper.CheckTypeResponseWeight:
		return true
	default:
		return false
	}
}
