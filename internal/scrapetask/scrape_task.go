package scrapetask

import "time"

type TaskStatus string

const (
	TaskStatusPending  TaskStatus = "pending"
	TaskStatusRunning  TaskStatus = "running"
	TaskStatusComplete TaskStatus = "complete"
	TaskStatusError    TaskStatus = "error"
)

type ScrapeTask struct {
	ID          int64
	Status      TaskStatus
	CreatedAt   time.Time
	Job         string
	Addresses   []string
	TargetGroup string
	Env         string
	Cluster     string
	TeamName    string
}
