-- +goose Up
CREATE INDEX scrape_tasks_non_pending_created_at_idx
    ON scrape_tasks (created_at)
    WHERE status <> 'pending';

-- +goose Down
DROP INDEX scrape_tasks_non_pending_created_at_idx;
