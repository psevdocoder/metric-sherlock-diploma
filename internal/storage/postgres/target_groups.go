package postgres

import (
	"context"
	"time"

	"git.server.lan/maksim/metric-sherlock-diploma/internal/scraper"
	"github.com/jackc/pgx/v5"
)

const saveOrUpdateTargetGroupsSQL = `
INSERT INTO target_groups (first_check, last_check, job, env, cluster, target_group, team_name)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (target_group, env, cluster) DO UPDATE
SET
    last_check = excluded.last_check
`

func (s *Storage) SaveOrUpdateTargetGroups(ctx context.Context, targetGroups []*scraper.TargetGroup) error {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	total := 0
	batch := &pgx.Batch{}

	now := time.Now()

	for _, targetGroup := range targetGroups {
		batch.Queue(
			saveOrUpdateTargetGroupsSQL,
			now,
			now,
			targetGroup.Job,
			targetGroup.Env,
			targetGroup.Cluster,
			targetGroup.Name,
			targetGroup.TeamName,
		)

		total++

		if batch.Len() >= rowsPerBatch || total == len(targetGroups) {
			if err = s.sendBatch(ctx, batch, conn.Conn()); err != nil {
				return err
			}
			batch = &pgx.Batch{}
		}
	}

	return nil

}
