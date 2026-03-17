package cron

import "context"

// Task интерфейс, которому должны удовлетворять cron задачи для их выполнения из Manager
type Task interface {
	Work(ctx context.Context) error
	Name() string
}
