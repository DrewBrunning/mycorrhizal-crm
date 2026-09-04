package perfbench

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	"mycorrhizal/models"
	"mycorrhizal/services"
)

// GrowthClass ranks how an operation's cost scales with the dataset. Ordered:
// constant < linear < superlinear.
type GrowthClass string

const (
	GrowthConstant    GrowthClass = "constant"
	GrowthLinear      GrowthClass = "linear"
	GrowthSuperlinear GrowthClass = "superlinear"
)

func (g GrowthClass) rank() int {
	switch g {
	case GrowthConstant:
		return 0
	case GrowthLinear:
		return 1
	case GrowthSuperlinear:
		return 2
	default:
		return -1
	}
}

// Operation is one measured core operation.
type Operation struct {
	// Name is the stable key used by the baseline and the report.
	Name string
	// Category is "read" or "write".
	Category string
	// Destructive operations mutate the fixture irreversibly (merge, delete
	// cascade). They get their own fresh Env and are measured exactly once.
	Destructive bool
	// SlowRead marks a read whose wall-clock is large enough that N iterations
	// would dominate CI time (the deep recursive-CTE traversals). Measured
	// once — the query count from that run is still the deterministic gate;
	// only the median loses its averaging.
	SlowRead bool
	// SkipAtScale omits this operation from the `large` profile only (it still
	// runs at smoke + typical). It is for operations that are super-linear by
	// construction — a walk-enumeration recursive CTE over a synthetically
	// dense graph — whose one deterministic signal (a constant query count) is
	// already pinned at smoke + typical, and which nothing asserts at `large`:
	// growth.go classifies them on query count (see ClassifyResultGrowth's
	// "a graph traversal returns more rows on a denser dataset by design"
	// note), CommittedBaselineIsCurrent rebuilds from smoke + typical, and
	// QueryCountsStayBoundedAtLarge only checks whatever ran. At `large` the
	// PERF-01 graph shape (25 hubs x 150 fan-out) makes a depth-3/4 traversal
	// enumerate millions of partial walks, each triggering a full scan of the
	// edge table because the CTE's `(source_id = ? OR target_id = ?)` join
	// cannot use an index — tens of minutes to hours for zero extra gated
	// signal. Mirrors datamovement.go's bounded `large` import batch
	// (maxImportContacts), same rationale, different lever.
	SkipAtScale bool
	// ExpectedGrowth is the worst growth class this operation is allowed to
	// exhibit across scales. Every core operation is "constant" in query count
	// — a bounded number of statements regardless of data volume — except the
	// one known pairwise finding (duplicate detection, "superlinear"). A
	// measured class worse than this fails TestCoreOperationBenchmarks.
	ExpectedGrowth GrowthClass
	// ClassifyResultGrowth folds the result-set-size growth into the class,
	// for operations where the OUTPUT CARDINALITY is the algorithmic cost
	// (duplicate detection is O(group^2) pairs while issuing O(1) queries). It
	// is off by default: a page-limited list or a graph traversal returns more
	// rows on a denser dataset by design, not by inefficiency.
	ClassifyResultGrowth bool
	// Prepare, when set, runs before the warm-up and before every measured
	// iteration. Any DB writes it makes MUST go through e.SeedDB() so they are
	// never counted. It resets state that would otherwise make a repeated call
	// unrepresentative — a daily job's lock and cursor, a merge's conflict
	// resolutions.
	Prepare func(e *Env) error
	// Run executes the operation against e.DB and returns the size of its
	// result set (0 when the operation has no natural cardinality — a
	// single-record fetch, a write). Setup must not happen inside Run: the
	// query counter is reset immediately before each call.
	Run func(e *Env) (resultSize int, err error)
}

