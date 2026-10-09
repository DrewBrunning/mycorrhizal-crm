package meerkat

import (
	"os"
	"path/filepath"
	"testing"

	"mycorrhizal/internal/meerkatfixture"
)

// maxMeerkatFuzzInput bounds what FuzzMeerkatReader writes to disk. The target
// is a panic/crash tripwire for the reader, not a wall-clock benchmark; parser
// bugs are shape-driven, not size-driven. Real uploads are separately bounded
// upstream, and the SQLite header check rejects most oversized noise before
// the driver ever opens it — but a valid-header file is written in full and
// handed to sqlite, so an unbounded input is still work the fuzzer need not
// do. Same rationale as the vcard3/vcard4/jscontact input caps (#265/#376).
const maxMeerkatFuzzInput = 1 << 20 // 1 MiB

// FuzzMeerkatReader covers issue #1625's Meerkat reader target. meerkat.Open
// is the untrusted-input boundary for the direct-DB import path (ADR 0007):
// the uploaded file is an arbitrary SQLite database from a source deployment
// whose schema may be any migration version, so the reader's schema-drift
// tolerance and sqlite driver interaction are exactly what a hostile/malformed
// file exercises.
//
// Open takes a path (there is no bytes-level reader), so each input is written
// to a per-worker source file and read back through the real entry point. The
// seed corpus is a real Meerkat-schema database built from the shared manifest
// by the same loader the import tests use.
//
// The harness only needs to not panic; go test -fuzz catches panics
// automatically. The extra check below pins Open's documented contract: it
// never returns a nil snapshot alongside a nil error (a caller would nil-deref)
// and never a non-nil snapshot alongside an error.
func FuzzMeerkatReader(f *testing.F) {
	m, err := meerkatfixture.Read()
	if err != nil {
		f.Fatalf("read meerkat fixture manifest: %v", err)
	}
	seedPath := filepath.Join(f.TempDir(), "meerkat-seed.db")
	if err := meerkatfixture.Populate(seedPath, m); err != nil {
		f.Fatalf("populate meerkat fixture database: %v", err)
	}
	seed, err := os.ReadFile(seedPath)
	if err != nil {
		f.Fatalf("read meerkat fixture database: %v", err)
	}
	f.Add(seed)
	// The other edge: a bare SQLite header is the minimum a file must have to
	// get past the magic check and reach the driver with no schema at all.
	f.Add([]byte("SQLite format 3\x00"))
	f.Add([]byte("not a database at all"))

	// One source file per worker, overwritten each input. f.TempDir() is
	// per-process (each fuzz worker re-runs this function in its own process),
	// so this is race-free — and unlike t.TempDir() inside the callback it does
	// not accumulate a fresh directory per execution for the whole run (the
	// fuzz callback's *testing.T lives for the entire worker, so those dirs are
	// never removed until it exits — that cliff stalled an earlier version at
	// ~19k execs).
	path := filepath.Join(f.TempDir(), "source.db")

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > maxMeerkatFuzzInput {
			return
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatalf("write fuzz input: %v", err)
		}
		snap, err := Open(path)
		if err == nil && snap == nil {
			t.Fatalf("Open returned a nil snapshot with a nil error for input %q", data)
		}
		if err != nil && snap != nil {
			t.Fatalf("Open returned a non-nil snapshot alongside error %v for input %q", err, data)
		}
	})
}
