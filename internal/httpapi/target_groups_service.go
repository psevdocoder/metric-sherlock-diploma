package httpapi

import (
	"context"
	"errors"
	"time"

	"git.server.lan/maksim/metric-sherlock-diploma/internal/scraper"
	"git.server.lan/maksim/metric-sherlock-diploma/internal/storage/postgres"
	targetgroupsv1 "git.server.lan/maksim/metric-sherlock-diploma/proto/metricsherlock/targetgroups/v1"
	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type targetGroupStorage interface {
	ListTargetGroups(ctx context.Context, teamName string) ([]*postgres.TargetGroupWithReport, error)
	GetTargetGroupByID(ctx context.Context, id int64) (*postgres.TargetGroupWithReport, error)
	ListMetricWhitelist(ctx context.Context, targetGroup, env string) ([]scraper.MetricWhitelistRule, error)
	UpsertMetricWhitelist(ctx context.Context, item scraper.MetricWhitelistRule) error
	DeleteMetricWhitelist(ctx context.Context, targetGroup, env, metricName string) error
	ListTargetWhitelist(ctx context.Context, targetGroup, env string) ([]scraper.TargetWhitelistRule, error)
	UpsertTargetWhitelist(ctx context.Context, item scraper.TargetWhitelistRule) error
	DeleteTargetWhitelist(ctx context.Context, targetGroup, env string) error
}

type targetGroupsService struct {
	targetgroupsv1.UnimplementedTargetGroupsServiceServer
	storage targetGroupStorage
}

func newTargetGroupsService(storage targetGroupStorage) *targetGroupsService {
	return &targetGroupsService{storage: storage}
}

func (s *targetGroupsService) ListTargetGroups(
	ctx context.Context,
	req *targetgroupsv1.ListTargetGroupsRequest,
) (*targetgroupsv1.ListTargetGroupsResponse, error) {
	targetGroups, err := s.storage.ListTargetGroups(ctx, req.GetTeamName())
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to load target groups")
	}

	resp := &targetgroupsv1.ListTargetGroupsResponse{
		TargetGroups: make([]*targetgroupsv1.TargetGroupSummary, 0, len(targetGroups)),
	}

	for _, group := range targetGroups {
		resp.TargetGroups = append(resp.TargetGroups, toTargetGroupSummary(group))
	}

	return resp, nil
}

func (s *targetGroupsService) GetTargetGroup(
	ctx context.Context,
	req *targetgroupsv1.GetTargetGroupRequest,
) (*targetgroupsv1.GetTargetGroupResponse, error) {
	if req.GetId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "id must be positive")
	}

	targetGroup, err := s.storage.GetTargetGroupByID(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, status.Error(codes.NotFound, "target group not found")
		}
		return nil, status.Error(codes.Internal, "failed to load target group")
	}

	return &targetgroupsv1.GetTargetGroupResponse{
		TargetGroup: &targetgroupsv1.TargetGroupDetails{
			Summary:         toTargetGroupSummary(targetGroup),
			Violations:      toViolationDetails(targetGroup.Details),
			ReportCreatedAt: toTimestamp(targetGroup.ReportCreatedAt),
		},
	}, nil
}

func (s *targetGroupsService) ListMetricWhitelist(
	ctx context.Context,
	req *targetgroupsv1.ListMetricWhitelistRequest,
) (*targetgroupsv1.ListMetricWhitelistResponse, error) {
	items, err := s.storage.ListMetricWhitelist(ctx, req.GetTargetGroup(), req.GetEnv())
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to load metric whitelist")
	}

	resp := &targetgroupsv1.ListMetricWhitelistResponse{
		Items: make([]*targetgroupsv1.MetricWhitelistItem, 0, len(items)),
	}
	for _, item := range items {
		resp.Items = append(resp.Items, toMetricWhitelistItem(item))
	}

	return resp, nil
}

