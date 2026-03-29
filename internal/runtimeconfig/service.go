package runtimeconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"git.server.lan/maksim/metric-sherlock-diploma/internal/scraper"
	"git.server.lan/maksim/metric-sherlock-diploma/internal/storage/etcdstorage"
	"github.com/robfig/cron/v3"
)

const (
	metricCheckLimitsKey = "/metric-sherlock/runtime/metric-check-limits"
	scrapeTasksCronKey   = "/metric-sherlock/runtime/scrape-tasks-cron"
)

var (
	ErrInvalidLimitsConfig = errors.New("invalid metric check limits config")
	ErrInvalidCronExpr     = errors.New("invalid scrape tasks cron expression")
)

var cronExprParser = cron.NewParser(
	cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

type CronScheduleApplier interface {
	UpdateSchedule(ctx context.Context, spec string) error
}

type Service struct {
	storage         *etcdstorage.Storage
	defaultLimits   scraper.LimitsConfig
	defaultCronExpr string
	cronApplier     CronScheduleApplier
}

func New(storage *etcdstorage.Storage, defaultLimits scraper.LimitsConfig, defaultCronExpr string) *Service {
	return &Service{
		storage:         storage,
		defaultLimits:   defaultLimits,
		defaultCronExpr: defaultCronExpr,
	}
}

func (s *Service) SetCronScheduleApplier(applier CronScheduleApplier) {
	s.cronApplier = applier
}

func (s *Service) Bootstrap(ctx context.Context) error {
	if err := ValidateLimitsConfig(s.defaultLimits); err != nil {
		return fmt.Errorf("invalid default metric check limits config: %w", err)
	}

	defaultCronExpr, err := normalizeAndValidateCronExpr(s.defaultCronExpr)
	if err != nil {
		return fmt.Errorf("invalid default scrape tasks cron expression: %w", err)
	}

	limitsBytes, err := json.Marshal(s.defaultLimits)
	if err != nil {
		return fmt.Errorf("failed to marshal default metric check limits config: %w", err)
	}

	if err = s.ensureKey(ctx, metricCheckLimitsKey, string(limitsBytes)); err != nil {
		return err
	}

	if err = s.ensureKey(ctx, scrapeTasksCronKey, defaultCronExpr); err != nil {
		return err
	}

	return nil
}

func (s *Service) GetMetricLimits(ctx context.Context) (scraper.LimitsConfig, error) {
	raw, err := s.storage.Get(ctx, metricCheckLimitsKey)
	if err != nil {
		return scraper.LimitsConfig{}, fmt.Errorf("failed to load metric check limits config: %w", err)
	}

	var limits scraper.LimitsConfig
	if err = json.Unmarshal([]byte(raw), &limits); err != nil {
		return scraper.LimitsConfig{}, fmt.Errorf("failed to parse metric check limits config: %w", err)
	}

	if err = ValidateLimitsConfig(limits); err != nil {
		return scraper.LimitsConfig{}, err
	}

	return limits, nil
}

func (s *Service) SetMetricLimits(ctx context.Context, limits scraper.LimitsConfig) error {
	if err := ValidateLimitsConfig(limits); err != nil {
		return err
	}

	payload, err := json.Marshal(limits)
	if err != nil {
		return fmt.Errorf("failed to marshal metric check limits config: %w", err)
	}

	if err = s.storage.Put(ctx, metricCheckLimitsKey, string(payload)); err != nil {
		return fmt.Errorf("failed to save metric check limits config: %w", err)
	}

	return nil
}

func (s *Service) GetProduceTasksCronExpr(ctx context.Context) (string, error) {
	raw, err := s.storage.Get(ctx, scrapeTasksCronKey)
	if err != nil {
		return "", fmt.Errorf("failed to load scrape tasks cron expression: %w", err)
	}

	raw, err = normalizeAndValidateCronExpr(raw)
	if err != nil {
		return "", err
	}

	return raw, nil
}

func (s *Service) SetProduceTasksCronExpr(ctx context.Context, cronExpr string) error {
	cronExpr, err := normalizeAndValidateCronExpr(cronExpr)
	if err != nil {
		return err
	}

	current, err := s.GetProduceTasksCronExpr(ctx)
	if err != nil {
		return err
	}

	if current == cronExpr {
		return nil
	}

	if err = s.storage.Put(ctx, scrapeTasksCronKey, cronExpr); err != nil {
		return fmt.Errorf("failed to save scrape tasks cron expression: %w", err)
	}

	if s.cronApplier != nil {
		if err = s.cronApplier.UpdateSchedule(ctx, cronExpr); err != nil {
			_ = s.storage.Put(ctx, scrapeTasksCronKey, current)
			return fmt.Errorf("failed to apply scrape tasks cron expression: %w", err)
		}
	}

	return nil
}

func ValidateLimitsConfig(limits scraper.LimitsConfig) error {
	switch {
	case limits.MaxMetricNameLen <= 0:
		return fmt.Errorf("%w: max_metric_name_len must be positive", ErrInvalidLimitsConfig)
	case limits.MaxLabelNameLen <= 0:
		return fmt.Errorf("%w: max_label_name_len must be positive", ErrInvalidLimitsConfig)
	case limits.MaxLabelValueLen <= 0:
		return fmt.Errorf("%w: max_label_value_len must be positive", ErrInvalidLimitsConfig)
	case limits.MaxMetricCardinality <= 0:
		return fmt.Errorf("%w: max_metric_cardinality must be positive", ErrInvalidLimitsConfig)
	case limits.MaxHistogramBuckets <= 0:
		return fmt.Errorf("%w: max_histogram_buckets must be positive", ErrInvalidLimitsConfig)
	case limits.MaxBytesWeight <= 0:
		return fmt.Errorf("%w: max_bytes_weight must be positive", ErrInvalidLimitsConfig)
	default:
		return nil
	}
}

func normalizeAndValidateCronExpr(spec string) (string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", fmt.Errorf("%w: value is empty", ErrInvalidCronExpr)
	}

	if _, err := cronExprParser.Parse(spec); err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidCronExpr, err)
	}

	return spec, nil
}

func (s *Service) ensureKey(ctx context.Context, key, defaultValue string) error {
	_, err := s.storage.Get(ctx, key)
	if err == nil {
		return nil
	}

	if !errors.Is(err, etcdstorage.ErrKeyNotFound) {
		return fmt.Errorf("failed to check key %q in etcd: %w", key, err)
	}

	if err = s.storage.Put(ctx, key, defaultValue); err != nil {
		return fmt.Errorf("failed to bootstrap key %q in etcd: %w", key, err)
	}

	return nil
}
