package soak

import (
	"fmt"
	"strings"
)

const (
	kib = 1024.0
	mib = 1024 * kib
)

// Budgets is the committed soak threshold table (issue #1496), the soak
// counterpart of internal/perfbench/testdata/budgets.json: hand-authored, one
// reasoned entry per signal. It is checked in code rather than JSON so a
// number and its Reason edit in the same hunk; docs/development/soak-baseline.md
// renders this table and a drift test fails until the two agree.
//
// What a number means: KindGrowth is the *fitted growth over the tail window*
// (the last 2/3 of the run — the first third is warm-up), so the same figure
// governs the 60 s per-PR smoke and the 45 min nightly, and a steady state
// that is truly flat passes both. A leak of fixed rate shows ~linearly more
// growth in the longer run, which is why the long form is the sensitive one.
// Limits are set a wide multiple above the noise a healthy run was measured to
// show (see soak-baseline.md) and far below what even a one-per-request leak
// produces at the workload's offered rate.
//
// Changing a number is a deliberate, reviewed edit that must rewrite Reason.
var Budgets = []Budget{
	{
		Signal: SigGoroutines, Kind: KindGrowth, Limit: 12, Unit: "goroutines",
		Reason: "A healthy server holds a flat goroutine population (HTTP keep-alive connections, the scheduler, the cleanup ticker, the bounded async-audit and webhook workers). Request- or job-scoped goroutines that never exit (an unbounded fire-and-forget, a leaked client transport) grow by ~1 per leaking operation. 12 absorbs connection-pool jitter and in-flight work at the sample instant.",
	},
	{
		Signal: SigOpenFDs, Kind: KindGrowth, Limit: 6, Unit: "fds",
		Reason: "Open descriptors track connections plus a few files (db, wal, shm, logs) and are flat in steady state. A leaked response body, file or SQLite connection grows by one per leak. 6 absorbs keep-alive connection churn between two samples.",
	},
	{
		Signal: SigHeapInuse, Kind: KindGrowth, Limit: 24 * mib, Unit: "bytes",
		Reason: "Live heap after a forced GC (in-process mode) is bounded by the working set: the dataset is capped per user, so a growing heap means retained garbage (an unbounded map or slice keyed by request/user, a cache without eviction). 24 MiB clears allocator/size-class noise and the SQLite page cache settling.",
	},
	{
		Signal: SigRSS, Kind: KindGrowth, Limit: 64 * mib, Unit: "bytes",
		Reason: "Resident memory follows the heap but also moves with page cache, goroutine stacks and the Go scavenger's release timing, so it is noisier than heap_inuse and given a looser bound (a healthy 60 s run fits under 1 MiB). It is the backstop for memory the Go heap does not account for (cgo, mmap); heap_inuse is the sensitive probe for Go-heap leaks.",
	},
	{
		Signal: SigWALBytes, Kind: KindCeiling, Limit: 16 * mib, Unit: "bytes",
		Reason: "SQLite's auto-checkpoint (1000 pages, ~4 MiB) resets the write-ahead log whenever no reader pins it. A healthy run peaks near 4-5 MiB; a WAL past 16 MiB under this workload is not being checkpointed — a long-lived read transaction (checkpoint starvation) or a disabled auto-checkpoint — and would grow without bound on a real instance.",
	},
	{
		Signal: SigLimiterEntries, Kind: KindGrowth, Limit: 20, Unit: "keys",
		Reason: "Per-IP and per-account limiter maps are evicted by the 5-minute cleanup ticker. The workload keeps its client set and its failed-login identifiers fixed, so the tracked-key count must be flat; per-request growth means state keyed by something unbounded that never evicts. 20 allows the failed-login lockout entries and known-good-IP records to settle.",
	},
	{
		Signal: SigDBConnsOpen, Kind: KindCeiling, Limit: 25, Unit: "connections",
		Reason: "The pool is bounded; an open-connection count that exceeds it is a leaked *sql.Rows/*sql.Tx pinning connections. 25 sits above the observed steady-state pool under the offered concurrency.",
	},
	{
		Signal: SigDBBytes, Kind: KindGrowth, Limit: 96 * mib, Unit: "bytes",
		Advisory: true,
		Reason:   "The database legitimately grows during a run — every request writes audit rows, soft-deleted contacts stay recoverable until the retention purge, and the FTS index tracks live rows — so this is reported, not failed. It is bounded loosely so a runaway (a write loop, an un-purged table) is still visible in the report.",
	},
	{
		Signal: SigP95Read, Kind: KindDegradation, Optional: true, Limit: 0.25, Ratio: 4, Unit: "s",
		Reason: "Read latency must not degrade with process lifetime: the tail median p95 may be at most 4x the first third's plus 250 ms. This is a relative growth class, not a wall-clock budget — CI hardware speed cancels out — and catches an index/derived-data decay (FTS fragmentation, a growing scan) that only shows after many writes.",
	},
	{
		Signal: SigP95Write, Kind: KindDegradation, Optional: true, Limit: 1.0, Ratio: 4, Unit: "s",
		Reason: "Writes serialise on the single SQLite writer, so their p95 is dominated by queueing and fsync and is noisier; the allowance is 4x the first third's plus 1 s. A WAL that stops checkpointing or a lock held across a request shows here as a ratchet.",
	},
	{
		Signal: SigP95Auth, Kind: KindDegradation, Optional: true, Limit: 1.0, Ratio: 4, Unit: "s",
		Reason: "Login/logout are bcrypt-bound (deliberately slow, CPU-contended with the rest of the workload), so the allowance is 4x the first third's plus 1 s. Growth beyond that means session/limiter state is accumulating on the auth path.",
	},
}

