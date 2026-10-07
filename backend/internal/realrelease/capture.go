package realrelease

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// Snapshot is the API read-back of one instance: a JSON document keyed by
// resource. It is captured once through the OLD release's API before the
// upgrade and once through the CURRENT API after it; Compare asserts the
// second still says everything the first did.
type Snapshot map[string]any

func readAll(resp *http.Response) string {
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

// listAll follows next_cursor over a list endpoint and returns the items under
// key. extra is an already-encoded query string ("a=b&c=d") or "".
func (c *Client) listAll(ctx context.Context, path, key, extra string) ([]any, error) {
	var items []any
	cursor := ""
	for page := 0; page < 1000; page++ {
		q := url.Values{"limit": {"100"}}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		full := path + "?" + q.Encode()
		if extra != "" {
			full += "&" + extra
		}
		var resp map[string]json.RawMessage
		if err := c.call(ctx, http.MethodGet, full, nil, &resp); err != nil {
			return nil, err
		}
		raw, ok := resp[key]
		if !ok {
			return nil, fmt.Errorf("GET %s: response has no %q key", path, key)
		}
		var batch []any
		if err := json.Unmarshal(raw, &batch); err != nil {
			return nil, fmt.Errorf("GET %s: %q is not a list: %w", path, key, err)
		}
		items = append(items, batch...)
		cursor = ""
		if nc, ok := resp["next_cursor"]; ok {
			_ = json.Unmarshal(nc, &cursor)
		}
		if cursor == "" {
			return items, nil
		}
	}
	return nil, fmt.Errorf("GET %s: pagination did not terminate", path)
}

// Capture reads the seeded account's data back through the API.
func Capture(ctx context.Context, c *Client) (Snapshot, error) {
	snap := Snapshot{}

	contacts, err := c.listAll(ctx, "/contacts", "contacts", "include_archived=true")
	if err != nil {
		return nil, fmt.Errorf("list contacts: %w", err)
	}
	snap["contacts"] = contacts

	maxID := 0
	records := map[string]any{}
	notes := map[string]any{}
	attachments := map[string]any{}
	for _, it := range contacts {
		m, _ := it.(map[string]any)
		idf, _ := m["id"].(float64)
		id := int(idf)
		if id > maxID {
			maxID = id
		}
		key := fmt.Sprintf("%d", id)

		var rec any
		if err := c.call(ctx, http.MethodGet, "/contacts/"+key, nil, &rec); err != nil {
			return nil, fmt.Errorf("read contact %s: %w", key, err)
		}
		records[key] = rec

		ns, err := c.listAll(ctx, "/contacts/"+key+"/notes", "notes", "")
		if err != nil {
			return nil, fmt.Errorf("list notes of %s: %w", key, err)
		}
		notes[key] = ns

		var atts struct {
			Attachments []struct {
				ID uint `json:"id"`
			} `json:"attachments"`
		}
		if err := c.call(ctx, http.MethodGet, "/contacts/"+key+"/attachments", nil, &atts); err != nil {
			return nil, fmt.Errorf("list attachments of %s: %w", key, err)
		}
		for _, a := range atts.Attachments {
			status, body, err := c.raw(ctx, http.MethodGet, fmt.Sprintf("/attachments/%d/download", a.ID), nil, nil)
			if err != nil {
				return nil, fmt.Errorf("download attachment %d: %w", a.ID, err)
			}
			if status != http.StatusOK {
				return nil, fmt.Errorf("download attachment %d: HTTP %d", a.ID, status)
			}
			sum := sha256.Sum256(body)
			attachments[fmt.Sprintf("%d", a.ID)] = map[string]any{
				"contact_id": id, "sha256": hex.EncodeToString(sum[:]), "bytes": len(body),
			}
		}
	}
	snap["contact_records"] = records
	snap["contact_notes"] = notes
	snap["attachment_files"] = attachments

	// A soft-deleted contact must stay invisible: probe every id up to one
	// past the highest live id and record the status code per id.
	probe := map[string]any{}
	for id := 1; id <= maxID+3; id++ {
		status, _, err := c.raw(ctx, http.MethodGet, fmt.Sprintf("/contacts/%d", id), nil, nil)
		if err != nil {
			return nil, fmt.Errorf("probe contact %d: %w", id, err)
		}
		probe[fmt.Sprintf("%d", id)] = status
	}
	snap["contact_id_status"] = probe

	for _, l := range []struct{ name, path, key, extra string }{
		{"activities", "/activities", "activities", ""},
		{"circles", "/circles", "circles", "include_members=true"},
		{"tags", "/tags", "tags", "include_contacts=true"},
		{"relationship_edges", "/relationship-edges", "relationship_edges", ""},
		{"life_events", "/life-events", "life_events", ""},
	} {
		items, err := c.listAll(ctx, l.path, l.key, l.extra)
		if err != nil {
			return nil, fmt.Errorf("list %s: %w", l.name, err)
		}
		snap[l.name] = items
	}

	for _, s := range []struct{ name, path string }{
		{"user", "/users/me"},
		{"api_tokens", "/api-tokens"},
		{"audit", "/audit?limit=500"},
	} {
		var v any
		if err := c.call(ctx, http.MethodGet, s.path, nil, &v); err != nil {
			return nil, fmt.Errorf("read %s: %w", s.name, err)
		}
		if um, ok := v.(map[string]any); ok && s.name == "user" {
			// Every login writes the user row (last-login bookkeeping), so
			// its updated_at differs between any two captures.
			delete(um, "updated_at")
		}
		if s.name == "audit" {
			v = withoutAuthEvents(v)
		}
		snap[s.name] = v
	}
	return normalize(snap)
}

// withoutAuthEvents drops login/logout audit rows: the harness's own logins
// between the two captures legitimately add them, and they carry no user data.
func withoutAuthEvents(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}
	events, _ := m["audit_events"].([]any)
	kept := make([]any, 0, len(events))
	for _, e := range events {
		if em, ok := e.(map[string]any); ok && em["entity_type"] == "auth" {
			continue
		}
		kept = append(kept, e)
	}
	m["audit_events"] = kept
	m["total"] = float64(len(kept))
	return m
}

