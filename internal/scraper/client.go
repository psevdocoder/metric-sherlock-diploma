package scraper

import (
	"context"
	"fmt"
	"net/http"

	"git.server.lan/pkg/zaplogger/logger"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"go.uber.org/zap"
)

type MetricsClient struct {
	client *http.Client
}

func NewMetricsClient() *MetricsClient {
	client := &http.Client{}

	return &MetricsClient{
		client: client,
	}
}

func (c *MetricsClient) GetTargetMetrics(ctx context.Context, url string) (map[string]*dto.MetricFamily, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Error("Failed to close response body", zap.Error(err), zap.String("url", url))
		}

		c.client.CloseIdleConnections()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	parser := expfmt.TextParser{}
	return parser.TextToMetricFamilies(resp.Body)
}
