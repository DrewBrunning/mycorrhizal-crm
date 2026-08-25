// Command bolacheck is the cross-account BOLA/IDOR sweep for issue #369's
// Schemathesis work — the deterministic complement to the spec-derived fuzzing
// Schemathesis runs.
//
// Schemathesis derives request cases from openapi.yaml and catches 500s on
// malformed/edge input and auth-protected operations that return data without
// auth. What it cannot do is vary a *resource ID across accounts*: it has no
// second identity to borrow, so a handler that forgets its `user_id` scope
// (the BOLA class, API1 in docs/security/asvs-l2.md) is invisible to it. This
// command fills that gap:
//
//  1. Two throwaway users register and log in (A and B).
//  2. B creates one resource of each tracked entity (contact, note, activity,
//     circle, household, tag, life event, gift, preference, cadence policy,
//     conversation-agenda item, relationship edge, reminder), recording each
//     resource's ID.
//  3. A then attempts GET/PUT/DELETE on every one of B's resources.
//  4. Every cross-account access must be scoped: a 404 (the app's existence
//     masking — see note_controller.go GetNote) or a 403 is the expected
//     answer. A 2xx is an authorization gap, and a 5xx is a crash — both fail
//     the run.
//
// It drives the real HTTP surface (not an in-process harness) via a base URL
// (BOLACHECK_BASE_URL, default http://localhost:7300), so it runs against the
// docker-compose.test.yml artifact — never a real database.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// ts is a fixed, valid RFC3339 timestamp for required time.Time fields.
	ts = "2024-01-01T00:00:00Z"
	// password must satisfy the strong_password validator.
	password = "CorrectHorseBattery9!"
)

type bolaConfig struct {
	baseURL string
}

var defaultBolaConfig = bolaConfig{baseURL: "http://localhost:7300"}

func loadConfig(getenv func(string) string) bolaConfig {
	cfg := defaultBolaConfig
	if v := getenv("BOLACHECK_BASE_URL"); v != "" {
		cfg.baseURL = strings.TrimRight(v, "/")
	}
	return cfg
}

// contactRef is B's contact, the entity other resources reference via their
// `entity_id`/`contact_id` fields.
type contactRef struct {
	id  uint
	uid string
}

// access is one cross-account attempt A makes against one of B's resources.
type access struct {
	method string
	// path is a format template with a single %s for the resource ID (e.g.
	// "/api/v1/notes/%s"), substituted at request time.
	path string
	// body is the JSON body for writes (nil for GET/DELETE). Always a body that
	// would be valid for the owner, so a cross-account write reaches the
	// handler's ownership check rather than failing validation first.
	body any
}

// resourceProbe is one entity type the sweep tracks. create runs as B and
// returns the resource ID (the URL path segment) to probe; accesses are the
// cross-account attempts A makes against that ID.
type resourceProbe struct {
	name     string
	create   func(c *http.Client, base string, ref contactRef) (string, error)
	accesses []access
}

func main() {
	if err := run(os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, "bolacheck:", err)
		os.Exit(1)
	}
}

func run(getenv func(string) string) error {
	cfg := loadConfig(getenv)

	clientA, userA, err := registerAndLogin(cfg.baseURL)
	if err != nil {
		return fmt.Errorf("register/login user A: %w", err)
	}
	clientB, userB, err := registerAndLogin(cfg.baseURL)
	if err != nil {
		return fmt.Errorf("register/login user B: %w", err)
	}
	fmt.Printf("bolacheck: A=%s B=%s against %s\n", userA, userB, cfg.baseURL)

	bContact, err := createContact(clientB, cfg.baseURL)
	if err != nil {
		return fmt.Errorf("B create contact: %w", err)
	}

	probes := buildProbes(bContact)

	var violations []string
	checked := 0
	for _, p := range probes {
		id, err := p.create(clientB, cfg.baseURL, bContact)
		if err != nil {
			return fmt.Errorf("B create %s: %w", p.name, err)
		}
		for _, a := range p.accesses {
			checked++
			path := fmt.Sprintf(a.path, id)
			_, status, err := doJSON(clientA, a.method, cfg.baseURL+path, a.body)
			if err != nil {
				return fmt.Errorf("%s %s: %w", a.method, p.name, err)
			}
			if verdict, ok := classify(status); !ok {
				violations = append(violations,
					fmt.Sprintf("%s: %s %s -> %d (%s)", p.name, a.method, path, status, verdict))
			}
		}
	}

	fmt.Printf("bolacheck: %d cross-account accesses, %d violation(s)\n", checked, len(violations))
	if len(violations) > 0 {
		return fmt.Errorf("%d cross-account authorization gap(s):\n  %s",
			len(violations), strings.Join(violations, "\n  "))
	}
	fmt.Println("OK: bolacheck passed — every cross-account access was scoped (404/403).")
	return nil
}

