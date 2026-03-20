package scraper

import "time"

type TargetGroup struct {
	ID         int
	Name       string
	Env        string
	Cluster    string
	FirstCheck time.Time
	LastCheck  time.Time
	Job        string
	TeamName   string
}
