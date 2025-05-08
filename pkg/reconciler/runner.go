package reconciler

import (
	"context"
	"github.com/cenkalti/backoff/v5"
	"time"
)

type Runner struct {
	FailBackOff backoff.BackOff
	WaitBackOff backoff.BackOff
	ReconcileFn func(ctx context.Context) error
}

type RunnerOption func(*Runner)

func WithFailBackOff(b backoff.BackOff) RunnerOption {
	return func(r *Runner) {
		r.FailBackOff = b
	}
}

func WithWaitBackOff(b backoff.BackOff) RunnerOption {
	return func(r *Runner) {
		r.WaitBackOff = b
	}
}

func (r *Runner) Loop(ctx context.Context) {
	for {
		err := r.ReconcileFn(ctx)
		if err != nil {
			time.Sleep(r.FailBackOff.NextBackOff())
			continue
		}
		r.FailBackOff.Reset()
		time.Sleep(r.WaitBackOff.NextBackOff())
		r.WaitBackOff.Reset()
	}
}

func NewRunner(
	reconcile func(ctx context.Context) error,
	options ...RunnerOption,
) *Runner {
	p := &Runner{
		ReconcileFn: reconcile,
		FailBackOff: &backoff.ExponentialBackOff{
			InitialInterval:     backoff.DefaultInitialInterval,
			RandomizationFactor: backoff.DefaultRandomizationFactor,
			Multiplier:          backoff.DefaultMultiplier,
			MaxInterval:         backoff.DefaultMaxInterval,
		},
		WaitBackOff: &backoff.ConstantBackOff{
			Interval: time.Hour,
		},
	}

	for _, option := range options {
		option(p)
	}

	return p
}

func RunReconciler(
	ctx context.Context,
	reconcile func(ctx context.Context) error,
	options ...RunnerOption,
) {
	p := NewRunner(reconcile, options...)
	go p.Loop(ctx)
}
