package scrapetask

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	cronpkg "git.server.lan/maksim/metric-sherlock-diploma/pkg/cron"
	"git.server.lan/pkg/zaplogger/logger"
	"go.uber.org/zap"
)

type scheduleProvider interface {
	GetProduceTasksCronExpr(ctx context.Context) (string, error)
}

type cronManager interface {
	AddTask(ctx context.Context, spec string, task cronpkg.Task) error
	RemoveTask(name string) error
}

type Scheduler struct {
	manager  cronManager
	task     cronpkg.Task
	provider scheduleProvider

	mu          sync.Mutex
	currentSpec string
}

func NewScheduler(manager cronManager, task cronpkg.Task, provider scheduleProvider) *Scheduler {
	return &Scheduler{
		manager:  manager,
		task:     task,
		provider: provider,
	}
}

func (s *Scheduler) Init(ctx context.Context) error {
	spec, err := s.provider.GetProduceTasksCronExpr(ctx)
	if err != nil {
		return err
	}

	return s.UpdateSchedule(ctx, spec)
}

func (s *Scheduler) UpdateSchedule(ctx context.Context, spec string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if spec == s.currentSpec {
		return nil
	}

	prevSpec := s.currentSpec

	if err := s.manager.RemoveTask(s.task.Name()); err != nil && !errors.Is(err, cronpkg.ErrSpecifiedTaskNotFound) {
		return fmt.Errorf("failed to remove existing task from scheduler: %w", err)
	}

	if err := s.manager.AddTask(ctx, spec, s.task); err != nil {
		if prevSpec != "" {
			if rollbackErr := s.manager.AddTask(ctx, prevSpec, s.task); rollbackErr != nil {
				logger.Error(
					"Failed to rollback scrape task schedule",
					zap.String("task", s.task.Name()),
					zap.String("schedule", prevSpec),
					zap.Error(rollbackErr),
				)
			}
		}
		return fmt.Errorf("failed to add task with new schedule: %w", err)
	}

	s.currentSpec = spec
	return nil
}

func (s *Scheduler) Sync(ctx context.Context) error {
	spec, err := s.provider.GetProduceTasksCronExpr(ctx)
	if err != nil {
		return err
	}

	return s.UpdateSchedule(ctx, spec)
}

func (s *Scheduler) RunSyncLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.Sync(ctx); err != nil {
				logger.Error("Failed to sync scrape task schedule from etcd", zap.Error(err))
			}
		}
	}
}
