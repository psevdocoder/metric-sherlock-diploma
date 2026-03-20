package scrapetask

import (
	"context"
	"fmt"
	"time"
)

const produceScrapeTasksJobName = "produce-scrape-tasks"

type taskStorage interface {
	SaveScrapeTasks(ctx context.Context, tasks []ScrapeTask) error
}

// Job фоновая задача для создания задач на сбор метрик.
// Обращается к Service Discovery, получает актуальные объекты, сохраняет их в очередь
type Job struct {
	taskStorage  taskStorage
	sdConfigPath string
}

// NewJob конструктор ProducerJob
func NewJob(taskStorage taskStorage, sdConfigPath string) *Job {
	return &Job{
		taskStorage:  taskStorage,
		sdConfigPath: sdConfigPath,
	}
}

// Name имплементирует интерфейс cron.Manager для возврата имени фоновой задачи
func (j *Job) Name() string {
	return produceScrapeTasksJobName
}

// Work имплементирует интерфейс cron.Manager для описания необходимых действий для создания задач на сбор метрик
func (j *Job) Work(ctx context.Context) error {
	sdConfig, err := LoadSDConfig(j.sdConfigPath)
	if err != nil {
		return err
	}

	scrapeTasks := make([]ScrapeTask, 0)

	for _, scrapeConfig := range sdConfig.ScrapeConfigs {
		for _, staticConfig := range scrapeConfig.StaticConfigs {
			addresses := make([]string, 0, len(staticConfig.Targets))

			for _, address := range staticConfig.Targets {
				addresses = append(addresses, fmt.Sprintf("http://%s%s", address, scrapeConfig.MetricsPath))
			}

			scrapeTasks = append(scrapeTasks, ScrapeTask{
				Status:      TaskStatusPending,
				CreatedAt:   time.Now(),
				Job:         scrapeConfig.JobName,
				Addresses:   addresses,
				TargetGroup: staticConfig.Labels.TargetGroup,
				Env:         staticConfig.Labels.Env,
				Cluster:     staticConfig.Labels.Cluster,
				TeamName:    staticConfig.Labels.TeamName,
			})
		}
	}

	return j.taskStorage.SaveScrapeTasks(ctx, scrapeTasks)
}
