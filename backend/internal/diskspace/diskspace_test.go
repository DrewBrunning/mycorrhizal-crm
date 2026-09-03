package diskspace

import (
	"errors"
	"strings"
	"testing"
)

func TestAvailable_RealFilesystemReportsPlausibleNumbers(t *testing.T) {
	free, total, err := Available(t.TempDir())
	if err != nil {
		t.Fatalf("Available: %v", err)
	}
	if total == 0 {
		t.Fatal("total bytes = 0, want a real filesystem size")
	}
	if free == 0 {
		t.Fatal("free bytes = 0, want some free space on the test filesystem")
	}
	if free > total {
		t.Fatalf("free %d > total %d", free, total)
	}
}

func TestAvailable_ErrorsOnUnstatablePath(t *testing.T) {
	_, _, err := Available("/definitely/not/a/real/path/xyzzy")
	if err == nil {
		t.Fatal("Available on a non-existent path should return the statfs error")
	}
	if !strings.Contains(err.Error(), "statfs") {
		t.Fatalf("error %q should mention statfs", err.Error())
	}
}

func TestRequire_RefusesWhenBelowNeed(t *testing.T) {
	restore := StubForTest(10 << 20) // 10 MiB free
	t.Cleanup(restore)

	err := Require("/data", 64<<20) // needs 64 MiB
	if err == nil {
		t.Fatal("Require returned nil, want ErrInsufficientSpace")
	}
	var ins *ErrInsufficientSpace
	if !errors.As(err, &ins) {
		t.Fatalf("error is %T, want *ErrInsufficientSpace", err)
	}
	if ins.Path != "/data" || ins.NeedBytes != 64<<20 || ins.FreeBytes != 10<<20 {
		t.Fatalf("unexpected fields: %+v", ins)
	}
	if !strings.Contains(err.Error(), "insufficient disk space") {
		t.Fatalf("message %q missing the operator-facing phrase", err.Error())
	}
}

func TestRequire_AllowsWhenEnoughFree(t *testing.T) {
	restore := StubForTest(1 << 30) // 1 GiB free
	t.Cleanup(restore)

	if err := Require("/data", 64<<20); err != nil {
		t.Fatalf("Require = %v, want nil", err)
	}
}

func TestRequire_DoesNotBlockWhenStatfsUnreadable(t *testing.T) {
	restore := StubErrorForTest(errors.New("statfs boom"))
	t.Cleanup(restore)

	// A broken statfs must not be the thing that fails an operation — the
	// operation keeps its own fail-closed path.
	if err := Require("/data", 1<<50); err != nil {
		t.Fatalf("Require = %v, want nil when statfs cannot be read", err)
	}
}