// Registry returns every core operation issue #469 lists, in a stable order.
func Registry() []Operation {
	return []Operation{
		// --- contact list: pagination, filters, sorting -------------------
		{
			Name: "contact_list.plain", Category: "read", ExpectedGrowth: GrowthConstant,
			Run: func(e *Env) (int, error) {
				return e.listLen(e.get("/contacts?limit=50"))
			},
		},
		{
			// sort=name (the denormalized sort_name cursor) + a circle-
			// membership EXISTS filter together. An empty circle name is
			// simply ignored by the handler.
			Name: "contact_list.filtered_sorted", Category: "read", ExpectedGrowth: GrowthConstant,
			Run: func(e *Env) (int, error) {
				return e.listLen(e.get("/contacts?limit=50&sort=name&circle=" + url.QueryEscape(e.CircleName)))
			},
		},

		// --- contact detail: nested Card + every projection ---------------
		{
			Name: "contact_detail", Category: "read", ExpectedGrowth: GrowthConstant,
			Run: func(e *Env) (int, error) {
				_, err := e.expect(e.get(fmt.Sprintf("/contacts/%d/detail", e.HubContact.ID)), http.StatusOK)
				return 0, err
			},
		},
		{
			Name: "contact_detail.pathological", Category: "read", ExpectedGrowth: GrowthConstant,
			Run: func(e *Env) (int, error) {
				_, err := e.expect(e.get(fmt.Sprintf("/contacts/%d/detail", e.PathologicalContact.ID)), http.StatusOK)
				return 0, err
			},
		},

		// --- full-text search across contacts_fts/notes_fts/activities_fts
		{
			Name: "fts.search_all", Category: "read", ExpectedGrowth: GrowthConstant,
			Run: func(e *Env) (int, error) {
				body, err := e.expect(e.get("/search?q=Lovelace"), http.StatusOK)
				if err != nil { // # pragma: no cover — a valid q always returns 200
					return 0, err
				}
				var r struct {
					Contacts   []json.RawMessage `json:"contacts"`
					Notes      []json.RawMessage `json:"notes"`
					Activities []json.RawMessage `json:"activities"`
				}
				if err := json.Unmarshal(body, &r); err != nil { // # pragma: no cover — SearchAll always returns a JSON object
					return 0, err
				}
				return len(r.Contacts) + len(r.Notes) + len(r.Activities), nil
			},
		},
		{
			Name: "fts.contact_list_search", Category: "read", ExpectedGrowth: GrowthConstant,
			Run: func(e *Env) (int, error) {
				return e.listLen(e.get("/contacts?limit=50&search=Lovelace"))
			},
		},

		// --- relationship traversal: multi-hop, dense hubs ----------------
		{
			Name: "graph.traverse_shallow", Category: "read", ExpectedGrowth: GrowthConstant,
			Run: func(e *Env) (int, error) {
				chains, err := services.TraverseGraph(e.DB, e.UserID, e.NormalContact.VCardUID, 2, "")
				return len(chains), err
			},
		},
		{
			// SkipAtScale: from the block-0 lead — which the PERF-01 shape makes
			// both the chain head and hub #0 — a depth-4 traversal of the
			// `large` graph enumerates millions of partial walks against a
			// non-indexable CTE join. Runs at smoke + typical (where the
			// constant query count is pinned); see SkipAtScale's doc.
			Name: "graph.traverse_deep", Category: "read", ExpectedGrowth: GrowthConstant, SlowRead: true, SkipAtScale: true,
			Run: func(e *Env) (int, error) {
				chains, err := services.TraverseGraph(e.DB, e.UserID, e.HubContact.VCardUID, 4, "")
				return len(chains), err
			},
		},
		{
			// SkipAtScale: depth-3 from a second dense hub — same combinatorial
			// blow-up on the `large` graph as graph.traverse_deep.
			Name: "graph.traverse_hub", Category: "read", ExpectedGrowth: GrowthConstant, SlowRead: true, SkipAtScale: true,
			Run: func(e *Env) (int, error) {
				chains, err := services.TraverseGraph(e.DB, e.UserID, e.SecondHubContact.VCardUID, 3, "")
				return len(chains), err
			},
		},

		// --- dashboard + aggregates -------------------------------------
		{
			Name: "dashboard", Category: "read", ExpectedGrowth: GrowthConstant,
			Run: func(e *Env) (int, error) {
				_, err := e.expect(e.get("/dashboard"), http.StatusOK)
				return 0, err
			},
		},

		// --- reach-out / cadence over the whole contact set --------------
		{
			Name: "cadence.list_overdue", Category: "read", ExpectedGrowth: GrowthConstant,
			Run: func(e *Env) (int, error) {
				overdue, err := services.ListOverdueCadences(e.DB, e.UserID, time.Now())
				return len(overdue), err
			},
		},
		{
			Name: "reachout.detect", Category: "read", ExpectedGrowth: GrowthConstant,
			// The daily detection job is lock- and cursor-guarded: without a
			// reset the second call is a no-op. Clear both on the seed
			// connection so every measured run is a full audit-batch scan.
			Prepare: func(e *Env) error {
				seed := e.SeedDB()
				if err := seed.Exec("DELETE FROM job_executions WHERE job_name = ?", models.JobNameReachOutDetection).Error; err != nil { // # pragma: no cover — a plain DELETE on a migrated schema
					return err
				}
				return seed.Exec("UPDATE reach_out_cursors SET last_audit_event_id = 0").Error
			},
			Run: func(e *Env) (int, error) {
				n, err := services.DetectReachOutSuggestions(e.DB, e.Cfg)
				return n, err
			},
		},

		// --- duplicate detection: inherently pairwise -------------------
		{
			// KNOWN super-linear (issue #469 calls it "the most likely
			// quadratic surprise"): the query count is bounded (one window
			// function per tier) but the in-memory pair expansion is
			// O(group^2), and the block-replicated fixture puts an identical
			// card name in every block. Recorded as a finding here; PERF-04
			// (#471) waives its wall-clock budget (background/review surface)
			// in budgets.json — the deterministic query-count budget still holds.
			Name: "duplicates.find_pairs", Category: "read", ExpectedGrowth: GrowthSuperlinear, ClassifyResultGrowth: true,
			Run: func(e *Env) (int, error) {
				pairs, err := services.FindDuplicatePairs(e.DB, e.UserID)
				return len(pairs), err
			},
		},

		// --- write path ------------------------------------------------
		{
			Name: "contact_create", Category: "write", ExpectedGrowth: GrowthConstant,
			Run: func(e *Env) (int, error) {
				body := contactRecordInputJSON(fmt.Sprintf("Perf%d", time.Now().UnixNano()), "Bench")
				_, err := e.expect(e.post("/contacts", body), http.StatusCreated)
				return 0, err
			},
		},
		{
			Name: "contact_update", Category: "write", ExpectedGrowth: GrowthConstant,
			Run: func(e *Env) (int, error) {
				body := contactRecordInputJSON(e.NormalContact.Firstname, e.NormalContact.Lastname)
				_, err := e.expect(e.put(fmt.Sprintf("/contacts/%d", e.NormalContact.ID), body), http.StatusOK)
				return 0, err
			},
		},

		// --- contact merge: repoints every association ------------------
		{
			Name: "contact_merge", Category: "write", Destructive: true, ExpectedGrowth: GrowthConstant,
			// Pre-compute the scalar-conflict resolutions the API requires
			// (keeper wins each) so the measured request is the merge itself,
			// not a 400.
			Prepare: func(e *Env) error {
				var keep, loser models.Contact
				if err := e.SeedDB().First(&keep, e.NormalContact.ID).Error; err != nil { // # pragma: no cover — the handles were just resolved from this DB
					return err
				}
				if err := e.SeedDB().First(&loser, e.HubContact.ID).Error; err != nil { // # pragma: no cover — see above
					return err
				}
				res := services.ComputeContactMergeResolution(&keep, &loser)
				m := make(map[string]string, len(res.Conflicts))
				for _, c := range res.Conflicts {
					m[c.Field] = c.KeeperValue
				}
				e.mergeResolutions = m
				return nil
			},
			Run: func(e *Env) (int, error) {
				// Merge the dense hub INTO a normal contact: RepointContactAssociations
				// walks every dependent table for the loser.
				body := mustJSON(models.ContactMergeRequest{
					KeepID:      e.NormalContact.ID,
					MergeID:     e.HubContact.ID,
					Resolutions: e.mergeResolutions,
				})
				_, err := e.expect(e.post("/contacts/merge", body), http.StatusOK)
				return 0, err
			},
		},

		// --- delete cascade: enumerates every dependent table ----------
		{
			Name: "delete_cascade", Category: "write", Destructive: true, ExpectedGrowth: GrowthConstant,
			Run: func(e *Env) (int, error) {
				_, err := e.expect(e.del(fmt.Sprintf("/contacts/%d", e.HubContact.ID)), http.StatusOK)
				return 0, err
			},
		},
	}
}

