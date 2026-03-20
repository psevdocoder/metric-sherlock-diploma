package scraper

import (
	"context"
	"fmt"
	"io"
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

// countingReader кастомный ридер для подсчета размера ответа
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

func (c *MetricsClient) GetTargetMetrics(ctx context.Context, url string) (map[string]*dto.MetricFamily, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Error("Failed to close response body", zap.Error(err), zap.String("url", url))
		}
		c.client.CloseIdleConnections()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	cr := &countingReader{r: resp.Body}

	parser := expfmt.TextParser{}
	metrics, err := parser.TextToMetricFamilies(cr)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to parse metrics: %w", err)
	}

	return metrics, cr.n, nil
}
