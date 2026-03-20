package postgres

import (
	"context"
	"encoding/json"
	"time"

	"git.server.lan/maksim/metric-sherlock-diploma/internal/scraper"
	"github.com/jackc/pgx/v5"
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
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	total := 0
	batch := &pgx.Batch{}

	createdAt := time.Now()

	for _, report := range reports {
		detailsBytes, err := json.Marshal(report.Details)
		if err != nil {
			return err
		}

		batch.Queue(saveReportsSQL, report.TargetGroup, report.Env, report.Cluster, report.TeamName, detailsBytes, createdAt)

		total++

		if batch.Len() >= rowsPerBatch || total == len(reports) {
			if err = s.sendBatch(ctx, batch, conn.Conn()); err != nil {
				return err
			}

			batch = &pgx.Batch{}
		}
	}

	return nil
}