// RequiredSignals are the signals whose absence from /metrics is itself a
// failure (a gauge that silently stopped being exported must not read as "no
// growth"). Linux-only ones (rss, open_fds) are required too: the soak runs on
// Linux CI and in the production image.
var RequiredSignals = []string{
	SigRSS, SigHeapInuse, SigGoroutines, SigOpenFDs, SigWALBytes, SigDBBytes, SigLimiterEntries, SigDBConnsOpen,
}

// DefaultBudgets returns a copy of the committed table, so a caller (or a
// test) that tightens one never mutates the shared slice.
func DefaultBudgets() []Budget {
	return append([]Budget(nil), Budgets...)
}

// fmtLimit renders a budget limit with its unit for the docs table: byte
// limits in MiB, everything else as-is.
func fmtLimit(b Budget) string {
	switch {
	case b.Unit == "bytes":
		return fmt.Sprintf("%.0f MiB", b.Limit/mib)
	case b.Kind == KindDegradation:
		return fmt.Sprintf("%.4g x + %.4g %s", b.Ratio, b.Limit, b.Unit)
	default:
		return fmt.Sprintf("%.4g %s", b.Limit, b.Unit)
	}
}

// BudgetsMarkdown renders the committed budget table — the deterministic half
// of docs/development/soak-baseline.md, drift-tested against the doc.
func BudgetsMarkdown() string {
	var sb strings.Builder
	sb.WriteString("| Signal | Judged as | Limit | Gating | Why |\n")
	sb.WriteString("|---|---|---|---|---|\n")
	for _, b := range Budgets {
		gate := "fails the run"
		if b.Advisory {
			gate = "advisory"
		}
		fmt.Fprintf(&sb, "| `%s` | %s | %s | %s | %s |\n", b.Signal, kindLabel(b.Kind), fmtLimit(b), gate, b.Reason)
	}
	return sb.String()
}

// Markers delimiting the generated budget table in
// docs/development/soak-baseline.md.
const (
	DocBudgetBegin = "<!-- soak:budgets:begin -->"
	DocBudgetEnd   = "<!-- soak:budgets:end -->"
)

// ReplaceBudgetBlock returns doc with the text between the budget markers
// replaced by the current BudgetsMarkdown.
func ReplaceBudgetBlock(doc string) (string, error) {
	i := strings.Index(doc, DocBudgetBegin)
	j := strings.Index(doc, DocBudgetEnd)
	if i < 0 || j < i {
		return "", fmt.Errorf("document has no %s ... %s block", DocBudgetBegin, DocBudgetEnd)
	}
	return doc[:i+len(DocBudgetBegin)] + "\n" + BudgetsMarkdown() + doc[j:], nil
}

func kindLabel(k Kind) string {
	switch k {
	case KindCeiling:
		return "ceiling (tail max)"
	case KindDegradation:
		return "degradation (tail vs first third)"
	default:
		return "fitted growth over the tail window"
	}
}
