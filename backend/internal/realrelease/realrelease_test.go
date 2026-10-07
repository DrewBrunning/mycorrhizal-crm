package realrelease

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedWithCurrentServer plays the "old release" with the current code: boots
// it over a fresh database, seeds + enrols 2FA + captures, and stops it. The
// real old releases are exercised by scripts/realrelease-leg.sh in CI; this
// proves the harness itself end to end without docker.
func seedWithCurrentServer(t *testing.T) (dbPath string, creds *Credentials, pre Snapshot) {
	t.Helper()
	dir := t.TempDir()
	dbPath = filepath.Join(dir, "mycorrhizal.db")
	ctx := context.Background()

	cur, err := StartCurrent(ctx, dbPath, "", "")
	require.NoError(t, err)
	c := NewClient(cur.BaseURL(), nil)
	creds, err = Seed(ctx, c)
	require.NoError(t, err)
	require.NoError(t, EnableTwoFactor(ctx, c, creds))
	pre, err = Capture(ctx, c)
	require.NoError(t, err)
	cur.Stop()
	return dbPath, creds, pre
}

func TestVerifyCleanUpgradeIsOK(t *testing.T) {
	dbPath, creds, pre := seedWithCurrentServer(t)

	var logs []string
	res, err := Verify(context.Background(), VerifyOptions{
		DBPath: dbPath, Creds: creds, Pre: pre,
		Logf: func(f string, a ...any) { logs = append(logs, f) },
	})
	require.NoError(t, err)
	assert.Empty(t, res.Diffs)
	assert.Empty(t, res.Problems)
	assert.True(t, res.OK())
	assert.NotEmpty(t, logs)

	// The seed really produced what the harness claims to cover.
	assert.Len(t, pre["contacts"], 6, "5 seeded + the registration self-contact")
	assert.NotEmpty(t, creds.RecoveryCodes)
	assert.NotEmpty(t, creds.APIToken)
	atts, _ := pre["attachment_files"].(map[string]any)
	assert.Len(t, atts, 1)
}

// The issue's hand-verify: damage only rows an old write path produced and
// the harness must name it.
func TestVerifyDetectsDataCorruptedByTheUpgrade(t *testing.T) {
	dbPath, creds, pre := seedWithCurrentServer(t)

	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE contacts SET firstname = '' WHERE firstname = 'Grace'`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	res, err := Verify(context.Background(), VerifyOptions{DBPath: dbPath, Creds: creds, Pre: pre})
	require.NoError(t, err)
	assert.False(t, res.OK())
	joined := ""
	for _, d := range append(append([]string{}, res.Diffs...), res.Problems...) {
		joined += d + "\n"
	}
	assert.Contains(t, joined, "firstname")
}

func TestVerifyDetectsLostSecondFactorState(t *testing.T) {
	dbPath, creds, pre := seedWithCurrentServer(t)

	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, err = db.Exec(`DELETE FROM api_tokens`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	res, err := Verify(context.Background(), VerifyOptions{DBPath: dbPath, Creds: creds, Pre: pre})
	require.NoError(t, err)
	assert.False(t, res.OK())
	assert.NotEmpty(t, res.Problems, "a dead API token must be a named problem")
}

func TestVerifyRejectsMissingInputs(t *testing.T) {
	_, err := Verify(context.Background(), VerifyOptions{})
	require.Error(t, err)
}

func TestVerifyReportsABootFailure(t *testing.T) {
	// A database the server cannot open (a directory).
	_, err := Verify(context.Background(), VerifyOptions{
		DBPath: t.TempDir(), Creds: &Credentials{}, Pre: Snapshot{},
	})
	require.Error(t, err)
}

func TestVerifyWithoutTwoFactor(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "mycorrhizal.db")
	ctx := context.Background()
	cur, err := StartCurrent(ctx, dbPath, "", "")
	require.NoError(t, err)
	c := NewClient(cur.BaseURL(), nil)
	creds, err := Seed(ctx, c)
	require.NoError(t, err)
	pre, err := Capture(ctx, c)
	require.NoError(t, err)
	// Login helper with no second factor.
	require.NoError(t, Login(ctx, NewClient(cur.BaseURL(), nil), creds))
	cur.Stop()

	res, err := Verify(ctx, VerifyOptions{DBPath: dbPath, Creds: creds, Pre: pre})
	require.NoError(t, err)
	assert.True(t, res.OK(), "%v %v", res.Diffs, res.Problems)
}

func TestLoginWithTwoFactor(t *testing.T) {
	dbPath, creds, _ := seedWithCurrentServer(t)
	cur, err := StartCurrent(context.Background(), dbPath, "", "")
	require.NoError(t, err)
	defer cur.Stop()
	require.NoError(t, Login(context.Background(), NewClient(cur.BaseURL(), nil), creds))

	bad := *creds
	bad.Password = "wrong-password-1!"
	require.Error(t, Login(context.Background(), NewClient(cur.BaseURL(), nil), &bad))
}

func TestHandlerTransportDrivesARouterInProcess(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/x" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ip":"` + r.RemoteAddr + `"}`))
			return
		}
		http.NotFound(w, r)
	})
	c := NewClient("http://inproc/", HandlerTransport(h))
	var out struct {
		IP string `json:"ip"`
	}
	require.NoError(t, c.call(context.Background(), http.MethodGet, "/x", nil, &out))
	assert.Equal(t, "127.0.0.1:40000", out.IP)

	err := c.call(context.Background(), http.MethodGet, "/missing", nil, nil)
	var ae *APIError
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, 404, ae.Status)
	assert.Contains(t, ae.Error(), "HTTP 404")
}

func TestCallErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	c := NewClient(srv.URL, nil)
	require.ErrorContains(t, c.call(context.Background(), http.MethodGet, "/x", nil, new(map[string]any)), "decoding response")
	srv.Close()
	require.Error(t, c.call(context.Background(), http.MethodGet, "/x", nil, nil), "connection refused")
	require.Error(t, c.upload(context.Background(), "/x", "f", []byte("x")))
	_, _, err := c.raw(context.Background(), "BAD METHOD", "/x", nil, nil)
	require.Error(t, err)
}

func TestListAllErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/nokey", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{}`)) })
	mux.HandleFunc("/api/v1/notlist", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"k":1}`)) })
	mux.HandleFunc("/api/v1/forever", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"k":[1],"next_cursor":"again"}`))
	})
	mux.HandleFunc("/api/v1/paged", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("cursor") == "" {
			_, _ = w.Write([]byte(`{"k":[1],"next_cursor":"p2"}`))
			return
		}
		_, _ = w.Write([]byte(`{"k":[2],"next_cursor":""}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := NewClient(srv.URL, nil)
	ctx := context.Background()

	_, err := c.listAll(ctx, "/nokey", "k", "")
	require.ErrorContains(t, err, "no \"k\" key")
	_, err = c.listAll(ctx, "/notlist", "k", "")
	require.ErrorContains(t, err, "not a list")
	_, err = c.listAll(ctx, "/forever", "k", "x=1")
	require.ErrorContains(t, err, "did not terminate")
	_, err = c.listAll(ctx, "/missing", "k", "")
	require.Error(t, err)
	items, err := c.listAll(ctx, "/paged", "k", "")
	require.NoError(t, err)
	assert.Len(t, items, 2)
}

func TestCompare(t *testing.T) {
	pre := Snapshot{
		"a":           "x",
		"n":           float64(1),
		"list":        []any{map[string]any{"k": "v"}},
		"next_cursor": "volatile-1",
		"obj":         map[string]any{"inner": "keep"},
	}
	t.Run("identical and additive are clean", func(t *testing.T) {
		post := Snapshot{
			"a": "x", "n": float64(1), "extra": "new field",
			"list":        []any{map[string]any{"k": "v", "added": 1}},
			"next_cursor": "volatile-2",
			"obj":         map[string]any{"inner": "keep"},
		}
		assert.Empty(t, Compare(pre, post))
	})
	t.Run("every kind of loss is named", func(t *testing.T) {
		post := Snapshot{
			"a":    "y",            // scalar changed
			"list": []any{},        // length changed
			"obj":  "now a string", // type changed (object)
			"n":    []any{},        // type changed (scalar -> list)
		}
		diffs := Compare(pre, post)
		joined := ""
		for _, d := range diffs {
			joined += d + "\n"
		}
		assert.Contains(t, joined, "$.a:")
		assert.Contains(t, joined, "list length 1 before, 0 after")
		assert.Contains(t, joined, "$.obj: was an object")
		assert.Contains(t, joined, "$.n:")
	})
	t.Run("a missing key and a list that became a scalar", func(t *testing.T) {
		diffs := Compare(Snapshot{"gone": "v", "l": []any{1.0}}, Snapshot{"l": "scalar"})
		joined := ""
		for _, d := range diffs {
			joined += d + "\n"
		}
		assert.Contains(t, joined, "missing after")
		assert.Contains(t, joined, "was a list")
	})
	t.Run("long values are abbreviated", func(t *testing.T) {
		long := make([]byte, 400)
		for i := range long {
			long[i] = 'a'
		}
		diffs := Compare(Snapshot{"k": string(long)}, Snapshot{"k": "b"})
		require.Len(t, diffs, 1)
		assert.Contains(t, diffs[0], "...")
	})
}

func TestWithoutAuthEvents(t *testing.T) {
	assert.Equal(t, "plain", withoutAuthEvents("plain"))
	got := withoutAuthEvents(map[string]any{"audit_events": []any{
		map[string]any{"entity_type": "auth"},
		map[string]any{"entity_type": "contact"},
		"not-a-map",
	}}).(map[string]any)
	assert.Len(t, got["audit_events"], 2)
	assert.Equal(t, float64(2), got["total"])
}

func TestTruncateAndEnv(t *testing.T) {
	assert.Equal(t, "ab", truncate("ab", 5))
	assert.Equal(t, "ab...", truncate("abcdef", 2))

	t.Setenv("REALRELEASE_TEST_KEEP", "orig")
	restore := setEnv(map[string]string{"REALRELEASE_TEST_KEEP": "new", "REALRELEASE_TEST_NEW": "v"})
	assert.Equal(t, "new", os.Getenv("REALRELEASE_TEST_KEEP"))
	restore()
	assert.Equal(t, "orig", os.Getenv("REALRELEASE_TEST_KEEP"))
	assert.Equal(t, "", os.Getenv("REALRELEASE_TEST_NEW"))
}