// normalize round-trips the snapshot through JSON so numbers are float64 in a
// freshly captured snapshot exactly as in one read back from a file.
func normalize(s Snapshot) (Snapshot, error) {
	b, err := json.Marshal(s)
	if err != nil { // # pragma: no cover — built from decoded JSON
		return nil, err
	}
	var out Snapshot
	if err := json.Unmarshal(b, &out); err != nil { // # pragma: no cover — just marshalled
		return nil, err
	}
	return out, nil
}

// volatileKeys are the response keys whose value legitimately differs between
// the pre-upgrade read and the post-upgrade read, each with the reason. This
// is the harness's DeliberateException register: adding a key here is a
// reviewed decision that the datum is not user-visible meaning. Nothing that
// could carry user data belongs here.
var volatileKeys = map[string]string{
	"next_cursor":  "an opaque cursor that encodes the (updated_at, id) position",
	"sync":         "sync-protocol advertisement, versioned with the API",
	"last_used_at": "the post-upgrade login/bearer calls touch it",
	// The audit hash chain is recomputed by upgrade backfills
	// (atrest.RecomputeAuditChain); its integrity is asserted by
	// verify's audit-chain check, not by equality with the old hashes.
	"hash":      "audit chain is recomputed by the upgrade; verified separately",
	"prev_hash": "audit chain is recomputed by the upgrade; verified separately",
}

// Compare reports every place where post fails to say what pre said. It is a
// SUBSET comparison: a key present in post but not in pre is fine (a newer
// release adds response fields — that is additive, not data loss), but every
// scalar pre carried must be present and equal, every list must keep its
// length, and the per-id HTTP status of each probed contact must match.
func Compare(pre, post Snapshot) []string {
	var diffs []string
	compareValue("", map[string]any(pre), map[string]any(post), &diffs)
	sort.Strings(diffs)
	return diffs
}

func compareValue(path string, pre, post any, diffs *[]string) {
	switch p := pre.(type) {
	case map[string]any:
		q, ok := post.(map[string]any)
		if !ok {
			*diffs = append(*diffs, fmt.Sprintf("%s: was an object, now %T", pathOrRoot(path), post))
			return
		}
		for k, pv := range p {
			if _, skip := volatileKeys[k]; skip {
				continue
			}
			qv, present := q[k]
			if !present {
				*diffs = append(*diffs, fmt.Sprintf("%s.%s: present before the upgrade (%s), missing after", pathOrRoot(path), k, brief(pv)))
				continue
			}
			compareValue(path+"."+k, pv, qv, diffs)
		}
	case []any:
		q, ok := post.([]any)
		if !ok {
			*diffs = append(*diffs, fmt.Sprintf("%s: was a list, now %T", pathOrRoot(path), post))
			return
		}
		if len(p) != len(q) {
			*diffs = append(*diffs, fmt.Sprintf("%s: list length %d before, %d after", pathOrRoot(path), len(p), len(q)))
			return
		}
		for i := range p {
			compareValue(fmt.Sprintf("%s[%d]", path, i), p[i], q[i], diffs)
		}
	default:
		if !scalarEqual(pre, post) {
			*diffs = append(*diffs, fmt.Sprintf("%s: %s before, %s after", pathOrRoot(path), brief(pre), brief(post)))
		}
	}
}

func pathOrRoot(p string) string {
	if p == "" {
		return "$"
	}
	return "$" + p
}

func scalarEqual(a, b any) bool {
	return fmt.Sprintf("%T:%v", a, a) == fmt.Sprintf("%T:%v", b, b)
}

func brief(v any) string {
	b, err := json.Marshal(v)
	if err != nil { // # pragma: no cover — v came from json.Unmarshal
		return fmt.Sprint(v)
	}
	s := string(b)
	if len(s) > 120 {
		s = s[:120] + "..."
	}
	return strings.TrimSpace(s)
}