// --- request helpers --------------------------------------------------------

type httpResult struct {
	status int
	body   []byte
}

func (e *Env) do(method, path string, body []byte) httpResult {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	e.Router().ServeHTTP(w, req)
	return httpResult{status: w.Code, body: w.Body.Bytes()}
}

func (e *Env) get(path string) httpResult            { return e.do(http.MethodGet, path, nil) }
func (e *Env) del(path string) httpResult            { return e.do(http.MethodDelete, path, nil) }
func (e *Env) post(path string, b []byte) httpResult { return e.do(http.MethodPost, path, b) }
func (e *Env) put(path string, b []byte) httpResult  { return e.do(http.MethodPut, path, b) }

// expect asserts the response status and returns the body.
func (e *Env) expect(r httpResult, want int) ([]byte, error) {
	if r.status != want {
		return nil, fmt.Errorf("status %d (want %d): %s", r.status, want, truncate(r.body, 300))
	}
	return r.body, nil
}

// listLen decodes a {"contacts":[...]} list response and returns the page size.
func (e *Env) listLen(r httpResult) (int, error) {
	body, err := e.expect(r, http.StatusOK)
	if err != nil { // # pragma: no cover — the list endpoint always returns 200 for these fixtures
		return 0, err
	}
	var parsed struct {
		Contacts []json.RawMessage `json:"contacts"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil { // # pragma: no cover — GetContacts always returns a JSON object with a contacts array
		return 0, err
	}
	return len(parsed.Contacts), nil
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err) // # pragma: no cover — the values marshalled here are plain structs
	}
	return b
}

// contactRecordInputJSON builds the minimal models.ContactRecordInput body the
// create/update routes accept: one given-name component (CreateContact
// requires a derivable Firstname) and one surname.
func contactRecordInputJSON(given, surname string) []byte {
	if given == "" {
		given = "Perf"
	}
	return mustJSON(map[string]any{
		"card": map[string]any{
			"name": map[string]any{
				"components": []map[string]string{
					{"kind": "given", "value": given},
					{"kind": "surname", "value": surname},
				},
			},
		},
		"crm": map[string]any{},
	})
}
