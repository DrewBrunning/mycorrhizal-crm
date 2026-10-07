package soak

import (
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-co-op/gocron"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPickOp_FollowsWeightsAndCoversEveryOp(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	seen := map[string]int{}
	for i := 0; i < 20000; i++ {
		seen[pickOp(r)]++
	}
	for _, o := range opWeights {
		assert.Greater(t, seen[o.name], 0, "%s was never drawn", o.name)
	}
	assert.Greater(t, seen[opList], seen[opExport]*5, "reads dominate rare heavy ops")
}

func TestAlpha(t *testing.T) {
	assert.Equal(t, "a", alpha(0))
	assert.Equal(t, "b", alpha(1))
	assert.Equal(t, "ab", alpha(26))
	for _, c := range alpha(123456789) {
		assert.True(t, c >= 'a' && c <= 'z')
	}
}

func TestUserPool(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	u := &user{}
	assert.Zero(t, u.pick(r))
	assert.Zero(t, u.take(r))
	u.addID(5, "n5")
	u.addID(6, "n6")
	assert.Equal(t, 2, u.count())
	assert.NotZero(t, u.pick(r))

	u.touched(5)
	assert.NotContains(t, u.pristine, uint(5), "an updated contact leaves the probe set")
	assert.Equal(t, uint(5), u.lastUpdated)

	u.deleted(6)
	assert.Equal(t, []string{"n6"}, u.gone)
	u.deleted(5) // not pristine: no gone entry
	assert.Len(t, u.gone, 1)

	a, b := u.take(r), u.take(r)
	assert.ElementsMatch(t, []uint{5, 6}, []uint{a, b})
	assert.Zero(t, u.count())
}

func TestRecord_ClassifiesOutcomes(t *testing.T) {
	w := &Workload{opCounts: map[string]int64{}}
	w.record("a", "GET", "/x", 200, nil, nil)
	w.record("a", "GET", "/x", 404, nil, nil)
	w.record("a", "GET", "/x", 429, nil, nil)
	w.record("a", "GET", "/x", 500, []byte("boom"), nil)
	w.record("a", "GET", "/x", 0, nil, assert.AnError)
	res := w.Result()
	assert.EqualValues(t, 5, res.Total)
	assert.EqualValues(t, 1, res.RateLimited)
	assert.EqualValues(t, 2, res.ServerErrs, "5xx and transport errors fail; 4xx churn does not")
	assert.Len(t, res.Samples, 2)
	assert.EqualValues(t, 5, res.Ops["a"])

	for i := 0; i < 20; i++ {
		w.addSample("more")
	}
	assert.LessOrEqual(t, len(w.Result().Samples), maxSamples)
}

func TestSearchResultHasFirstname(t *testing.T) {
	body := []byte(`{"query":"Zed","contacts":[{"firstname":"Alpha"}]}`)
	assert.True(t, searchResultHasFirstname(body, "alpha"))
	assert.False(t, searchResultHasFirstname(body, "Zed"), "the echoed query is not a hit")
	assert.False(t, searchResultHasFirstname([]byte("not json"), "x"))
}

