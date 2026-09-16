package config

import (
	"os"
	"testing"
)

// committedReferenceRel is the generated artifact, relative to this package
// (backend/config -> repo docs/).
const committedReferenceRel = "../../docs/configuration-reference.md"

// TestConfigRegisterCoversEveryField is the completeness gate #933 asks for:
// every exported field of Config (recursing into nested config structs like
// OIDCConfig) must carry a cfgreg struct tag that parses. A field added
// without one fails this test, not silently ships undocumented.
func TestConfigRegisterCoversEveryField(t *testing.T) {
	t.Parallel()
	errs := RegisterErrors()
	for _, err := range errs {
		t.Error(err)
	}
	if len(errs) > 0 {
		t.Fatalf("%d Config field(s) missing or have a malformed `cfgreg` tag — see errors above", len(errs))
	}
}

// TestConfigurationReferenceDocUpToDate is the drift test (same pattern as
// backend/correspondence/matrix_test.go): docs/configuration-reference.md
// must be exactly what RenderRegister() produces right now, so a cfgreg tag
// change shows up as a reviewable doc diff.
func TestConfigurationReferenceDocUpToDate(t *testing.T) {
	t.Parallel()
	got := RenderRegister()
	committed, err := os.ReadFile(committedReferenceRel)
	if err != nil {
		t.Fatalf("reading committed reference %s: %v", committedReferenceRel, err)
	}
	if string(committed) != got {
		t.Errorf("%s is stale: it no longer matches the cfgreg tags — "+
			"regenerate with `cd backend && go run ./cmd/genconfigreference` and commit the diff",
			committedReferenceRel)
	}
}

// TestRegisterTagParsing pins the cfgreg tag parser's behavior directly,
// independent of the real Config struct, so a parser bug is caught even if
// every real field happens to be tagged correctly.
func TestRegisterTagParsing(t *testing.T) {
	t.Parallel()

	t.Run("valid env field", func(t *testing.T) {
		e, err := parseCfgTag("Port", "env=PORT;type=int;range=1..65535;default=8080;required=false;restart=true;desc=HTTP listen port")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if e.EnvVar != "PORT" || e.Type != "int" || e.Range != "1..65535" || e.Default != "8080" ||
			e.Required || !e.Restart || e.Desc != "HTTP listen port" || e.Derived {
			t.Errorf("unexpected entry: %+v", e)
		}
	})

	t.Run("valid derived field", func(t *testing.T) {
		e, err := parseCfgTag("UseResend", "derived=true;desc=true when both are set")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !e.Derived || e.EnvVar != "" || e.Desc != "true when both are set" {
			t.Errorf("unexpected entry: %+v", e)
		}
	})

	t.Run("desc may contain reserved characters", func(t *testing.T) {
		e, err := parseCfgTag("X", "env=X;type=string;default=;required=false;restart=true;desc=a=b, and more")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if e.Desc != "a=b, and more" {
			t.Errorf("desc = %q, want %q", e.Desc, "a=b, and more")
		}
	})

	t.Run("empty tag", func(t *testing.T) {
		if _, err := parseCfgTag("X", ""); err == nil {
			t.Error("expected an error for an empty tag")
		}
	})

	t.Run("missing desc", func(t *testing.T) {
		if _, err := parseCfgTag("X", "env=X;type=string"); err == nil {
			t.Error("expected an error for a tag with no desc")
		}
	})

	t.Run("neither env nor derived", func(t *testing.T) {
		if _, err := parseCfgTag("X", "type=string;desc=x"); err == nil {
			t.Error("expected an error when neither env nor derived=true is set")
		}
	})

	t.Run("env without type", func(t *testing.T) {
		if _, err := parseCfgTag("X", "env=X;desc=x"); err == nil {
			t.Error("expected an error when env is set but type is missing")
		}
	})

	t.Run("derived with env is contradictory", func(t *testing.T) {
		if _, err := parseCfgTag("X", "env=X;derived=true;desc=x"); err == nil {
			t.Error("expected an error when derived=true also sets env")
		}
	})

	t.Run("malformed pair", func(t *testing.T) {
		if _, err := parseCfgTag("X", "env;desc=x"); err == nil {
			t.Error("expected an error for a pair with no '='")
		}
	})

	t.Run("unknown key", func(t *testing.T) {
		if _, err := parseCfgTag("X", "env=X;type=string;bogus=1;desc=x"); err == nil {
			t.Error("expected an error for an unknown cfgreg key")
		}
	})
}