func (s *targetGroupsService) UpsertMetricWhitelist(
	ctx context.Context,
	req *targetgroupsv1.UpsertMetricWhitelistRequest,
) (*emptypb.Empty, error) {
	item := req.GetItem()
	if item == nil {
		return nil, status.Error(codes.InvalidArgument, "item is required")
	}

	if item.GetTargetGroup() == "" {
		return nil, status.Error(codes.InvalidArgument, "target_group is required")
	}
	if item.GetEnv() == "" {
		return nil, status.Error(codes.InvalidArgument, "env is required")
	}
	if item.GetMetricName() == "" {
		return nil, status.Error(codes.InvalidArgument, "metric_name is required")
	}

	checks := fromMetricWhitelistChecks(item.GetChecks())
	if len(checks) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one positive check limit is required")
	}

	err := s.storage.UpsertMetricWhitelist(ctx, scraper.MetricWhitelistRule{
		TargetGroup: item.GetTargetGroup(),
		Env:         item.GetEnv(),
		MetricName:  item.GetMetricName(),
		Checks:      checks,
	})
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to save metric whitelist")
	}

	return &emptypb.Empty{}, nil
}

func (s *targetGroupsService) DeleteMetricWhitelist(
	ctx context.Context,
	req *targetgroupsv1.DeleteMetricWhitelistRequest,
) (*emptypb.Empty, error) {
	if req.GetTargetGroup() == "" || req.GetEnv() == "" || req.GetMetricName() == "" {
		return nil, status.Error(codes.InvalidArgument, "target_group, env and metric_name are required")
	}

	if err := s.storage.DeleteMetricWhitelist(ctx, req.GetTargetGroup(), req.GetEnv(), req.GetMetricName()); err != nil {
		return nil, status.Error(codes.Internal, "failed to delete metric whitelist")
	}

	return &emptypb.Empty{}, nil
}

func (s *targetGroupsService) ListTargetWhitelist(
	ctx context.Context,
	req *targetgroupsv1.ListTargetWhitelistRequest,
) (*targetgroupsv1.ListTargetWhitelistResponse, error) {
	items, err := s.storage.ListTargetWhitelist(ctx, req.GetTargetGroup(), req.GetEnv())
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to load target whitelist")
	}

	resp := &targetgroupsv1.ListTargetWhitelistResponse{
		Items: make([]*targetgroupsv1.TargetWhitelistItem, 0, len(items)),
	}
	for _, item := range items {
		resp.Items = append(resp.Items, toTargetWhitelistItem(item))
	}

	return resp, nil
}

func (s *targetGroupsService) UpsertTargetWhitelist(
	ctx context.Context,
	req *targetgroupsv1.UpsertTargetWhitelistRequest,
) (*emptypb.Empty, error) {
	item := req.GetItem()
	if item == nil {
		return nil, status.Error(codes.InvalidArgument, "item is required")
	}

	if item.GetTargetGroup() == "" {
		return nil, status.Error(codes.InvalidArgument, "target_group is required")
	}
	if item.GetEnv() == "" {
		return nil, status.Error(codes.InvalidArgument, "env is required")
	}

	checks := fromTargetWhitelistChecks(item.GetChecks())
	if len(checks) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one check must be enabled")
	}

	err := s.storage.UpsertTargetWhitelist(ctx, scraper.TargetWhitelistRule{
		TargetGroup: item.GetTargetGroup(),
		Env:         item.GetEnv(),
		Checks:      checks,
	})
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to save target whitelist")
	}

	return &emptypb.Empty{}, nil
}

func (s *targetGroupsService) DeleteTargetWhitelist(
	ctx context.Context,
	req *targetgroupsv1.DeleteTargetWhitelistRequest,
) (*emptypb.Empty, error) {
	if req.GetTargetGroup() == "" || req.GetEnv() == "" {
		return nil, status.Error(codes.InvalidArgument, "target_group and env are required")
	}

	if err := s.storage.DeleteTargetWhitelist(ctx, req.GetTargetGroup(), req.GetEnv()); err != nil {
		return nil, status.Error(codes.Internal, "failed to delete target whitelist")
	}

	return &emptypb.Empty{}, nil
}

func toTargetGroupSummary(group *postgres.TargetGroupWithReport) *targetgroupsv1.TargetGroupSummary {
	return &targetgroupsv1.TargetGroupSummary{
		Id:             group.ID,
		Name:           group.Name,
		Env:            group.Env,
		Cluster:        group.Cluster,
		Job:            group.Job,
		TeamName:       group.TeamName,
		FirstCheck:     toTimestamp(&group.FirstCheck),
		LastCheck:      toTimestamp(&group.LastCheck),
		ViolationStats: toViolationStats(group.Details),
	}
}

