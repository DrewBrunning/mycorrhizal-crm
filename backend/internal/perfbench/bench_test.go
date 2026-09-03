package perfbench

import (
	"testing"

	"mycorrhizal/internal/largedata"
)

// BenchmarkCoreOperations exercises every registered operation once per
// sub-benchmark against the smoke fixture. CI runs it via
// `go test -run '^$' -bench . -benchtime=1x ./...` (unit-tests.yml,
// backend-checks) so the operation bodies stay compiled and executed; the
// actual regression gate is TestCoreOperationBenchmarks' query-count and
// growth assertions, not wall-clock, per issue #261.
func BenchmarkCoreOperations(b *testing.B) {
	env, err := NewEnv(EnvOptions{Profile: largedata.Smoke, WorkDir: b.TempDir()})
	if err != nil {
		b.Fatalf("building smoke env: %v", err)
	}
	b.Cleanup(env.Close)

	for _, op := range Registry() {
		if op.Destructive {
			// A destructive op consumes its fixture; skip it in the shared-env
			// benchmark (TestProfileBenchmarksAtScale still measures it).
			continue
		}
		op := op
		b.Run(op.Name, func(b *testing.B) {
			if op.Prepare != nil {
				if err := op.Prepare(env); err != nil {
					b.Fatalf("prepare: %v", err)
				}
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := op.Run(env); err != nil {
					b.Fatalf("run: %v", err)
				}
			}
		})
	}
}
