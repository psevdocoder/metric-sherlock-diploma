package postgres

import (
	"context"

	"git.server.lan/maksim/metric-sherlock-diploma/internal/scraper"
	scrapetask "git.server.lan/maksim/metric-sherlock-diploma/internal/scrapetask"
	"github.com/jackc/pgx/v5"
)

const saveScrapeTasksSQL = `
INSERT INTO scrape_tasks (status, created_at, job, address, target_id, cluster, env, target_group)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`

// SaveScrapeTasks сохраняет задачи на сбор метрик
func (s *Storage) SaveScrapeTasks(ctx context.Context, tasks []scrapetask.ScrapeTask) error {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	total := 0
	batch := &pgx.Batch{}

	for _, task := range tasks {
		batch.Queue(
			saveScrapeTasksSQL,
			task.Status,
			task.CreatedAt,
			task.Job,
			task.Address,
			task.TargetID,
			task.Cluster,
			task.Env,
			task.TargetGroup,
		)

		total++

		if batch.Len() >= rowsPerBatch || total == len(tasks) {
			if err = s.sendBatch(ctx, batch, conn.Conn()); err != nil {
				return err
			}

			batch = &pgx.Batch{}
		}
	}

	return nil
}

const getScrapeTasksSQL = `
WITH tasks AS (
    SELECT id
    FROM scrape_tasks
    WHERE status = 'pending'
    ORDER BY created_at
    LIMIT $1
    FOR UPDATE SKIP LOCKED
)
UPDATE scrape_tasks st
SET status = 'running'
FROM tasks
WHERE st.id = tasks.id
RETURNING 
    st.id,
    st.status,
    st.created_at,
    st.job,
    st.address,
    st.target_id,
    st.cluster,
    st.env,
    st.target_group;
`

func (s *Storage) GetScrapeTasks(ctx context.Context, limit int) ([]*scrapetask.ScrapeTask, error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	rows, err := conn.Query(ctx, getScrapeTasksSQL, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := make([]*scrapetask.ScrapeTask, 0)

	for rows.Next() {
		t := &scrapetask.ScrapeTask{}

		err = rows.Scan(
			&t.Status,
			&t.CreatedAt,
			&t.Job,
			&t.Address,
			&t.TargetID,
			&t.Cluster,
			&t.Env,
			&t.TargetGroup,
		)
		if err != nil {
			return nil, err
		}

		tasks = append(tasks, t)
	}

	return tasks, rows.Err()
}

const updateTaskStatusesSQL = `
UPDATE scrape_tasks t
SET status = v.status
FROM (SELECT
	UNNEST($1::bigint[]) AS id,
	UNNEST($2::text[])   AS status
) v
WHERE t.id = v.id;
`

func (s *Storage) UpdateTaskStatuses(ctx context.Context, updates []*scraper.TaskStatusUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	ids := make([]int64, len(updates))
	statuses := make([]string, len(updates))

	for i, u := range updates {
		ids[i] = u.ID
		statuses[i] = string(u.Status)
	}

	_, err = conn.Exec(ctx, updateTaskStatusesSQL, ids, statuses)
	return err
}
