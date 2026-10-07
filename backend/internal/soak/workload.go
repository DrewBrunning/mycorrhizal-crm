package soak

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/textproto"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// password is the throwaway password every soak account registers with.
const password = "CorrectHorseBattery9!"

// badLoginPool is how many distinct identifiers the failed-login op cycles
// through. It is deliberately a small FIXED pool: the account limiter keeps
// one entry per failed identifier, so a bounded pool keeps that map flat in a
// healthy run and any per-request growth in the limiter stands out against it.
const badLoginPool = 5

// op names, the keys of Workload.OpCounts and the report's op table.
const (
	opList     = "contacts.list"
	opGet      = "contacts.get"
	opSearch   = "search"
	opCreate   = "contacts.create"
	opUpdate   = "contacts.update"
	opDelete   = "contacts.delete"
	opUndo     = "audit.undo"
	opNote     = "notes.create"
	opActivity = "activities.create"
	opExport   = "export.csv"
	opImport   = "import.vcf"
	opLogin    = "auth.login_cycle"
	opBadLogin = "auth.bad_login"
)

// opWeights is the mix, per 1000 operations. Read-heavy like a real CRM
// session, with the periodic heavy operations the issue names (login/logout,
// import, export, contact delete/undo) at a low rate.
var opWeights = []struct {
	name   string
	weight int
}{
	{opList, 230},
	{opGet, 120},
	{opSearch, 150},
	{opCreate, 110},
	{opUpdate, 110},
	{opDelete, 60},
	{opUndo, 20},
	{opNote, 60},
	{opActivity, 60},
	{opExport, 8},
	{opImport, 8},
	{opLogin, 50},
	{opBadLogin, 14},
}

// WorkloadConfig tunes the driver.
type WorkloadConfig struct {
	BaseURL string
	// Users is how many distinct accounts the load is spread across.
	Users int
	// Rate is the target aggregate requests-ish (operations) per second.
	Rate float64
	// Seed makes the operation sequence reproducible.
	Seed int64
	// Client overrides the HTTP client template (timeout / transport); nil
	// builds a default.
	Client *http.Client
}

// maxSamples bounds the retained failure samples so a total failure stays
// readable.
const maxSamples = 8

// user is one authenticated account. mu serialises the login/logout cycle
// against the ordinary operations that share the cookie jar.
type user struct {
	name   string
	client *http.Client
	mu     sync.RWMutex

	idsMu sync.Mutex
	ids   []uint // live contact ids this user created
	// lastUpdated is a contact id this user updated (audit-undo target).
	lastUpdated uint
	// pristine maps a live contact id this user created and never touched
	// again to its name; gone holds the names of such contacts once deleted.
	// They feed the end-of-run search-freshness probe: a live pristine name
	// must be findable, a gone one must not (updates and undo rename a
	// contact, so those are excluded rather than reasoned about).
	pristine map[uint]string
	gone     []string
}

func (u *user) addID(id uint, name string) {
	u.idsMu.Lock()
	u.ids = append(u.ids, id)
	if u.pristine == nil {
		u.pristine = map[uint]string{}
	}
	u.pristine[id] = name
	u.idsMu.Unlock()
}

// touched removes id from the probe set (it was renamed or reverted).
func (u *user) touched(id uint) {
	u.idsMu.Lock()
	delete(u.pristine, id)
	u.lastUpdated = id
	u.idsMu.Unlock()
}

// deleted moves id's name to the gone set when it was still pristine.
func (u *user) deleted(id uint) {
	u.idsMu.Lock()
	if name, ok := u.pristine[id]; ok {
		u.gone = append(u.gone, name)
		delete(u.pristine, id)
	}
	u.idsMu.Unlock()
}

// pick returns a random live id (0 when none).
func (u *user) pick(r *rand.Rand) uint {
	u.idsMu.Lock()
	defer u.idsMu.Unlock()
	if len(u.ids) == 0 {
		return 0
	}
	return u.ids[r.Intn(len(u.ids))]
}

// take removes and returns a random live id (0 when none).
func (u *user) take(r *rand.Rand) uint {
	u.idsMu.Lock()
	defer u.idsMu.Unlock()
	if len(u.ids) == 0 {
		return 0
	}
	i := r.Intn(len(u.ids))
	id := u.ids[i]
	u.ids = append(u.ids[:i], u.ids[i+1:]...)
	return id
}

