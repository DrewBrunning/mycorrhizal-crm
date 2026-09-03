// Command loadsmoke fires parallel contact create/update/delete requests at
// a running instance for a short fixed duration and fails if any response
// shows either of the two live bug classes CLAUDE.md backend trap #9 and
// docker-compose.test.yml's own comments document:
//
//   - SQLITE_BUSY / "database is locked" under concurrent writes (fixed via
//     _txlock=immediate, pinned in-process by
//     database/concurrent_write_test.go — but never exercised against the
//     actual deployed artifact: nginx + backend under supervisord, the
//     all-in-one image).
//   - The production rate limit producing spurious 429s under sustained
//     load (the reason docker-compose.test.yml raises
//     API_RATE_LIMIT_INTERVAL_MS/API_RATE_LIMIT_BURST in the first place).
//
// It exists so a regression that reintroduces either — someone removing
// _txlock=immediate, or a new write path that upgrades a deferred
// transaction, or a lowered test rate limit — fails CI instead of shipping
// unnoticed (issue #262). Run via `go run ./cmd/loadsmoke` against the
// docker-compose.test.yml backend (see .github/workflows/e2e-tests.yml,
// which already brings that instance up for the Playwright suite — this
// reuses it rather than standing up a second workflow).
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"mycorrhizal/database"
)

type loadConfig struct {
	baseURL     string
	workers     int
	duration    time.Duration
	minRequests int64
	// users is how many distinct authenticated accounts to spread the workers
	// across (issue #498's "many users on one instance" axis). 1 keeps the
	// original single-shared-session behaviour — one user's browser firing
	// overlapping writes. >1 registers that many accounts and binds each
	// worker to one round-robin, so the contention is cross-user writers on
	// the shared tables and FTS indexes against the one SQLite writer.
	users int
	// dbPath, when set, is a SQLite database file the tool runs
	// PRAGMA integrity_check against after the load finishes. Only usable
	// where the load generator can see the server's database file (a local
	// run, or a CI job with a bind mount) — the docker-compose e2e job checks
	// integrity via `docker compose exec` instead.
	dbPath string
}

// defaultLoadConfig mirrors cmd/backup's env-var-with-defaults style.
var defaultLoadConfig = loadConfig{
	baseURL:     "http://localhost:7300",
	workers:     20,
	duration:    10 * time.Second,
	minRequests: 50,
	users:       1,
}

// loadConfigFromEnv reads LOADSMOKE_* overrides via getenv (injected so this
// is testable without mutating real process env). An invalid override falls
// back to the default rather than failing the run — this tool's job is to
// generate load and check responses, not to be a second config-validation
// surface.
func loadConfigFromEnv(getenv func(string) string) loadConfig {
	cfg := defaultLoadConfig
	if v := getenv("LOADSMOKE_BASE_URL"); v != "" {
		cfg.baseURL = strings.TrimRight(v, "/")
	}
	if v := getenv("LOADSMOKE_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.workers = n
		}
	}
	if v := getenv("LOADSMOKE_DURATION"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.duration = d
		}
	}
	if v := getenv("LOADSMOKE_MIN_REQUESTS"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			cfg.minRequests = n
		}
	}
	if v := getenv("LOADSMOKE_USERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.users = n
		}
	}
	if v := getenv("LOADSMOKE_DB_PATH"); v != "" {
		cfg.dbPath = v
	}
	return cfg
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg := loadConfigFromEnv(os.Getenv)

	clients, err := registerSessions(cfg.baseURL, cfg.users)
	if err != nil {
		return err
	}

	fmt.Printf("loadsmoke: %d workers across %d user(s) for %s against %s\n",
		cfg.workers, len(clients), cfg.duration, cfg.baseURL)
	result := runLoad(clients, cfg.baseURL, cfg.workers, cfg.duration)
	fmt.Printf("loadsmoke: %d requests, %d busy, %d rate-limited, %d other errors\n",
		result.total, result.busy, result.rateLimited, result.other)

	if result.total < cfg.minRequests {
		return fmt.Errorf("only %d requests completed in %s — load generation itself may be broken (want >= %d)", result.total, cfg.duration, cfg.minRequests)
	}
	if result.busy > 0 {
		return fmt.Errorf("%d/%d requests returned SQLITE_BUSY/\"database is locked\" under concurrent writes — samples:\n%s",
			result.busy, result.total, strings.Join(result.busySamples, "\n"))
	}
	if result.rateLimited > 0 {
		return fmt.Errorf("%d/%d requests were rate-limited (429) against the raised test limit — samples:\n%s",
			result.rateLimited, result.total, strings.Join(result.rateLimitedSamples, "\n"))
	}

	// Issue #498: a concurrent write storm must never leave the database
	// corrupt. When the load generator can see the database file, prove it.
	if err := verifyNoCorruption(cfg.dbPath); err != nil {
		return err
	}
	return nil
}