func toViolationStats(details scraper.Details) *targetgroupsv1.ViolationStats {
	metricNameTooLong := int32(len(details.MetricNameTooLong))
	labelNameTooLong := int32(len(details.LabelNameTooLong))
	labelValueTooLong := int32(len(details.LabelValueTooLong))
	cardinality := int32(len(details.Cardinality))
	histogramBuckets := int32(len(details.HistogramBuckets))

	return &targetgroupsv1.ViolationStats{
		Total:             metricNameTooLong + labelNameTooLong + labelValueTooLong + cardinality + histogramBuckets,
		MetricNameTooLong: metricNameTooLong,
		LabelNameTooLong:  labelNameTooLong,
		LabelValueTooLong: labelValueTooLong,
		Cardinality:       cardinality,
		HistogramBuckets:  histogramBuckets,
		ResponseWeight:    details.ResponseWeight,
	}
}

func toViolationDetails(details scraper.Details) *targetgroupsv1.ViolationDetails {
	resp := &targetgroupsv1.ViolationDetails{
		MetricNameTooLong: make([]*targetgroupsv1.MetricNameViolation, 0, len(details.MetricNameTooLong)),
		LabelNameTooLong:  make([]*targetgroupsv1.LabelNameViolation, 0, len(details.LabelNameTooLong)),
		LabelValueTooLong: make([]*targetgroupsv1.LabelValueViolation, 0, len(details.LabelValueTooLong)),
		Cardinality:       make([]*targetgroupsv1.CardinalityViolation, 0, len(details.Cardinality)),
		HistogramBuckets:  make([]*targetgroupsv1.HistogramBucketsViolation, 0, len(details.HistogramBuckets)),
		ResponseWeight:    details.ResponseWeight,
		Checks:            toCheckMetrics(details),
	}

	for _, violation := range details.MetricNameTooLong {
		resp.MetricNameTooLong = append(resp.MetricNameTooLong, &targetgroupsv1.MetricNameViolation{
			MetricName: violation.MetricName,
			Length:     int32(violation.Length),
		})
	}
	for _, violation := range details.LabelNameTooLong {
		resp.LabelNameTooLong = append(resp.LabelNameTooLong, &targetgroupsv1.LabelNameViolation{
			MetricName: violation.MetricName,
			LabelName:  violation.LabelName,
			Length:     int32(violation.Length),
		})
	}
	for _, violation := range details.LabelValueTooLong {
		resp.LabelValueTooLong = append(resp.LabelValueTooLong, &targetgroupsv1.LabelValueViolation{
			MetricName: violation.MetricName,
			LabelName:  violation.LabelName,
			Value:      violation.Value,
			Length:     int32(violation.Length),
		})
	}
	for _, violation := range details.Cardinality {
		resp.Cardinality = append(resp.Cardinality, &targetgroupsv1.CardinalityViolation{
			MetricName: violation.MetricName,
			Value:      int32(violation.Value),
		})
	}
	for _, violation := range details.HistogramBuckets {
		resp.HistogramBuckets = append(resp.HistogramBuckets, &targetgroupsv1.HistogramBucketsViolation{
			MetricName: violation.MetricName,
			Buckets:    int32(violation.Buckets),
		})
	}

	if details.Max != nil {
		resp.Max = &targetgroupsv1.MaxViolationStats{}
		if details.Max.MetricNameTooLong != nil {
			resp.Max.MetricNameTooLong = &targetgroupsv1.MetricNameViolation{
				MetricName: details.Max.MetricNameTooLong.MetricName,
				Length:     int32(details.Max.MetricNameTooLong.Length),
			}
		}
		if details.Max.LabelNameTooLong != nil {
			resp.Max.LabelNameTooLong = &targetgroupsv1.LabelNameViolation{
				MetricName: details.Max.LabelNameTooLong.MetricName,
				LabelName:  details.Max.LabelNameTooLong.LabelName,
				Length:     int32(details.Max.LabelNameTooLong.Length),
			}
		}
		if details.Max.LabelValueTooLong != nil {
			resp.Max.LabelValueTooLong = &targetgroupsv1.LabelValueViolation{
				MetricName: details.Max.LabelValueTooLong.MetricName,
				LabelName:  details.Max.LabelValueTooLong.LabelName,
				Value:      details.Max.LabelValueTooLong.Value,
				Length:     int32(details.Max.LabelValueTooLong.Length),
			}
		}
		if details.Max.Cardinality != nil {
			resp.Max.Cardinality = &targetgroupsv1.CardinalityViolation{
				MetricName: details.Max.Cardinality.MetricName,
				Value:      int32(details.Max.Cardinality.Value),
			}
		}
		if details.Max.HistogramBuckets != nil {
			resp.Max.HistogramBuckets = &targetgroupsv1.HistogramBucketsViolation{
				MetricName: details.Max.HistogramBuckets.MetricName,
				Buckets:    int32(details.Max.HistogramBuckets.Buckets),
			}
		}
	}

	return resp
}