// classify maps one cross-account response to a pass/fail verdict. 404 (the
// existence mask) and 403 are the two legitimate scoped outcomes; anything
// else is a finding — a 2xx is an IDOR leak, a 5xx is a crash, and any other
// 4xx (401/400/405/422) means the probe itself is malformed.
func classify(status int) (string, bool) {
	switch {
	case status == http.StatusNotFound || status == http.StatusForbidden:
		return "", true
	case status >= 200 && status < 300:
		return "cross-account access leaked data (BOLA/IDOR)", false
	case status >= 500:
		return "cross-account access crashed the handler", false
	default:
		return "cross-account access returned an unexpected status (probe may be malformed)", false
	}
}

// buildProbes returns the tracked entities. B's contact is the anchor for
// every entity that references a contact (`entity_id`/`contact_id`).
func buildProbes(c contactRef) []resourceProbe {
	return []resourceProbe{
		contactProbe(),
		simpleProbe("note", "notes", "note", map[string]any{"content": "bola note", "date": ts}),
		simpleProbe("activity", "activities", "activity", map[string]any{"title": "bola activity", "date": ts}),
		simpleProbe("circle", "circles", "circle", map[string]any{"name": "bola circle"}),
		simpleProbe("household", "households", "household", map[string]any{"name": "bola household", "type": "other"}),
		simpleProbe("tag", "tags", "tag", map[string]any{"name": "bola tag"}),
		simpleProbe("life event", "life-events", "life_event", map[string]any{"entity_id": c.uid}),
		simpleProbe("gift", "gifts", "gift", map[string]any{"entity_id": c.uid, "description": "bola gift"}),
		simpleProbe("preference", "preferences", "preference", map[string]any{"entity_id": c.uid, "category": "bola", "value": "x"}),
		simpleProbe("cadence policy", "cadence-policies", "cadence_policy", map[string]any{"entity_id": c.uid, "target_interval_days": 30}),
		simpleProbe("conversation agenda", "conversation-agenda", "conversation_agenda", map[string]any{"entity_id": c.uid, "content": "bola agenda"}),
		relationshipEdgeProbe(c),
		reminderProbe(c),
	}
}

// simpleProbe builds the standard single-entity probe: POST
// /api/v1/<collection> with body, wrapped under <key>, CRUD via
// /api/v1/<collection>/{id}.
func simpleProbe(name, collection, key string, body any) resourceProbe {
	create := func(c *http.Client, base string, _ contactRef) (string, error) {
		b, status, err := doJSON(c, http.MethodPost, base+"/api/v1/"+collection, body)
		if err != nil {
			return "", err
		}
		if status != http.StatusOK && status != http.StatusCreated {
			return "", fmt.Errorf("status %d: %s", status, truncate(b, 200))
		}
		return extractID(b, key)
	}
	return resourceProbe{
		name:   name,
		create: create,
		accesses: []access{
			{method: http.MethodGet, path: "/api/v1/" + collection + "/%s"},
			{method: http.MethodPut, path: "/api/v1/" + collection + "/%s", body: body},
			{method: http.MethodDelete, path: "/api/v1/" + collection + "/%s"},
		},
	}
}

func contactProbe() resourceProbe {
	body := map[string]any{"card": map[string]any{"name": map[string]any{
		"components": []map[string]string{{"kind": "given", "value": "BolaContact"}},
	}}}
	create := func(cl *http.Client, base string, _ contactRef) (string, error) {
		// Reuse createContact but ignore its parsed contactRef — we already
		// have the anchor contact; this is a second, independent contact.
		ref, err := createContact(cl, base)
		if err != nil {
			return "", err
		}
		return strconv.FormatUint(uint64(ref.id), 10), nil
	}
	return resourceProbe{
		name:   "contact",
		create: create,
		accesses: []access{
			{method: http.MethodGet, path: "/api/v1/contacts/%s"},
			{method: http.MethodPut, path: "/api/v1/contacts/%s", body: body},
			{method: http.MethodDelete, path: "/api/v1/contacts/%s"},
		},
	}
}

func relationshipEdgeProbe(c contactRef) resourceProbe {
	body := map[string]any{
		"source_id": c.uid,
		"target_thin": map[string]string{
			"name": "Bola Target",
		},
		"type": "friend_of",
	}
	create := func(cl *http.Client, base string, _ contactRef) (string, error) {
		b, status, err := doJSON(cl, http.MethodPost, base+"/api/v1/relationship-edges", body)
		if err != nil {
			return "", err
		}
		if status != http.StatusCreated {
			return "", fmt.Errorf("status %d: %s", status, truncate(b, 200))
		}
		return extractID(b, "relationship_edge")
	}
	return resourceProbe{
		name:   "relationship edge",
		create: create,
		accesses: []access{
			{method: http.MethodGet, path: "/api/v1/relationship-edges/%s"},
			{method: http.MethodPut, path: "/api/v1/relationship-edges/%s", body: body},
			{method: http.MethodDelete, path: "/api/v1/relationship-edges/%s"},
		},
	}
}