// fakeAPI is a minimal stand-in for the endpoints the workload hits, with
// switches to make individual calls fail so every error branch is exercised
// without a real server.
type fakeAPI struct {
	mu        sync.Mutex
	nextID    int
	fail      map[string]int // path prefix -> status to return
	badJSON   bool
	noEvents  bool
	searchHit string
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for prefix, st := range f.fail {
		if strings.HasPrefix(r.URL.Path, prefix) {
			w.WriteHeader(st)
			_, _ = w.Write([]byte("forced"))
			return
		}
	}
	write := func(st int, v any) {
		w.WriteHeader(st)
		if f.badJSON {
			_, _ = w.Write([]byte("{"))
			return
		}
		_ = json.NewEncoder(w).Encode(v)
	}
	switch {
	case r.URL.Path == "/api/v1/register":
		write(http.StatusCreated, map[string]any{})
	case r.URL.Path == "/api/v1/login", r.URL.Path == "/api/v1/logout":
		write(http.StatusOK, map[string]any{})
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/contacts":
		f.nextID++
		write(http.StatusCreated, map[string]any{"contact": map[string]any{"id": f.nextID}})
	case strings.HasPrefix(r.URL.Path, "/api/v1/search"):
		write(http.StatusOK, map[string]any{"contacts": []map[string]any{{"firstname": f.searchHit}}})
	case r.URL.Path == "/api/v1/audit":
		if f.noEvents {
			write(http.StatusOK, map[string]any{"audit_events": []any{}})
		} else {
			write(http.StatusOK, map[string]any{"audit_events": []map[string]any{{"id": 9}}})
		}
	case r.URL.Path == "/api/v1/contacts/import/vcf/upload":
		write(http.StatusOK, map[string]any{"session_id": "s1", "total_rows": 3})
	default:
		write(http.StatusOK, map[string]any{})
	}
}

func newFakeWorkload(t *testing.T, f *fakeAPI) (*Workload, func()) {
	t.Helper()
	srv := httptest.NewServer(f)
	w, err := NewWorkload(context.Background(), WorkloadConfig{BaseURL: srv.URL, Users: 2, Rate: 200, Seed: 3})
	require.NoError(t, err)
	return w, srv.Close
}

func TestWorkload_EveryOpAgainstAFakeServer(t *testing.T) {
	f := &fakeAPI{fail: map[string]int{}, searchHit: "Soakx"}
	w, closeFn := newFakeWorkload(t, f)
	defer closeFn()
	ctx := context.Background()
	r := rand.New(rand.NewSource(1))
	u := w.users[0]

	// Seed a couple of contacts, then run every op (some twice so the
	// pool-empty and pool-populated branches both run).
	w.create(ctx, u, opCreate)
	w.create(ctx, u, opCreate)
	for _, o := range opWeights {
		w.runOp(ctx, u, r, o.name)
		w.runOp(ctx, u, r, o.name)
	}
	// Undo with a recorded update and no audit events.
	u.touched(1)
	f.noEvents = true
	w.undo(ctx, u)
	res := w.Result()
	assert.Zero(t, res.ServerErrs)
	for _, o := range opWeights {
		assert.Greater(t, res.Ops[o.name], int64(0), "%s never recorded", o.name)
	}

	// A full pool turns a create into a delete (steady-state dataset).
	for i := 0; i < maxLivePerUser; i++ {
		u.addID(uint(1000+i), "x")
	}
	before := u.count()
	w.runOp(ctx, u, r, opCreate)
	assert.Less(t, u.count(), before)

	// The search probe finds names the fake lists and flags ones it does not.
	for _, x := range w.users {
		x.pristine, x.gone = nil, nil
	}
	u.addID(4242, "Soakx")
	assert.True(t, w.searchProbe(ctx).OK)
	u.deleted(4242)
	assert.False(t, w.searchProbe(ctx).OK, "a deleted name the index still returns is drift")
	f.searchHit = "other"
	u.addID(77, "Soakx")
	c := w.searchProbe(ctx)
	assert.False(t, c.OK)
	assert.Contains(t, c.Detail, "not found")
}

