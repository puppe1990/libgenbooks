package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/puppe1990/libgenbooks/internal/libgen"
)

func TestRetryUnsynced_RetriesUntilSynced(t *testing.T) {
	calls := 0
	attempt := func(context.Context) (libgen.Result, error) {
		calls++
		if calls < 3 {
			return libgen.Result{}, fmt.Errorf("download x: %w", libgen.ErrNotSynced)
		}
		return libgen.Result{Path: "/tmp/x.epub", Bytes: 3}, nil
	}

	result, err := retryUnsynced(context.Background(), retryOptions{attempts: 3, interval: time.Millisecond}, attempt)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
	if result.Path != "/tmp/x.epub" || result.Bytes != 3 {
		t.Fatalf("result = %+v", result)
	}
}

func TestRetryUnsynced_StopsOnOtherErrors(t *testing.T) {
	calls := 0
	attempt := func(context.Context) (libgen.Result, error) {
		calls++
		return libgen.Result{}, fmt.Errorf("download x: status 404")
	}

	_, err := retryUnsynced(context.Background(), retryOptions{attempts: 5, interval: time.Millisecond}, attempt)
	if err == nil {
		t.Fatal("err = nil, want the 404 back")
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 (only unsynced retries)", calls)
	}
}

func TestRetryUnsynced_ExhaustsAttempts(t *testing.T) {
	calls := 0
	attempt := func(context.Context) (libgen.Result, error) {
		calls++
		return libgen.Result{}, fmt.Errorf("download x: %w", libgen.ErrNotSynced)
	}

	_, err := retryUnsynced(context.Background(), retryOptions{attempts: 4, interval: time.Millisecond}, attempt)
	if !errors.Is(err, libgen.ErrNotSynced) {
		t.Fatalf("err = %v, want ErrNotSynced", err)
	}
	if calls != 4 {
		t.Fatalf("calls = %d, want 4", calls)
	}
}

func TestRetryUnsynced_CancelStopsTheWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	attempt := func(context.Context) (libgen.Result, error) {
		calls++
		return libgen.Result{}, fmt.Errorf("download x: %w", libgen.ErrNotSynced)
	}
	notices := 0
	opts := retryOptions{
		attempts: 10,
		interval: time.Hour,
		notice: func(int, time.Duration) {
			notices++
			cancel()
		},
	}

	_, err := retryUnsynced(ctx, opts, attempt)
	if !errors.Is(err, libgen.ErrNotSynced) {
		t.Fatalf("err = %v, want the pending ErrNotSynced", err)
	}
	if calls != 1 || notices != 1 {
		t.Fatalf("calls/notices = %d/%d, want 1/1", calls, notices)
	}
}
