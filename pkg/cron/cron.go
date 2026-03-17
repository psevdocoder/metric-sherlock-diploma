package cron

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"git.server.lan/pkg/zaplogger/logger"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

var ErrSpecifiedTaskNotFound = errors.New("specified task not found")

// Manager представляет структуру для выполнения фоновых задач по cron
type Manager struct {
	cron    *cron.Cron
	mu      sync.Mutex
	entries map[string]cron.EntryID
}

// NewCronManager конструктор для Manager
func NewCronManager() *Manager {
	return &Manager{
		cron:    cron.New(),
		mu:      sync.Mutex{},
		entries: make(map[string]cron.EntryID),
	}
}

// AddTask добавляет задачу с расписанием по cron выражению
func (m *Manager) AddTask(ctx context.Context, spec string, task Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.entries[task.Name()]; exists {
		return fmt.Errorf("task %s already exists", task.Name())
	}

	id, err := m.cron.AddFunc(spec, func() {
		logger.Info("Task execution started", zap.String("name", task.Name()))
		start := time.Now()
		if err := task.Work(ctx); err != nil {
			logger.Error("Task failed", zap.String("name", task.Name()), zap.Error(err))
			return
		}

		logger.Info("Task executed", zap.String("name", task.Name()), zap.Duration("duration", time.Since(start)))
	})

	if err != nil {
		return err
	}

	m.entries[task.Name()] = id

	logger.Info("Added task", zap.String("name", task.Name()))
	return nil
}

// RemoveTask удаляет задачу по имени
func (m *Manager) RemoveTask(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	id, exists := m.entries[name]
	if !exists {
		return ErrSpecifiedTaskNotFound
	}

	m.cron.Remove(id)
	delete(m.entries, name)

	logger.Info("Removed task", zap.String("name", name))

	return nil
}

// Start запускает cron
func (m *Manager) Start() {
	m.cron.Start()
}

// Stop корректно останавливает cron
func (m *Manager) Stop(ctx context.Context) error {
	stopCtx := m.cron.Stop()

	select {
	case <-stopCtx.Done():
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
