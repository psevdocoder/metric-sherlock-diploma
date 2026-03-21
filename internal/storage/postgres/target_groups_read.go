package postgres

import (
	"context"
	"encoding/json"
	"time"

	"git.server.lan/maksim/metric-sherlock-diploma/internal/scraper"
	"github.com/jackc/pgx/v5/pgtype"
)

type TargetGroupWithReport struct {
	ID         int64
	Name       string
	Env        string
	Cluster    string
	Job        string
	TeamName   string
	FirstCheck time.Time
	LastCheck  time.Time

	Details         scraper.Details
	ReportCreatedAt *time.Time
	HasReport       bool
}

const listTargetGroupsSQL = `
SELECT
    tg.id,
    tg.target_group,
    tg.env,
    tg.cluster,
    tg.job,
    tg.team_name,
    tg.first_check,
    tg.last_check,
    r.details,
    r.created_at
FROM target_groups tg
LEFT JOIN reports r
  ON r.target_group = tg.target_group
 AND r.env = tg.env
 AND r.cluster = tg.cluster
WHERE ($1 = '' OR tg.team_name = $1)
ORDER BY tg.team_name, tg.target_group, tg.env, tg.cluster;
`

func (s *Storage) ListTargetGroups(ctx context.Context, teamName string) ([]*TargetGroupWithReport, error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	rows, err := conn.Query(ctx, listTargetGroupsSQL, teamName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groups := make([]*TargetGroupWithReport, 0)
	for rows.Next() {
		item, err := scanTargetGroupWithReport(rows)
		if err != nil {
			return nil, err
		}
		groups = append(groups, item)
	}

	return groups, rows.Err()
}

const getTargetGroupByIDSQL = `
SELECT
    tg.id,
    tg.target_group,
    tg.env,
    tg.cluster,
    tg.job,
    tg.team_name,
    tg.first_check,
    tg.last_check,
    r.details,
    r.created_at
FROM target_groups tg
LEFT JOIN reports r
  ON r.target_group = tg.target_group
 AND r.env = tg.env
 AND r.cluster = tg.cluster
WHERE tg.id = $1;
`

func (s *Storage) GetTargetGroupByID(ctx context.Context, id int64) (*TargetGroupWithReport, error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	row := conn.QueryRow(ctx, getTargetGroupByIDSQL, id)
	item, err := scanTargetGroupWithReport(row)
	if err != nil {
		return nil, err
	}

	return item, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanTargetGroupWithReport(row scanner) (*TargetGroupWithReport, error) {
	item := &TargetGroupWithReport{}

	var (
		firstCheckRaw  pgtype.Timestamp
		lastCheckRaw   pgtype.Timestamp
		reportAtRaw    pgtype.Timestamp
		detailsRawJSON []byte
	)

	err := row.Scan(
		&item.ID,
		&item.Name,
		&item.Env,
		&item.Cluster,
		&item.Job,
		&item.TeamName,
		&firstCheckRaw,
		&lastCheckRaw,
		&detailsRawJSON,
		&reportAtRaw,
	)
	if err != nil {
		return nil, err
	}

	if firstCheckRaw.Valid {
		item.FirstCheck = firstCheckRaw.Time
	}
	if lastCheckRaw.Valid {
		item.LastCheck = lastCheckRaw.Time
	}

	if reportAtRaw.Valid {
		item.ReportCreatedAt = new(reportAtRaw.Time)
	}

	if len(detailsRawJSON) > 0 {
		if err = json.Unmarshal(detailsRawJSON, &item.Details); err != nil {
			return nil, err
		}
		item.HasReport = true
	}

	return item, nil
}