// verifyNoCorruption runs PRAGMA integrity_check against dbPath, if one was
// configured (LOADSMOKE_DB_PATH). An empty path is a no-op — the docker-based
// e2e job checks integrity via `docker compose exec` instead, since the
// database is inside the container there.
func verifyNoCorruption(dbPath string) error {
	if dbPath == "" {
		return nil
	}
	res, err := database.IntegrityCheck(dbPath)
	if err != nil {
		return fmt.Errorf("post-load PRAGMA integrity_check on %s: %w", dbPath, err)
	}
	if res != "ok" { // # pragma: no cover — a genuinely corrupt SQLite file is not reproducible in a unit test; the assertion is what matters
		return fmt.Errorf("post-load PRAGMA integrity_check on %s reported: %s", dbPath, res)
	}
	fmt.Printf("loadsmoke: post-load integrity_check on %s = ok\n", dbPath)
	return nil
}

// registerSessions creates n throwaway accounts and returns one authenticated
// *http.Client per account. Each client carries its own cookie jar, so the
// sessions are independent; http.Client + Jar are documented safe for
// concurrent use, so one client is still shared across the workers bound to
// it.
func registerSessions(baseURL string, n int) ([]*http.Client, error) {
	if n < 1 {
		n = 1
	}
	clients := make([]*http.Client, n)
	base := time.Now().UnixNano()
	for i := 0; i < n; i++ {
		jar, err := cookiejar.New(nil)
		if err != nil { // # pragma: no cover — cookiejar.New(nil) never returns an error
			return nil, fmt.Errorf("create cookie jar: %w", err)
		}
		c := &http.Client{Jar: jar, Timeout: 10 * time.Second}
		username := fmt.Sprintf("loadsmoke-%d-%d", base, i)
		if err := registerAndLogin(c, baseURL, username); err != nil {
			return nil, fmt.Errorf("register/login test user %d/%d: %w", i+1, n, err)
		}
		clients[i] = c
	}
	return clients, nil
}

// registerAndLogin creates a throwaway user and authenticates as them. The
// client's cookie jar then carries auth_token for every subsequent request,
// including the concurrent ones runLoad fires — http.Client and its Jar are
// documented safe for concurrent use, so this one authenticated session is
// shared across all worker goroutines rather than one per worker (mirroring
// the trap #9 scenario: one user's browser firing overlapping writes).
func registerAndLogin(client *http.Client, baseURL, username string) error {
	registerBody := map[string]string{
		"username": username,
		"email":    username + "@example.com",
		"password": "CorrectHorseBattery9!",
	}
	if body, status, err := doJSON(client, http.MethodPost, baseURL+"/api/v1/register", registerBody); err != nil {
		return err
	} else if status != http.StatusCreated {
		return fmt.Errorf("register: status %d: %s", status, body)
	}

	loginBody := map[string]string{
		"identifier": username,
		"password":   "CorrectHorseBattery9!",
	}
	if body, status, err := doJSON(client, http.MethodPost, baseURL+"/api/v1/login", loginBody); err != nil {
		return err
	} else if status != http.StatusOK {
		return fmt.Errorf("login: status %d: %s", status, body)
	}
	return nil
}

const maxSamples = 5

// tally aggregates runLoad's outcome across every worker goroutine. All
// counters are atomic; the sample slices are mutex-protected and capped at
// maxSamples so a total failure doesn't produce an unreadable error message.
type tally struct {
	total, busy, rateLimited, other int64

	mu                 sync.Mutex
	busySamples        []string
	rateLimitedSamples []string
}

func (t *tally) record(method, url string, status int, body []byte, err error) {
	atomic.AddInt64(&t.total, 1)
	if err != nil {
		atomic.AddInt64(&t.other, 1)
		return
	}
	switch classify(status, body) {
	case classifyBusy:
		atomic.AddInt64(&t.busy, 1)
		t.addSample(&t.busySamples, method, url, status, body)
	case classifyRateLimited:
		atomic.AddInt64(&t.rateLimited, 1)
		t.addSample(&t.rateLimitedSamples, method, url, status, body)
	case classifyOther:
		atomic.AddInt64(&t.other, 1)
	}
}

func (t *tally) addSample(samples *[]string, method, url string, status int, body []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(*samples) < maxSamples {
		*samples = append(*samples, fmt.Sprintf("%s %s -> %d: %s", method, url, status, truncate(body, 200)))
	}
}

