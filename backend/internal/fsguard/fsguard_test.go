package fsguard

import (
	"errors"
	"strings"
	"testing"
)

// stubTypeProbe forces typeProbe to return exactly (magic, err) until the
// returned restore func runs. Test-only helper; call the restore in
// t.Cleanup, mirroring diskspace.StubForTest/StubErrorForTest.
func stubTypeProbe(magic int64, err error) (restore func()) {
	prev := typeProbe
	typeProbe = func(string) (int64, error) { return magic, err }
	return func() { typeProbe = prev }
}

func TestNetworkFilesystemWarning_RealLocalFilesystemIsNil(t *testing.T) {
	// t.TempDir() is always on local disk (or a FUSE-typed overlay in a
	// container, which this check deliberately does not flag either) —
	// either way it must never warn.
	if w := NetworkFilesystemWarning(t.TempDir()); w != nil {
		t.Fatalf("NetworkFilesystemWarning(local tempdir) = %+v, want nil", w)
	}
}

func TestNetworkFilesystemWarning_FlagsEveryKnownNetworkMagic(t *testing.T) {
	for magic, name := range knownNetworkFilesystems {
		restore := stubTypeProbe(magic, nil)
		w := NetworkFilesystemWarning("/data")
		restore()

		if w == nil {
			t.Fatalf("magic %#x (%s): NetworkFilesystemWarning = nil, want a warning", magic, name)
		}
		if w.FilesystemName != name {
			t.Fatalf("magic %#x: FilesystemName = %q, want %q", magic, w.FilesystemName, name)
		}
		if w.Path != "/data" {
			t.Fatalf("magic %#x: Path = %q, want %q", magic, w.Path, "/data")
		}
		if !strings.Contains(w.String(), name) || !strings.Contains(w.String(), "/data") {
			t.Fatalf("magic %#x: message %q missing path or filesystem name", magic, w.String())
		}
	}
}

func TestNetworkFilesystemWarning_DoesNotFlagFUSE(t *testing.T) {
	// FUSE-overlayfs backs an entirely-local Docker writable layer; flagging
	// FUSE would false-positive on ordinary containerized deployments.
	const fuseMagic = 0x65735546
	restore := stubTypeProbe(fuseMagic, nil)
	defer restore()

	if w := NetworkFilesystemWarning("/data"); w != nil {
		t.Fatalf("NetworkFilesystemWarning(FUSE) = %+v, want nil", w)
	}
}

func TestNetworkFilesystemWarning_UnknownMagicIsNil(t *testing.T) {
	restore := stubTypeProbe(0xDEADBEEF, nil)
	defer restore()

	if w := NetworkFilesystemWarning("/data"); w != nil {
		t.Fatalf("NetworkFilesystemWarning(unrecognized magic) = %+v, want nil", w)
	}
}

func TestNetworkFilesystemWarning_DoesNotFailWhenProbeErrors(t *testing.T) {
	// A statfs that cannot be read (or a non-Linux build, via errUnsupported)
	// must degrade to "no warning", never panic or error out — this check
	// has no fail-closed path of its own to fall back on.
	restore := stubTypeProbe(0, errors.New("statfs boom"))
	defer restore()

	if w := NetworkFilesystemWarning("/data"); w != nil {
		t.Fatalf("NetworkFilesystemWarning(probe error) = %+v, want nil", w)
	}
}

func TestStatfsType_UnsupportedPlatformStubReturnsError(t *testing.T) {
	// Exercises fsguard_other.go's errUnsupported path directly wherever this
	// test binary is actually built; on Linux this instead exercises the
	// real syscall.Statfs against a path that does exist (t.TempDir()), which
	// must succeed and return a plausible magic number rather than an error.
	magic, err := statfsType(t.TempDir())
	if err != nil {
		t.Skipf("statfsType unsupported on this platform: %v", err)
	}
	if magic == 0 {
		t.Fatal("statfsType returned magic 0 for a real, existing directory")
	}
}
