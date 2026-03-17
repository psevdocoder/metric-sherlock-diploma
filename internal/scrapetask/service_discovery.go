package scrapetask

import (
	"os"

	"gopkg.in/yaml.v3"
)

type SDConfig struct {
	ScrapeConfigs []ScrapeConfig `yaml:"scrape_configs"`
}

type ScrapeConfig struct {
	JobName       string         `yaml:"job_name"`
	MetricsPath   string         `yaml:"metrics_path"`
	StaticConfigs []StaticConfig `yaml:"static_configs"`
}

type StaticConfig struct {
	Targets []string `yaml:"targets"`
	Labels  Labels   `yaml:"labels"`
}

type Labels struct {
	Env         string `yaml:"env"`
	TargetGroup string `yaml:"target_group"`
	Cluster     string `yaml:"cluster"`
}

func LoadSDConfig(filename string) (*SDConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var cfg SDConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
