package fireandforget

import (
	"context"
	"testing"
	"time"
)

func TestWaitContext_ReturnsNilOnceTrackedWorkFinishes(t *testing.T) {
	release := make(chan struct{})
	Run(func() { <-release })
	close(release)
	if err := WaitContext(context.Background()); err != nil {
		t.Fatalf("WaitContext = %v, want nil", err)
	}
}

func TestWaitContext_GivesUpWhenContextEnds(t *testing.T) {
	release := make(chan struct{})
	Run(func() { <-release })
	defer func() { close(release); Wait() }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := WaitContext(ctx); err != context.Canceled {
		t.Fatalf("WaitContext = %v, want context.Canceled", err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := WaitContext(ctx); err != context.DeadlineExceeded {
		t.Fatalf("WaitContext = %v, want context.DeadlineExceeded", err)
	}
}
