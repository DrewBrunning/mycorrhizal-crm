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
	env, err := NewEnv(EnvOptions{Profile: largedata.Smoke, WorkDir: b.TempDir(), OpenMigrated: migratedOpener(b)})
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

// BenchmarkDataMovement is the PERF-03 analogue: it runs every non-isolating
// data-movement operation once per sub-benchmark against the smoke fixture so
// the operation bodies stay compiled and executed under the same
// `-bench . -benchtime=1x` sweep. The regression gate is
// TestDataMovementBenchmarks / TestDataMovementAtScale (rows touched +
// memory-growth class), not wall-clock.
func BenchmarkDataMovement(b *testing.B) {
	env, err := NewEnv(EnvOptions{Profile: largedata.Smoke, WorkDir: b.TempDir(), OpenMigrated: migratedOpener(b)})
	if err != nil {
		b.Fatalf("building smoke env: %v", err)
	}
	b.Cleanup(env.Close)

	for _, op := range DataMovementRegistry() {
		if op.Isolate {
			// An isolating op mutates its fixture; the shared-env benchmark
			// skips it (TestDataMovementBenchmarks still measures it on a fork).
			continue
		}
		op := op
		b.Run(op.Name, func(b *testing.B) {
			wd := b.TempDir()
			if op.Prepare != nil {
				if err := op.Prepare(env, wd); err != nil {
					b.Fatalf("prepare: %v", err)
				}
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, _, err := op.Run(env, wd); err != nil {
					b.Fatalf("run: %v", err)
				}
			}
		})
	}
}
