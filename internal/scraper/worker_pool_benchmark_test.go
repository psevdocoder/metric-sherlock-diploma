package scraper

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"

	"git.server.lan/maksim/metric-sherlock-diploma/internal/scrapetask"
	"git.server.lan/pkg/zaplogger/logger"
	"git.server.lan/pkg/zaplogger/zaploggercore"
	"go.uber.org/zap/zapcore"
)

const benchmarkSeriesPerTarget = 10_000

var benchmarkLoggerInit sync.Once

func BenchmarkWorkerPoolProcessTargets10kSeries(b *testing.B) {
	for _, workers := range []int{1, 2, 4, 8} {
		b.Run(fmt.Sprintf("workers_%d", workers), func(b *testing.B) {
			benchmarkWorkerPoolProcessTargets(b, workers, benchmarkSeriesPerTarget)
		})
	}
}

func benchmarkWorkerPoolProcessTargets(b *testing.B, workers int, seriesPerTarget int) {
	b.Helper()
	benchmarkLoggerInit.Do(func() {
		logger.Init(zaploggercore.LogPretty)
		logger.SetLogLevel(zapcore.InfoLevel)
	})

	metricsPayload := buildMockMetricsPayload(seriesPerTarget)
	mockClient := &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"text/plain; version=0.0.4"},
				},
				Body: io.NopCloser(bytes.NewReader(metricsPayload)),
			}, nil
		}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	processor := NewProcessor(
		&MetricsClient{client: mockClient},
		LimitsConfig{
			MaxMetricNameLen:     1000,
			MaxLabelNameLen:      1000,
			MaxLabelValueLen:     1000,
			MaxMetricCardinality: seriesPerTarget + 1,
			MaxHistogramBuckets:  1000,
			MaxBytesWeight:       int64(len(metricsPayload)) + 1,
		},
		nil,
		nil,
	)

	pool := NewWorkerPool(ctx, processor, workers)
	defer pool.Stop()

	errCh := make(chan error, 1)
	done := make(chan struct{})

	b.ReportAllocs()
	b.SetBytes(int64(len(metricsPayload)))
	b.ResetTimer()

	go func() {
		defer close(done)

		for i := 0; i < b.N; i++ {
			res, ok := <-pool.Results()
			if !ok {
				errCh <- fmt.Errorf("worker pool results channel closed before reading all results")
				return
			}

			if res.err != nil {
				errCh <- fmt.Errorf("task %d failed: %w", res.taskID, res.err)
				return
			}
		}
	}()

	for i := 0; i < b.N; i++ {
		pool.Submit(&scrapetask.ScrapeTask{
			ID:          int64(i + 1),
			Job:         "benchmark-job",
			Addresses:   []string{"http://benchmark.local/metrics"},
			TargetGroup: "benchmark-target-group",
			Env:         "benchmark",
			Cluster:     "benchmark-cluster",
			TeamName:    "benchmark-team",
		})
	}

	<-done
	b.StopTimer()

	select {
	case err := <-errCh:
		b.Fatal(err)
	default:
	}

	elapsedNs := b.Elapsed().Nanoseconds()
	if elapsedNs > 0 {
		elapsedSec := float64(elapsedNs) / 1e9
		b.ReportMetric(float64(b.N)/elapsedSec, "targets/sec")
		b.ReportMetric(float64(b.N*seriesPerTarget)/elapsedSec, "series/sec")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func buildMockMetricsPayload(series int) []byte {
	var sb strings.Builder
	sb.Grow(series * 64)

	sb.WriteString("# HELP benchmark_mock_metric Mock metric for worker pool benchmark\n")
	sb.WriteString("# TYPE benchmark_mock_metric gauge\n")

	for i := 0; i < series; i++ {
		sb.WriteString("benchmark_mock_metric{series=\"")
		sb.WriteString(strconv.Itoa(i))
		sb.WriteString("\",target=\"mock-target\"} ")
		sb.WriteString(strconv.Itoa(i + 1))
		sb.WriteByte('\n')
	}

	return []byte(sb.String())
}