func TestWorkload_ServerFaultsAreCounted(t *testing.T) {
	f := &fakeAPI{fail: map[string]int{}}
	w, closeFn := newFakeWorkload(t, f)
	defer closeFn()
	ctx := context.Background()
	r := rand.New(rand.NewSource(1))
	u := w.users[0]

	f.fail["/api/v1/contacts"] = 500
	w.create(ctx, u, opCreate)
	w.runOp(ctx, u, r, opList)
	w.runOp(ctx, u, r, opExport)
	assert.Greater(t, w.Result().ServerErrs, int64(0))
	delete(f.fail, "/api/v1/contacts")

	// Malformed JSON bodies on the parse-dependent follow-ups must not panic.
	f.badJSON = true
	w.create(ctx, u, opCreate)
	w.importVCF(ctx, u)
	u.touched(3)
	w.undo(ctx, u)
	f.badJSON = false

	f.fail["/api/v1/contacts/import/vcf/upload"] = 400
	w.importVCF(ctx, u)
	delete(f.fail, "/api/v1/contacts/import/vcf/upload")

	f.fail["/api/v1/audit"] = 500
	u.touched(3)
	w.undo(ctx, u)
	delete(f.fail, "/api/v1/audit")

	u.touched(3)
	w.undo(ctx, u) // lastUpdated consumed above; second call is a no-op path
	w.undo(ctx, u)

	f.fail["/api/v1/login"] = 500
	w.runOp(ctx, u, r, opLogin)
}

func TestNewWorkload_RegistrationFailures(t *testing.T) {
	f := &fakeAPI{fail: map[string]int{"/api/v1/register": 500}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	_, err := NewWorkload(context.Background(), WorkloadConfig{BaseURL: srv.URL})
	assert.Error(t, err)

	f.fail = map[string]int{"/api/v1/login": 401}
	_, err = NewWorkload(context.Background(), WorkloadConfig{BaseURL: srv.URL, Users: 0, Rate: -1})
	assert.Error(t, err)

	_, err = NewWorkload(context.Background(), WorkloadConfig{BaseURL: "http://127.0.0.1:1"})
	assert.Error(t, err, "an unreachable server is a registration error")
}

func TestWorkload_RunPacesAndDropsUnderBackpressure(t *testing.T) {
	f := &fakeAPI{fail: map[string]int{}}
	w, closeFn := newFakeWorkload(t, f)
	defer closeFn()
	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()
	w.Run(ctx)
	res := w.Result()
	assert.Greater(t, res.Total, int64(20), "a 200 ops/s offered rate must complete many ops in 0.7s")
	assert.Zero(t, res.ServerErrs, "ops in flight at shutdown finish rather than failing as 'context canceled'")
}

func TestWorkload_RunDropsWhenTheServerIsSlow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "register") {
			w.WriteHeader(http.StatusCreated)
			return
		}
		if r.URL.Path != "/api/v1/login" {
			time.Sleep(150 * time.Millisecond)
		}
	}))
	defer srv.Close()
	w, err := NewWorkload(context.Background(), WorkloadConfig{BaseURL: srv.URL, Users: 1, Rate: 500, Seed: 1})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	w.Run(ctx)
	assert.Greater(t, w.Result().Dropped, int64(0), "a saturated pool drops ticks instead of spawning unbounded goroutines")
}

func TestSchedulerCheck(t *testing.T) {
	now := time.Now()
	assert.False(t, schedulerCheck(nil, now).OK, "no jobs registered is a failure")

	s := gocron.NewScheduler(time.UTC)
	j, err := s.Every(1).Hour().Tag("hourly").Do(func() {})
	require.NoError(t, err)
	s.StartAsync()
	defer s.Stop()
	require.Eventually(t, func() bool { return !j.NextRun().IsZero() }, 2*time.Second, 10*time.Millisecond)
	c := schedulerCheck(s.Jobs(), now)
	assert.True(t, c.OK, c.Detail)

	// A job whose next run is in the past is wedged.
	c = schedulerCheck(s.Jobs(), now.Add(48*time.Hour))
	assert.False(t, c.OK)
	assert.Contains(t, c.Detail, "hourly")

	// Never-started scheduler: no next run, and an untagged job is named.
	s2 := gocron.NewScheduler(time.UTC)
	_, err = s2.Every(1).Hour().Do(func() {})
	require.NoError(t, err)
	c = schedulerCheck(s2.Jobs(), now)
	assert.False(t, c.OK)
	assert.Contains(t, c.Detail, "(untagged)")
}
