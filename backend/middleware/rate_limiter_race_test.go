package middleware

import (
	"sync"
	"testing"
	"time"
)

// TestConfigureAPIRateLimiterIsRaceFree pins issue #1565: the init-started
// sweeper reads the package-level IP limiters while ConfigureAPIRateLimiter
// replaces apiLimiter. Under -race this fails if the globals are plain
// pointers instead of atomic ones.
func TestConfigureAPIRateLimiterIsRaceFree(t *testing.T) {
	before := apiLimiter.Load()
	defer apiLimiter.Store(before)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { // the sweeper tick body, in a tight loop
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				sweepLimiters()
			}
		}
	}()
	go func() { // a /metrics-style reader
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = apiLimiter.Load().EntryCount()
			}
		}
	}()

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		ConfigureAPIRateLimiter(time.Millisecond, 10)
	}
	close(stop)
	wg.Wait()

	if got := apiLimiter.Load(); got == before || got == nil {
		t.Fatalf("apiLimiter not replaced by ConfigureAPIRateLimiter: %p", got)
	}
}
