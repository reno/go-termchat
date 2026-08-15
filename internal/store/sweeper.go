package store

import (
	"context"
	"time"
)

type Sweeper struct {
	store    Store
	clock    Clock
	interval time.Duration
	onSweep  func(SweepStats)
}

type SweeperOption func(*Sweeper)

func WithClock(c Clock) SweeperOption {
	return func(s *Sweeper) { s.clock = c }
}

func WithSweepHook(fn func(SweepStats)) SweeperOption {
	return func(s *Sweeper) { s.onSweep = fn }
}

func NewSweeper(st Store, interval time.Duration, opts ...SweeperOption) *Sweeper {
	s := &Sweeper{store: st, clock: SystemClock{}, interval: interval}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Sweeper) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := s.Once(ctx); err != nil {
				return err
			}
		}
	}
}

func (s *Sweeper) Once(ctx context.Context) (SweepStats, error) {
	stats, err := s.store.Sweep(ctx, s.clock.Now())
	if err != nil {
		return SweepStats{}, err
	}
	if s.onSweep != nil {
		s.onSweep(stats)
	}
	return stats, nil
}
