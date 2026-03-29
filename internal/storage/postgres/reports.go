package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"git.server.lan/maksim/metric-sherlock-diploma/internal/scraper"
	metricviolationsv1 "git.server.lan/maksim/metric-sherlock-diploma/proto/metricsherlock/metricviolations/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const saveReportsSQL = `
INSERT INTO reports (target_group, env, cluster, team_name, details, created_at)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (target_group, env, cluster)
DO UPDATE SET
team_name = EXCLUDED.team_name,
details   = EXCLUDED.details,
created_at = EXCLUDED.created_at
`

func (s *Storage) SaveReports(ctx context.Context, reports []*scraper.Report) error {
	if len(reports) == 0 {
		return nil
	}

	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	createdAt := time.Now()

	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, report := range reports {
		detailsBytes, err := json.Marshal(report.Details)
		if err != nil {
			return err
		}

		_, err = tx.Exec(
			ctx,
			saveReportsSQL,
			report.TargetGroup,
			report.Env,
			report.Cluster,
			report.TeamName,
			detailsBytes,
			createdAt,
		)
		if err != nil {
			return err
		}

		events, err := s.buildViolationOutboxEvents(report, createdAt)
		if err != nil {
			return err
		}

		if err = s.enqueueOutboxEvents(ctx, tx, events); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (s *Storage) buildViolationOutboxEvents(report *scraper.Report, createdAt time.Time) ([]outboxEvent, error) {
	events := make([]outboxEvent, 0, len(report.Checks))

	for _, check := range report.Checks {
		if !check.Violated {
			continue
		}

		payload, err := proto.Marshal(&metricviolationsv1.ServiceViolationFact{
			TargetGroup:   report.TargetGroup,
			Env:           report.Env,
			Cluster:       report.Cluster,
			TeamName:      report.TeamName,
			ViolationType: mapCheckType(check.Type),
			Limit:         check.Limit,
			CurrentValue:  check.Current,
			Violated:      check.Violated,
			CheckedAt:     timestamppb.New(createdAt),
		})
		if err != nil {
			return nil, err
		}

		events = append(events, outboxEvent{
			Topic: s.outboxTopic,
			Key: fmt.Sprintf(
				"%s|%s|%s|%s",
				report.TargetGroup,
				report.Env,
				report.Cluster,
				check.Type,
			),
			Body:      payload,
			CreatedAt: createdAt,
		})
	}

	return events, nil
}

func mapCheckType(checkType scraper.CheckType) metricviolationsv1.ViolationType {
	switch checkType {
	case scraper.CheckTypeMetricNameLength:
		return metricviolationsv1.ViolationType_VIOLATION_TYPE_METRIC_NAME_LENGTH
	case scraper.CheckTypeLabelNameLength:
		return metricviolationsv1.ViolationType_VIOLATION_TYPE_LABEL_NAME_LENGTH
	case scraper.CheckTypeLabelValueLength:
		return metricviolationsv1.ViolationType_VIOLATION_TYPE_LABEL_VALUE_LENGTH
	case scraper.CheckTypeCardinality:
		return metricviolationsv1.ViolationType_VIOLATION_TYPE_CARDINALITY
	case scraper.CheckTypeHistogramBuckets:
		return metricviolationsv1.ViolationType_VIOLATION_TYPE_HISTOGRAM_BUCKETS
	case scraper.CheckTypeResponseWeight:
		return metricviolationsv1.ViolationType_VIOLATION_TYPE_RESPONSE_WEIGHT
	default:
		return metricviolationsv1.ViolationType_VIOLATION_TYPE_UNSPECIFIED
	}
}