func toCheckMetrics(details scraper.Details) *targetgroupsv1.CheckMetrics {
	if len(details.Checks) > 0 {
		return toCheckMetricsFromResults(details.Checks)
	}

	if details.Limits == nil && details.Current == nil {
		return nil
	}

	return &targetgroupsv1.CheckMetrics{
		MetricNameLength: toCheckMetric(details, func(l *scraper.CheckLimits) int64 { return l.MetricNameLength }, func(c *scraper.CheckCurrent) int64 { return c.MetricNameLength }),
		LabelNameLength:  toCheckMetric(details, func(l *scraper.CheckLimits) int64 { return l.LabelNameLength }, func(c *scraper.CheckCurrent) int64 { return c.LabelNameLength }),
		LabelValueLength: toCheckMetric(details, func(l *scraper.CheckLimits) int64 { return l.LabelValueLength }, func(c *scraper.CheckCurrent) int64 { return c.LabelValueLength }),
		Cardinality:      toCheckMetric(details, func(l *scraper.CheckLimits) int64 { return l.Cardinality }, func(c *scraper.CheckCurrent) int64 { return c.Cardinality }),
		HistogramBuckets: toCheckMetric(details, func(l *scraper.CheckLimits) int64 { return l.HistogramBuckets }, func(c *scraper.CheckCurrent) int64 { return c.HistogramBuckets }),
		ResponseWeight:   toCheckMetric(details, func(l *scraper.CheckLimits) int64 { return l.ResponseWeight }, func(c *scraper.CheckCurrent) int64 { return c.ResponseWeight }),
	}
}

func toCheckMetricsFromResults(checks []scraper.CheckResult) *targetgroupsv1.CheckMetrics {
	byType := make(map[scraper.CheckType]scraper.CheckResult, len(checks))
	for _, check := range checks {
		byType[check.Type] = check
	}

	return &targetgroupsv1.CheckMetrics{
		MetricNameLength: toCheckMetricFromResult(byType, scraper.CheckTypeMetricNameLength),
		LabelNameLength:  toCheckMetricFromResult(byType, scraper.CheckTypeLabelNameLength),
		LabelValueLength: toCheckMetricFromResult(byType, scraper.CheckTypeLabelValueLength),
		Cardinality:      toCheckMetricFromResult(byType, scraper.CheckTypeCardinality),
		HistogramBuckets: toCheckMetricFromResult(byType, scraper.CheckTypeHistogramBuckets),
		ResponseWeight:   toCheckMetricFromResult(byType, scraper.CheckTypeResponseWeight),
	}
}

func toCheckMetricFromResult(
	byType map[scraper.CheckType]scraper.CheckResult,
	checkType scraper.CheckType,
) *targetgroupsv1.CheckMetric {
	check, ok := byType[checkType]
	if !ok {
		return nil
	}

	return &targetgroupsv1.CheckMetric{
		Limit:   check.Limit,
		Current: check.Current,
	}
}

func toCheckMetric(
	details scraper.Details,
	getLimit func(*scraper.CheckLimits) int64,
	getCurrent func(*scraper.CheckCurrent) int64,
) *targetgroupsv1.CheckMetric {
	var (
		limit   int64
		current int64
		hasData bool
	)

	if details.Limits != nil {
		limit = getLimit(details.Limits)
		hasData = true
	}

	if details.Current != nil {
		current = getCurrent(details.Current)
		hasData = true
	}

	if !hasData {
		return nil
	}

	return &targetgroupsv1.CheckMetric{
		Limit:   limit,
		Current: current,
	}
}

