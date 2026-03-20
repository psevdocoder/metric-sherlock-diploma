package scraper

import (
	"context"
	"sync/atomic"
	"time"

	"git.server.lan/maksim/metric-sherlock-diploma/internal/scrapetask"
	"git.server.lan/pkg/zaplogger/logger"
	"go.uber.org/zap"
)

const (
	waitNewTasksInterval = 5 * time.Second
	batchFlushInterval   = 5 * time.Second
	batchSize            = 1000

	tasksPerRequest = 10
)

type TaskStatusUpdate struct {
	ID     int64
	Status scrapetask.TaskStatus
}

type taskStorage interface {
	GetScrapeTasks(ctx context.Context, limit int) ([]*scrapetask.ScrapeTask, error)
	UpdateTaskStatuses(ctx context.Context, updates []*TaskStatusUpdate) error
}

type statisticStorage interface {
	SaveReports(ctx context.Context, stats []*Report) error
}

type targetGroupStorage interface {
	SaveOrUpdateTargetGroups(ctx context.Context, groups []*TargetGroup) error
}

type TaskConsumer struct {
	taskStorage        taskStorage
	statisticStorage   statisticStorage
	targetGroupStorage targetGroupStorage
	workerPool         *WorkerPool
	stopCh             chan struct{}
	isLeader           atomic.Bool
}

func NewTaskConsumer(
	taskStorage taskStorage,
	statisticStorage statisticStorage,
	targetGroupStorage targetGroupStorage,
	workerPool *WorkerPool,
	isLeader bool,
) *TaskConsumer {
	c := &TaskConsumer{
		taskStorage:        taskStorage,
		statisticStorage:   statisticStorage,
		targetGroupStorage: targetGroupStorage,
		workerPool:         workerPool,
		stopCh:             make(chan struct{}),
	}

	c.isLeader.Store(isLeader)

	return c
}

func (c *TaskConsumer) SetLeader(v bool) {
	c.isLeader.Store(v)
}

func (c *TaskConsumer) IsLeader() bool {
	return c.isLeader.Load()
}

func (c *TaskConsumer) Run(ctx context.Context) {
	statBatch := make([]*Report, 0, batchSize)
	statusBatch := make([]*TaskStatusUpdate, 0, batchSize)
	targetGroupBatch := make([]*TargetGroup, 0, batchSize)

	flushTicker := time.NewTicker(batchFlushInterval)
	taskTicker := time.NewTicker(waitNewTasksInterval)

	defer flushTicker.Stop()
	defer taskTicker.Stop()

	defer func() {
		if len(statBatch) > 0 {
			if err := c.statisticStorage.SaveReports(ctx, statBatch); err != nil {
				logger.Error("Failed to flush statistics on shutdown", zap.Error(err))
			}
		}

		if len(statusBatch) > 0 {
			if err := c.taskStorage.UpdateTaskStatuses(ctx, statusBatch); err != nil {
				logger.Error("Failed to flush task statuses on shutdown", zap.Error(err))
			}
		}

		if len(targetGroupBatch) > 0 {
			if err := c.targetGroupStorage.SaveOrUpdateTargetGroups(ctx, targetGroupBatch); err != nil {
				logger.Error("Failed to flush target groups on shutdown", zap.Error(err))
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Task consumer stopping: context cancelled")
			c.workerPool.Stop()
			return

		case <-c.stopCh:
			logger.Info("Task consumer stopping: stop signal received")
			c.workerPool.Stop()
			return

		case res, ok := <-c.workerPool.Results():
			if !ok {
				logger.Info("Worker pool results channel closed")
				return
			}

			if res.err != nil {
				statusBatch = append(statusBatch, &TaskStatusUpdate{
					ID:     res.taskID,
					Status: scrapetask.TaskStatusError,
				})

				logger.Error(
					"Worker returned error",
					zap.Int64("task_id", res.taskID),
					zap.Error(res.err),
				)
			} else {
				statusBatch = append(statusBatch, &TaskStatusUpdate{
					ID:     res.taskID,
					Status: scrapetask.TaskStatusComplete,
				})

				if res.stat != nil {
					statBatch = append(statBatch, res.stat)
				}

				if len(res.targetGroups) > 0 {
					targetGroupBatch = append(targetGroupBatch, res.targetGroups...)
				}
			}

			if len(statBatch) >= batchSize {
				if err := c.statisticStorage.SaveReports(ctx, statBatch); err != nil {
					logger.Error("Failed to save statistics batch", zap.Error(err))
				} else {
					statBatch = statBatch[:0]
				}
			}

			if len(statusBatch) >= batchSize {
				if err := c.taskStorage.UpdateTaskStatuses(ctx, statusBatch); err != nil {
					logger.Error("Failed to update task statuses", zap.Error(err))
				} else {
					statusBatch = statusBatch[:0]
				}
			}

			if len(targetGroupBatch) >= batchSize {
				if err := c.targetGroupStorage.SaveOrUpdateTargetGroups(ctx, targetGroupBatch); err != nil {
					logger.Error("Failed to save target groups batch", zap.Error(err))
				} else {
					targetGroupBatch = targetGroupBatch[:0]
				}
			}

		case <-flushTicker.C:
			if len(statBatch) > 0 {
				if err := c.statisticStorage.SaveReports(ctx, statBatch); err != nil {
					logger.Error("Failed to flush statistics", zap.Error(err))
				} else {
					statBatch = statBatch[:0]
				}
			}

			if len(statusBatch) > 0 {
				if err := c.taskStorage.UpdateTaskStatuses(ctx, statusBatch); err != nil {
					logger.Error("Failed to flush task statuses", zap.Error(err))
				} else {
					statusBatch = statusBatch[:0]
				}
			}

			if len(targetGroupBatch) > 0 {
				if err := c.targetGroupStorage.SaveOrUpdateTargetGroups(ctx, targetGroupBatch); err != nil {
					logger.Error("Failed to flush target groups", zap.Error(err))
				} else {
					targetGroupBatch = targetGroupBatch[:0]
				}
			}

		case <-taskTicker.C:
			if c.isLeader.Load() {
				continue
			}

			tasks, err := c.taskStorage.GetScrapeTasks(ctx, tasksPerRequest)
			if err != nil {
				logger.Error("Failed to get scrape tasks", zap.Error(err))
				continue
			}

			if len(tasks) == 0 {
				continue
			}

			logger.Debug("Task consumer new iteration")

			for _, task := range tasks {
				c.workerPool.Submit(task)
			}
		}
	}
}

func (c *TaskConsumer) Stop() {
	select {
	case <-c.stopCh:
		return
	default:
		close(c.stopCh)
	}
}