func reminderProbe(c contactRef) resourceProbe {
	body := map[string]any{
		"message":                 "bola reminder",
		"remind_at":               ts,
		"recurrence":              "once",
		"contact_id":              c.id,
		"by_mail":                 false,
		"reoccur_from_completion": true,
	}
	create := func(cl *http.Client, base string, ref contactRef) (string, error) {
		path := fmt.Sprintf("%s/api/v1/contacts/%d/reminders", base, ref.id)
		b, status, err := doJSON(cl, http.MethodPost, path, body)
		if err != nil {
			return "", err
		}
		if status != http.StatusOK {
			return "", fmt.Errorf("status %d: %s", status, truncate(b, 200))
		}
		return extractID(b, "reminder")
	}
	return resourceProbe{
		name:   "reminder",
		create: create,
		accesses: []access{
			{method: http.MethodGet, path: "/api/v1/reminders/%s"},
			{method: http.MethodPut, path: "/api/v1/reminders/%s", body: body},
			{method: http.MethodDelete, path: "/api/v1/reminders/%s"},
		},
	}
}

// createContact creates a contact and returns its numeric id + vCard uid.
func createContact(c *http.Client, base string) (contactRef, error) {
	body := map[string]any{"card": map[string]any{"name": map[string]any{
		"components": []map[string]string{{"kind": "given", "value": "BolaAnchor"}},
	}}}
	b, status, err := doJSON(c, http.MethodPost, base+"/api/v1/contacts", body)
	if err != nil {
		return contactRef{}, err
	}
	if status != http.StatusCreated {
		return contactRef{}, fmt.Errorf("status %d: %s", status, truncate(b, 200))
	}
	var env struct {
		Contact struct {
			ID  uint   `json:"id"`
			UID string `json:"uid"`
		} `json:"contact"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		return contactRef{}, fmt.Errorf("parse create response: %w", err)
	}
	if env.Contact.ID == 0 || env.Contact.UID == "" {
		return contactRef{}, fmt.Errorf("create response missing id/uid: %s", truncate(b, 200))
	}
	return contactRef{id: env.Contact.ID, uid: env.Contact.UID}, nil
}

// extractID pulls the resource's `id` field out of a wrapped create response
// (e.g. `{"note":{"id":123}}` or `{"circle":{"id":"<uuid>"}}`), formatting it
// as the URL path segment regardless of whether it is a JSON number or string.
func extractID(body []byte, key string) (string, error) {
	var env map[string]any
	if err := json.Unmarshal(body, &env); err != nil {
		return "", fmt.Errorf("parse create response: %w", err)
	}
	obj, ok := env[key].(map[string]any)
	if !ok {
		return "", fmt.Errorf("create response has no %q object: %s", key, truncate(body, 200))
	}
	raw, ok := obj["id"]
	if !ok {
		// gorm.Model-backed entities (Note, Activity, Reminder) serialize the
		// identity as PascalCase "ID" (no json tag on gorm.Model), while the
		// UUID-PK entities use a lowercase "id" tag.
		raw, ok = obj["ID"]
	}
	if !ok {
		return "", fmt.Errorf("create response %q object has no id: %s", key, truncate(body, 200))
	}
	switch v := raw.(type) {
	case string:
		if v == "" {
			return "", fmt.Errorf("create response %q id is empty", key)
		}
		return v, nil
	case float64:
		return strconv.FormatInt(int64(v), 10), nil
	default:
		return "", fmt.Errorf("create response %q id has unexpected type %T", key, raw)
	}
}

// registerAndLogin creates a throwaway user and returns an authenticated
// client (cookie jar carries auth_token) plus the username.
func registerAndLogin(base string) (*http.Client, string, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, "", err
	}
	client := &http.Client{Jar: jar, Timeout: 15 * time.Second}

	username := fmt.Sprintf("bola-%d", time.Now().UnixNano())
	regBody := map[string]string{
		"username": username,
		"email":    username + "@example.com",
		"password": password,
	}
	if body, status, err := doJSON(client, http.MethodPost, base+"/api/v1/register", regBody); err != nil {
		return nil, "", err
	} else if status != http.StatusCreated {
		return nil, "", fmt.Errorf("register: status %d: %s", status, truncate(body, 200))
	}

	loginBody := map[string]string{"identifier": username, "password": password}
	if body, status, err := doJSON(client, http.MethodPost, base+"/api/v1/login", loginBody); err != nil {
		return nil, "", err
	} else if status != http.StatusOK {
		return nil, "", fmt.Errorf("login: status %d: %s", status, truncate(body, 200))
	}
	return client, username, nil
}

func truncate(b []byte, n int) string {
	s := string(b)
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

// doJSON marshals body (nil for no body) as JSON, issues the request, and
// returns the drained response body + status. The *http.Response is
// deliberately not returned (no caller uses it, and returning it made
// bodyclose flag every call site for a body that is in fact drained here).
func doJSON(client *http.Client, method, url string, body any) ([]byte, int, error) {
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
