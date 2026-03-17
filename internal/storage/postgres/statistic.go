package postgres

import (
	"context"

	"git.server.lan/maksim/metric-sherlock-diploma/internal/scraper"
)

func (s *Storage) SaveStatistics(ctx context.Context, stats []*scraper.Statistic) error {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	panic("not implemented")
}