func toTimestamp(value *time.Time) *timestamppb.Timestamp {
	if value == nil || value.IsZero() {
		return nil
	}
	return timestamppb.New(*value)
}

func toMetricWhitelistItem(item scraper.MetricWhitelistRule) *targetgroupsv1.MetricWhitelistItem {
	return &targetgroupsv1.MetricWhitelistItem{
		TargetGroup: item.TargetGroup,
		Env:         item.Env,
		MetricName:  item.MetricName,
		Checks:      toMetricWhitelistChecks(item.Checks),
	}
}

func toMetricWhitelistChecks(checks map[scraper.CheckType]int64) *targetgroupsv1.MetricWhitelistChecks {
	if len(checks) == 0 {
		return nil
	}

	resp := &targetgroupsv1.MetricWhitelistChecks{}
	resp.MetricNameLength = checks[scraper.CheckTypeMetricNameLength]
	resp.LabelNameLength = checks[scraper.CheckTypeLabelNameLength]
	resp.LabelValueLength = checks[scraper.CheckTypeLabelValueLength]
	resp.Cardinality = checks[scraper.CheckTypeCardinality]
	resp.HistogramBuckets = checks[scraper.CheckTypeHistogramBuckets]
	return resp
}

func fromMetricWhitelistChecks(checks *targetgroupsv1.MetricWhitelistChecks) map[scraper.CheckType]int64 {
	if checks == nil {
		return nil
	}

	resp := make(map[scraper.CheckType]int64, 5)
	if value := checks.GetMetricNameLength(); value > 0 {
		resp[scraper.CheckTypeMetricNameLength] = value
	}
	if value := checks.GetLabelNameLength(); value > 0 {
		resp[scraper.CheckTypeLabelNameLength] = value
	}
	if value := checks.GetLabelValueLength(); value > 0 {
		resp[scraper.CheckTypeLabelValueLength] = value
	}
	if value := checks.GetCardinality(); value > 0 {
		resp[scraper.CheckTypeCardinality] = value
	}
	if value := checks.GetHistogramBuckets(); value > 0 {
		resp[scraper.CheckTypeHistogramBuckets] = value
	}
	return resp
}

func toTargetWhitelistItem(item scraper.TargetWhitelistRule) *targetgroupsv1.TargetWhitelistItem {
	return &targetgroupsv1.TargetWhitelistItem{
		TargetGroup: item.TargetGroup,
		Env:         item.Env,
		Checks:      toTargetWhitelistChecks(item.Checks),
	}
}

func toTargetWhitelistChecks(checks []scraper.CheckType) *targetgroupsv1.TargetWhitelistChecks {
	if len(checks) == 0 {
		return nil
	}

	resp := &targetgroupsv1.TargetWhitelistChecks{}
	for _, checkType := range checks {
		switch checkType {
		case scraper.CheckTypeMetricNameLength:
			resp.MetricNameLength = true
		case scraper.CheckTypeLabelNameLength:
			resp.LabelNameLength = true
		case scraper.CheckTypeLabelValueLength:
			resp.LabelValueLength = true
		case scraper.CheckTypeCardinality:
			resp.Cardinality = true
		case scraper.CheckTypeHistogramBuckets:
			resp.HistogramBuckets = true
		case scraper.CheckTypeResponseWeight:
			resp.ResponseWeight = true
		}
	}
	return resp
}

func fromTargetWhitelistChecks(checks *targetgroupsv1.TargetWhitelistChecks) []scraper.CheckType {
	if checks == nil {
		return nil
	}

	resp := make([]scraper.CheckType, 0, 6)
	if checks.GetMetricNameLength() {
		resp = append(resp, scraper.CheckTypeMetricNameLength)
	}
	if checks.GetLabelNameLength() {
		resp = append(resp, scraper.CheckTypeLabelNameLength)
	}
	if checks.GetLabelValueLength() {
		resp = append(resp, scraper.CheckTypeLabelValueLength)
	}
	if checks.GetCardinality() {
		resp = append(resp, scraper.CheckTypeCardinality)
	}
	if checks.GetHistogramBuckets() {
		resp = append(resp, scraper.CheckTypeHistogramBuckets)
	}
	if checks.GetResponseWeight() {
		resp = append(resp, scraper.CheckTypeResponseWeight)
	}
	return resp
}
