package meerkatapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// Generation labels the API adapter a server was matched to. Meerkat's
// /health reports a hard-coded "0.1.0", so the version cannot be read; it is
// inferred from the shape of the API (feature detection).
const (
	// GenerationV15toV17 covers Meerkat v1.5.0–v1.7.0: the multi-valued
	// contact fields (emails/phones/addresses/urls/impps), structured name and
	// organization fields, anniversary, and ?includes= preloading. The data
	// API did not change across that range (verified by diffing the tags).
	GenerationV15toV17 = "v1.5-v1.7"
	// GenerationUnknown means the signature did not match any adapter.
	GenerationUnknown = "unknown"
)

// Reason codes name why a server is unsupported, so clients can show a
// translated sentence; Reason stays as the English fallback for a code a
// client does not know. Empty when Supported.
const (
	// ReasonOlderThanSupported: the contacts lack fields the adapter reads.
	ReasonOlderThanSupported = "older_than_supported"
	// ReasonNewerThanTested: the contacts carry fields the adapter does not know.
	ReasonNewerThanTested = "newer_than_tested"
)

// VersionInfo is the result of feature detection.
type VersionInfo struct {
	// Generation is the matched adapter label, or GenerationUnknown.
	Generation string `json:"generation"`
	// Supported reports whether the adapter is tested against this server. An
	// unsupported server may still be tried, preview-only.
	Supported bool `json:"supported"`
	// Reason explains an unsupported result in one user-facing sentence.
	Reason string `json:"reason,omitempty"`
	// ReasonCode is the stable, translatable form of Reason.
	ReasonCode string `json:"reason_code,omitempty"`
}

// requiredContactKeys are the contact fields the v1.5–v1.7 adapter reads;
// their absence means an older (or very different) Meerkat.
var requiredContactKeys = []string{
	"firstname", "lastname", "emails", "phones", "addresses", "urls", "impps",
	"prefix", "middle_name", "suffix", "organization", "department", "job_title",
	"role", "anniversary", "circles", "custom_fields", "archived",
}

// knownContactKeys is every key the v1.5–v1.7 contact object carries. A key
// outside it means a newer server than the importer was tested against.
var knownContactKeys = func() map[string]bool {
	m := map[string]bool{}
	for _, k := range requiredContactKeys {
		m[k] = true
	}
	for _, k := range []string{
		"ID", "CreatedAt", "UpdatedAt", "DeletedAt", "nickname", "gender", "email",
		"phone", "birthday", "photo", "photo_thumbnail", "address", "how_we_met",
		"food_preference", "work_information", "contact_information",
		"relationships", "activities", "notes", "reminders",
	} {
		m[k] = true
	}
	return m
}()

// Detect identifies the server. It first checks the unauthenticated /health
// signature (so a non-Meerkat host fails fast, before any credential is
// sent), then inspects the shape of an authenticated contacts page. Call it
// after SetToken or Login.
func (c *Client) Detect(ctx context.Context) (VersionInfo, error) {
	if err := c.CheckHealth(ctx); err != nil {
		return VersionInfo{}, err
	}
	var page struct {
		Contacts []map[string]json.RawMessage `json:"contacts"`
	}
	q := url.Values{"limit": {"1"}, "include_archived": {"true"}}
	if err := c.getJSON(ctx, "/api/v1/contacts", q, &page); err != nil {
		return VersionInfo{}, err
	}
	if page.Contacts == nil {
		return VersionInfo{}, ErrNotMeerkat
	}
	if len(page.Contacts) == 0 {
		// An empty account has no shape to probe; the envelope matched.
		return VersionInfo{Generation: GenerationV15toV17, Supported: true}, nil
	}
	first := page.Contacts[0]
	for _, k := range requiredContactKeys {
		if _, ok := first[k]; !ok {
			return VersionInfo{
				Generation: GenerationUnknown,
				Reason:     "This Meerkat looks older than v1.5 (its contacts lack fields the importer reads).",
				ReasonCode: ReasonOlderThanSupported,
			}, nil
		}
	}
	for k := range first {
		if !knownContactKeys[k] {
			return VersionInfo{
				Generation: GenerationV15toV17,
				Reason:     "This Meerkat looks newer than the versions the importer was tested with (v1.5–v1.7).",
				ReasonCode: ReasonNewerThanTested,
			}, nil
		}
	}
	return VersionInfo{Generation: GenerationV15toV17, Supported: true}, nil
}

// CheckHealth confirms the host answers like Meerkat's /health before any
// credential is sent to it.
func (c *Client) CheckHealth(ctx context.Context) error {
	resp, err := c.do(ctx, http.MethodGet, "/health", nil, nil, false)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotMeerkat
	}
	var h struct {
		Status   string          `json:"status"`
		Database json.RawMessage `json:"database"`
	}
	if err := json.NewDecoder(limitBody(resp)).Decode(&h); err != nil || h.Status == "" || h.Database == nil {
		return ErrNotMeerkat
	}
	return nil
}
