package cron

import (
	"context"
	"errors"
	"testing"

	"git.server.lan/pkg/zaplogger/logger"
	"git.server.lan/pkg/zaplogger/zaploggercore"
)

type testTask struct {
	work func(ctx context.Context) error
	name string
}

func init() {
	logger.Init(zaploggercore.LogPretty)
}

func (t testTask) Work(ctx context.Context) error {
	return t.work(ctx)
}

func (t testTask) Name() string {
	return t.name
}

func TestManagerAddTask_DoesNotUseCanceledAddContext(t *testing.T) {
	manager := NewCronManager()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	called := false
	task := testTask{
		name: "test-task",
		work: func(workCtx context.Context) error {
			called = true
			if errors.Is(workCtx.Err(), context.Canceled) {
				t.Fatal("work context must not be canceled")
			}
			return nil
		},
	}

	if err := manager.AddTask(ctx, "* * * * * *", task); err != nil {
		t.Fatalf("add task: %v", err)
	}

	entries := manager.cron.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected one cron entry, got %d", len(entries))
	}

	entries[0].Job.Run()

	if !called {
		t.Fatal("task was not executed")
	}
}
