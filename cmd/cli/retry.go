package main

import (
	"context"
	"errors"
	"time"

	"github.com/puppe1990/libgenbooks/internal/libgen"
)

// retryOptions bounds retryUnsynced: how many attempts and the base pause
// between them (the pause grows linearly with the attempt number).
type retryOptions struct {
	attempts int
	interval time.Duration
	// notice is called before each pause, for user-facing progress lines.
	notice func(attempt int, pause time.Duration)
}

// retryUnsynced retries a download while the CDN reports the file as not yet
// synchronized — a transient state that clears when the node finishes copying
// the file. Every other failure is returned immediately.
func retryUnsynced(ctx context.Context, opts retryOptions, attempt func(context.Context) (libgen.Result, error)) (libgen.Result, error) {
	if opts.attempts < 1 {
		opts.attempts = 1
	}
	result, err := attempt(ctx)
	for tries := 1; err != nil && tries < opts.attempts && errors.Is(err, libgen.ErrNotSynced); tries++ {
		pause := opts.interval * time.Duration(tries)
		if opts.notice != nil {
			opts.notice(tries, pause)
		}
		if waitErr := waitOrDone(ctx, pause); waitErr != nil {
			return libgen.Result{}, err
		}
		result, err = attempt(ctx)
	}
	return result, err
}

// waitOrDone pauses for the given duration and gives up as soon as the
// context is cancelled.
func waitOrDone(ctx context.Context, pause time.Duration) error {
	if pause <= 0 {
		return nil
	}
	timer := time.NewTimer(pause)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