func (t *tally) snapshot() loadResult {
	t.mu.Lock()
	defer t.mu.Unlock()
	return loadResult{
		total:              atomic.LoadInt64(&t.total),
		busy:               atomic.LoadInt64(&t.busy),
		rateLimited:        atomic.LoadInt64(&t.rateLimited),
		other:              atomic.LoadInt64(&t.other),
		busySamples:        append([]string(nil), t.busySamples...),
		rateLimitedSamples: append([]string(nil), t.rateLimitedSamples...),
	}
}

// loadResult is the final, read-only outcome of one runLoad call.
type loadResult struct {
	total, busy, rateLimited, other int64
	busySamples, rateLimitedSamples []string
}

// runLoad hammers POST/PUT/DELETE /api/v1/contacts with `workers` concurrent
// goroutines for `duration`, each looping create-update-delete on its own
// contact (never touching another worker's row, so any lock contention
// observed is purely from concurrent writers on the same table/database, not
// two workers racing the same row). Workers are spread round-robin across
// `clients` — one entry is the original single-user shape; several entries is
// the many-users contention profile (issue #498).
func runLoad(clients []*http.Client, baseURL string, workers int, duration time.Duration) loadResult {
	t := &tally{}
	deadline := time.Now().Add(duration)

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			client := clients[worker%len(clients)]
			for i := 0; time.Now().Before(deadline); i++ {
				loadIteration(client, baseURL, worker, i, t)
			}
		}(w)
	}
	wg.Wait()

	return t.snapshot()
}

// loadIteration runs one create-update-delete cycle against baseURL,
// recording each of the three responses into t.
func loadIteration(client *http.Client, baseURL string, worker, iter int, t *tally) {
	name := fmt.Sprintf("Loadsmoke-%d-%d", worker, iter)
	createBody := map[string]interface{}{
		"card": map[string]interface{}{
			"name": map[string]interface{}{
				"components": []map[string]string{
					{"kind": "given", "value": name},
					{"kind": "surname", "value": "Test"},
				},
			},
		},
		"crm": map[string]interface{}{},
	}

	createURL := baseURL + "/api/v1/contacts"
	body, status, err := doJSON(client, http.MethodPost, createURL, createBody)
	t.record(http.MethodPost, createURL, status, body, err)
	if err != nil || status != http.StatusCreated {
		return
	}

	var created struct {
		Contact struct {
			ID float64 `json:"id"`
		} `json:"contact"`
	}
	if jsonErr := json.Unmarshal(body, &created); jsonErr != nil || created.Contact.ID == 0 {
		return
	}

	itemURL := fmt.Sprintf("%s/api/v1/contacts/%.0f", baseURL, created.Contact.ID)
	body, status, err = doJSON(client, http.MethodPut, itemURL, createBody)
	t.record(http.MethodPut, itemURL, status, body, err)

	body, status, err = doJSON(client, http.MethodDelete, itemURL, nil)
	t.record(http.MethodDelete, itemURL, status, body, err)
}

func truncate(b []byte, n int) string {
	s := string(b)
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

type classification int

const (
	classifyOK classification = iota
	classifyBusy
	classifyRateLimited
	classifyOther
)

// classify maps one HTTP response to the outcome runLoad cares about. A 2xx
// is always ok; a 500 mentioning either sentinel string trap #9 documents is
// "busy"; a 429 is "rate limited"; anything else unexpected (a validation
// error, a transient 404, etc.) is "other" — logged toward the total but not
// itself a failure, since this tool's job is to catch the two specific
// documented regressions, not to be a general-purpose API fuzzer.
func classify(status int, body []byte) classification {
	if status >= 200 && status < 300 {
		return classifyOK
	}
	if status == http.StatusTooManyRequests {
		return classifyRateLimited
	}
	if status == http.StatusInternalServerError &&
		(bytes.Contains(body, []byte("database is locked")) || bytes.Contains(body, []byte("SQLITE_BUSY"))) {
		return classifyBusy
	}
	return classifyOther
}

// doJSON marshals body (nil for no body) as JSON, issues the request, and
// returns the parsed status/response body. The response body is read fully
// and closed inside here (the *http.Response is deliberately not returned:
// no caller uses it, and returning it made bodyclose flag every call site for
// a body that is in fact drained and closed). Go's http.Client requires the
// body be drained either way to let the connection be reused.
func doJSON(client *http.Client, method, url string, body interface{}) ([]byte, int, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal request body: %w", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("%s %s: %w", method, url, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response body: %w", err)
	}
	return respBody, resp.StatusCode, nil
}