func (u *user) count() int {
	u.idsMu.Lock()
	defer u.idsMu.Unlock()
	return len(u.ids)
}

// Workload is a running mixed-load driver. Counters are atomic; Run is the
// only writer of the sample slices (under mu).
type Workload struct {
	cfg   WorkloadConfig
	users []*user

	total       atomic.Int64
	rateLimited atomic.Int64
	serverErrs  atomic.Int64
	dropped     atomic.Int64
	counterMu   sync.Mutex
	opCounts    map[string]int64

	sampleMu sync.Mutex
	samples  []string

	nameSeq atomic.Int64
}

// NewWorkload registers the accounts and returns a ready driver.
func NewWorkload(ctx context.Context, cfg WorkloadConfig) (*Workload, error) {
	if cfg.Users < 1 {
		cfg.Users = 1
	}
	if cfg.Rate <= 0 {
		cfg.Rate = 20
	}
	w := &Workload{
		cfg:      cfg,
		opCounts: map[string]int64{},
	}
	stamp := time.Now().UnixNano()
	for i := 0; i < cfg.Users; i++ {
		jar, err := cookiejar.New(nil)
		if err != nil { // # pragma: no cover — cookiejar.New(nil) never errors
			return nil, fmt.Errorf("cookie jar: %w", err)
		}
		c := &http.Client{Jar: jar, Timeout: 20 * time.Second}
		if cfg.Client != nil {
			c.Transport = cfg.Client.Transport
			if cfg.Client.Timeout > 0 {
				c.Timeout = cfg.Client.Timeout
			}
		}
		u := &user{name: fmt.Sprintf("soak-%d-%d", stamp, i), client: c}
		if err := w.register(ctx, u); err != nil {
			return nil, fmt.Errorf("register soak user %d/%d: %w", i+1, cfg.Users, err)
		}
		w.users = append(w.users, u)
	}
	return w, nil
}

// doRetry429 is do, retried while the production auth limiter answers 429
// (2 requests/s sustained, burst 50 per IP): setup registers every account
// from one IP, and several runs in one process share the bucket.
func (w *Workload) doRetry429(ctx context.Context, c *http.Client, method, path string, body any) ([]byte, int, error) {
	for attempt := 0; ; attempt++ {
		b, st, err := w.do(ctx, c, method, path, body)
		if err != nil || st != http.StatusTooManyRequests || attempt >= 20 {
			return b, st, err
		}
		select {
		case <-ctx.Done():
			return b, st, ctx.Err()
		case <-time.After(600 * time.Millisecond):
		}
	}
}

func (w *Workload) register(ctx context.Context, u *user) error {
	body, status, err := w.doRetry429(ctx, u.client, http.MethodPost, "/api/v1/register", map[string]string{
		"username": u.name, "email": u.name + "@example.com", "password": password,
	})
	if err != nil {
		return err
	}
	if status != http.StatusCreated {
		return fmt.Errorf("register: status %d: %s", status, truncate(body, 200))
	}
	return w.login(ctx, u)
}

func (w *Workload) login(ctx context.Context, u *user) error {
	body, status, err := w.doRetry429(ctx, u.client, http.MethodPost, "/api/v1/login", map[string]string{
		"identifier": u.name, "password": password,
	})
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("login: status %d: %s", status, truncate(body, 200))
	}
	return nil
}

// do issues one JSON request and returns the drained body and status.
func (w *Workload) do(ctx context.Context, c *http.Client, method, path string, body any) ([]byte, int, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil { // # pragma: no cover — callers pass plain maps
			return nil, 0, fmt.Errorf("marshal: %w", err)
		}
		rd = bytes.NewReader(b)
	}
	return w.send(ctx, c, method, path, "application/json", rd, body != nil)
}

func (w *Workload) send(ctx context.Context, c *http.Client, method, path, contentType string, rd io.Reader, hasBody bool) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, w.cfg.BaseURL+path, rd)
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	if hasBody {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read body: %w", err)
	}
	return b, resp.StatusCode, nil
}

func truncate(b []byte, n int) string {
	s := string(b)
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

// record tallies one response. Only 5xx (a server fault) and transport errors
// count as failures; 4xx outside 429 are expected churn (a contact another op
// just deleted, a validation reject) — this tool judges server health, not
// API semantics.
func (w *Workload) record(op, method, path string, status int, body []byte, err error) {
	w.total.Add(1)
	w.counterMu.Lock()
	w.opCounts[op]++
	w.counterMu.Unlock()
	switch {
	case err != nil:
		w.serverErrs.Add(1)
		w.addSample(fmt.Sprintf("%s %s -> transport error: %v", method, path, err))
	case status == http.StatusTooManyRequests:
		w.rateLimited.Add(1)
	case status >= 500:
		w.serverErrs.Add(1)
		w.addSample(fmt.Sprintf("%s %s -> %d: %s", method, path, status, truncate(body, 200)))
	}
}

func (w *Workload) addSample(s string) {
	w.sampleMu.Lock()
	defer w.sampleMu.Unlock()
	if len(w.samples) < maxSamples {
		w.samples = append(w.samples, s)
	}
}

func contactBody(name string) map[string]any {
	return map[string]any{
		"card": map[string]any{
			"name": map[string]any{
				"components": []map[string]string{
					{"kind": "given", "value": name},
					{"kind": "surname", "value": "Soak"},
				},
			},
		},
		"crm": map[string]any{},
	}
}

func (w *Workload) newName(u *user) string {
	// Letters only: the search tokenizer splits on digits/punctuation
	// inconsistently, and a unique alphabetic token is exact for the probe.
	return "Soakname" + alpha(w.nameSeq.Add(1)) + "x" + alpha(int64(len(u.name)))
}

// alpha renders n in base 26 using a-z so the token is purely alphabetic.
func alpha(n int64) string {
	if n == 0 {
		return "a"
	}
	var sb []byte
	for n > 0 {
		sb = append(sb, byte('a'+n%26))
		n /= 26
	}
	return string(sb)
}

// pickOp draws one operation name by weight.
func pickOp(r *rand.Rand) string {
	total := 0
	for _, o := range opWeights {
		total += o.weight
	}
	n := r.Intn(total)
	for _, o := range opWeights {
		if n < o.weight {
			return o.name
		}
		n -= o.weight
	}
	return opList // # pragma: no cover — n < total always lands above
}

// maxLivePerUser caps each account's contact pool: past it, a create becomes a
// delete, so the dataset (and with it the FTS index and DB size) reaches a
// steady state instead of growing for the whole run — otherwise "database
// grew" is indistinguishable from "database leaked".
const maxLivePerUser = 60

// runOp executes one operation for u.
func (w *Workload) runOp(ctx context.Context, u *user, r *rand.Rand, op string) {
	// A login cycle holds the write lock; every other op shares the read lock,
	// so a logout never races another request's cookie.
	if op == opLogin {
		u.mu.Lock()
		defer u.mu.Unlock()
		b, st, err := w.do(ctx, u.client, http.MethodPost, "/api/v1/logout", nil)
		w.record(op, http.MethodPost, "/api/v1/logout", st, b, err)
		if err := w.login(ctx, u); err != nil {
			w.record(op, http.MethodPost, "/api/v1/login", 0, nil, err)
			return
		}
		w.record(op, http.MethodPost, "/api/v1/login", http.StatusOK, nil, nil)
		return
	}
	u.mu.RLock()
	defer u.mu.RUnlock()

	if op == opCreate && u.count() >= maxLivePerUser {
		op = opDelete
	}
	switch op {
	case opList:
		b, st, err := w.do(ctx, u.client, http.MethodGet, "/api/v1/contacts?limit=25", nil)
		w.record(op, http.MethodGet, "/api/v1/contacts", st, b, err)
	case opGet:
		id := u.pick(r)
		if id == 0 {
			return
		}
		p := fmt.Sprintf("/api/v1/contacts/%d", id)
		b, st, err := w.do(ctx, u.client, http.MethodGet, p, nil)
		w.record(op, http.MethodGet, "/api/v1/contacts/:id", st, b, err)
	case opSearch:
		b, st, err := w.do(ctx, u.client, http.MethodGet, "/api/v1/search?q=Soakname*&limit=10", nil)
		w.record(op, http.MethodGet, "/api/v1/search", st, b, err)
	case opCreate:
		w.create(ctx, u, op)
	case opUpdate:
		id := u.pick(r)
		if id == 0 {
			w.create(ctx, u, op)
			return
		}
		p := fmt.Sprintf("/api/v1/contacts/%d", id)
		name := w.newName(u)
		b, st, err := w.do(ctx, u.client, http.MethodPut, p, contactBody(name))
		w.record(op, http.MethodPut, "/api/v1/contacts/:id", st, b, err)
		if err == nil && st == http.StatusOK {
			u.touched(id)
		}
	case opDelete:
		id := u.take(r)
		if id == 0 {
			return
		}
		p := fmt.Sprintf("/api/v1/contacts/%d", id)
		b, st, err := w.do(ctx, u.client, http.MethodDelete, p, nil)
		w.record(op, http.MethodDelete, "/api/v1/contacts/:id", st, b, err)
		if err == nil && st == http.StatusOK {
			u.deleted(id)
		}
	case opUndo:
		w.undo(ctx, u)
	case opNote:
		id := u.pick(r)
		if id == 0 {
			return
		}
		p := fmt.Sprintf("/api/v1/contacts/%d/notes", id)
		b, st, err := w.do(ctx, u.client, http.MethodPost, p, map[string]any{
			"content": "soak note " + alpha(w.nameSeq.Add(1)), "date": time.Now().UTC().Format(time.RFC3339),
		})
		w.record(op, http.MethodPost, "/api/v1/contacts/:id/notes", st, b, err)
	case opActivity:
		id := u.pick(r)
		if id == 0 {
			return
		}
		b, st, err := w.do(ctx, u.client, http.MethodPost, "/api/v1/activities", map[string]any{
			"title": "soak activity " + alpha(w.nameSeq.Add(1)), "date": time.Now().UTC().Format(time.RFC3339),
			"contact_ids": []uint{id},
		})
		w.record(op, http.MethodPost, "/api/v1/activities", st, b, err)
	case opExport:
		b, st, err := w.do(ctx, u.client, http.MethodGet, "/api/v1/export", nil)
		w.record(op, http.MethodGet, "/api/v1/export", st, b, err)
	case opImport:
		w.importVCF(ctx, u)
	case opBadLogin:
		who := fmt.Sprintf("soak-nobody-%d", r.Intn(badLoginPool))
		b, st, err := w.do(ctx, u.client, http.MethodPost, "/api/v1/login", map[string]string{
			"identifier": who, "password": "definitely-wrong-password-1",
		})
		// 401 (and the 429 lockout the limiter applies to a repeatedly failing
		// identifier) are the expected outcomes of a bad login.
		w.record(op, http.MethodPost, "/api/v1/login", st, b, err)
	}
}

func (w *Workload) create(ctx context.Context, u *user, op string) {
	name := w.newName(u)
	b, st, err := w.do(ctx, u.client, http.MethodPost, "/api/v1/contacts", contactBody(name))
	w.record(op, http.MethodPost, "/api/v1/contacts", st, b, err)
	if err != nil || st != http.StatusCreated {
		return
	}
	var created struct {
		Contact struct {
			ID uint `json:"id"`
		} `json:"contact"`
	}
	if json.Unmarshal(b, &created) == nil && created.Contact.ID != 0 {
		u.addID(created.Contact.ID, name)
	}
}

// undo reverts the user's most recent contact update through the audit trail.
func (w *Workload) undo(ctx context.Context, u *user) {
	u.idsMu.Lock()
	id := u.lastUpdated
	u.lastUpdated = 0
	u.idsMu.Unlock()
	if id == 0 {
		return
	}
	lp := fmt.Sprintf("/api/v1/audit?entity_type=contact&entity_id=%d&limit=1", id)
	b, st, err := w.do(ctx, u.client, http.MethodGet, lp, nil)
	w.record(opUndo, http.MethodGet, "/api/v1/audit", st, b, err)
	if err != nil || st != http.StatusOK {
		return
	}
	var list struct {
		Events []struct {
			ID uint `json:"id"`
		} `json:"audit_events"`
	}
	if json.Unmarshal(b, &list) != nil || len(list.Events) == 0 {
		return
	}
	up := fmt.Sprintf("/api/v1/audit/%d/undo", list.Events[0].ID)
	b, st, err = w.do(ctx, u.client, http.MethodPost, up, nil)
	w.record(opUndo, http.MethodPost, "/api/v1/audit/:id/undo", st, b, err)
}

// importVCF uploads a small VCF and confirms it, creating the contacts
// through the real ingestion + FTS path.
func (w *Workload) importVCF(ctx context.Context, u *user) {
	n := w.nameSeq.Add(1)
	var vcf strings.Builder
	for i := 0; i < 3; i++ {
		fmt.Fprintf(&vcf, "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Soakimport%s%d Soak\r\nN:Soak;Soakimport%s%d;;;\r\nEND:VCARD\r\n",
			alpha(n), i, alpha(n), i)
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="file"; filename="soak.vcf"`)
	h.Set("Content-Type", "text/vcard")
	part, err := mw.CreatePart(h)
	if err != nil { // # pragma: no cover — in-memory writer
		return
	}
	_, _ = part.Write([]byte(vcf.String()))
	_ = mw.Close()
	b, st, err := w.send(ctx, u.client, http.MethodPost, "/api/v1/contacts/import/vcf/upload", mw.FormDataContentType(), &buf, true)
	w.record(opImport, http.MethodPost, "/api/v1/contacts/import/vcf/upload", st, b, err)
	if err != nil || st != http.StatusOK {
		return
	}
	var prev struct {
		SessionID string `json:"session_id"`
		TotalRows int    `json:"total_rows"`
	}
	if json.Unmarshal(b, &prev) != nil || prev.SessionID == "" {
		return
	}
	actions := make([]map[string]any, 0, prev.TotalRows)
	for i := 0; i < prev.TotalRows; i++ {
		actions = append(actions, map[string]any{"row_index": i, "action": "add"})
	}
	b, st, err = w.do(ctx, u.client, http.MethodPost, "/api/v1/contacts/import/vcf/confirm", map[string]any{
		"session_id": prev.SessionID, "actions": actions,
	})
	w.record(opImport, http.MethodPost, "/api/v1/contacts/import/vcf/confirm", st, b, err)
}

// Result is the workload's outcome.
type Result struct {
	Total       int64            `json:"total"`
	ServerErrs  int64            `json:"server_errors"`
	RateLimited int64            `json:"rate_limited"`
	Dropped     int64            `json:"dropped"`
	Ops         map[string]int64 `json:"ops"`
	Samples     []string         `json:"failure_samples,omitempty"`
}

// Result snapshots the counters.
func (w *Workload) Result() Result {
	w.counterMu.Lock()
	ops := make(map[string]int64, len(w.opCounts))
	for k, v := range w.opCounts {
		ops[k] = v
	}
	w.counterMu.Unlock()
	w.sampleMu.Lock()
	samples := append([]string(nil), w.samples...)
	w.sampleMu.Unlock()
	return Result{
		Total: w.total.Load(), ServerErrs: w.serverErrs.Load(), RateLimited: w.rateLimited.Load(),
		Dropped: w.dropped.Load(), Ops: ops, Samples: samples,
	}
}

// Run paces operations at cfg.Rate until ctx ends. Each tick hands one
// operation to a bounded pool; if the pool is saturated (the server is
// slower than the offered rate) the tick is counted as dropped rather than
// piling up unbounded goroutines — the harness itself must not be the leak.
func (w *Workload) Run(ctx context.Context) {
	rate := w.cfg.Rate
	interval := time.Duration(float64(time.Second) / rate)
	if interval <= 0 { // # pragma: no cover — only for an absurd Rate
		interval = time.Microsecond
	}
	pool := make(chan struct{}, 4*len(w.users)+8)
	var wg sync.WaitGroup
	tick := time.NewTicker(interval)
	defer tick.Stop()
	r := rand.New(rand.NewSource(w.cfg.Seed)) // #nosec G404 -- workload mix, not security
	var n int
	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case <-tick.C:
		}
		op := pickOp(r)
		u := w.users[n%len(w.users)]
		n++
		select {
		case pool <- struct{}{}:
		default:
			w.dropped.Add(1)
			continue
		}
		wg.Add(1)
		seed := r.Int63()
		go func() {
			defer wg.Done()
			defer func() { <-pool }()
			// Detached from ctx: an operation in flight when the run ends
			// finishes and is counted, rather than failing with "context
			// canceled" and reading as a server error.
			w.runOp(context.WithoutCancel(ctx), u, rand.New(rand.NewSource(seed)), op) // #nosec G404 -- workload mix
		}()
	}
}
