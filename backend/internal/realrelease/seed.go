package realrelease

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"time"

	"github.com/pquerna/otp/totp"
)

// SeedPassword is the shared password of the synthetic seeded account. It is
// throwaway test data on a throwaway instance (CLAUDE.md "Testing the user's
// own application"): never a real credential, never used outside this harness.
const SeedPassword = "Rr-Seed-Pass-93!xK" //nolint:gosec // G101: published throwaway password for a synthetic account on a throwaway instance, not a credential

// SeedUsername is the synthetic account name.
const SeedUsername = "realdata"

// AttachmentBytes is the synthetic attachment payload; its sha256 is part of
// the snapshot so the on-disk file (the third piece of an install) is proven
// to survive the upgrade byte for byte.
var AttachmentBytes = []byte("synthetic attachment payload for issue 1489\n")

// Credentials is everything Verify needs to log back in to the seeded
// instance after the upgrade. It is written to a file by `realrelease seed`
// and read by `realrelease verify`; all of it is synthetic.
type Credentials struct {
	Username      string   `json:"username"`
	Password      string   `json:"password"`
	TOTPSecret    string   `json:"totp_secret"`
	RecoveryCodes []string `json:"recovery_codes"`
	APIToken      string   `json:"api_token"`
}

// Seed drives a running instance's public API into a representative state and
// returns the credentials to log back in. 2FA is enabled LAST so that Capture
// (which needs a plain password session) can run first against the same
// session, and so the TOTP step burned at enrollment is behind us.
//
// Everything here is available in every SupportedReleases API; a new write
// path that older releases lack must not be added unconditionally.
func Seed(ctx context.Context, c *Client) (*Credentials, error) {
	creds := &Credentials{Username: SeedUsername, Password: SeedPassword}

	if err := c.call(ctx, http.MethodPost, "/register", map[string]any{
		"username": SeedUsername, "email": "realdata@example.test", "password": SeedPassword,
	}, nil); err != nil {
		return nil, fmt.Errorf("register: %w", err)
	}
	if err := c.call(ctx, http.MethodPost, "/login", map[string]any{
		"identifier": SeedUsername, "password": SeedPassword,
	}, nil); err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}

	type created struct {
		ID  uint   `json:"id"`
		UID string `json:"uid"`
	}
	newContact := func(body map[string]any) (created, error) {
		var out struct {
			Contact created `json:"contact"`
		}
		err := c.call(ctx, http.MethodPost, "/contacts", body, &out)
		return out.Contact, err
	}

	ada, err := newContact(map[string]any{
		"gender": "female",
		"card": map[string]any{
			"name": map[string]any{"components": []map[string]any{
				{"kind": "given", "value": "Ada"}, {"kind": "surname", "value": "Lovelace"},
			}, "isOrdered": true},
			"nicknames":     []map[string]any{{"name": "Countess"}},
			"emails":        []map[string]any{{"address": "ada@example.test", "contexts": []string{"work"}, "pref": 1}, {"address": "ada.home@example.test"}},
			"phones":        []map[string]any{{"number": "+15551230001", "features": []string{"cell"}}, {"number": "+15551230002"}},
			"addresses":     []map[string]any{{"components": []map[string]any{{"kind": "name", "value": "1 Main St"}, {"kind": "locality", "value": "London"}, {"kind": "country", "value": "UK"}}}},
			"organizations": []map[string]any{{"name": "Analytical Engines Ltd"}},
			"anniversaries": []map[string]any{{"kind": "birth", "date": map[string]any{"partial": map[string]any{"year": 1815, "month": 12, "day": 10}}}},
			"notes":         []map[string]any{{"note": "Synthetic card note"}},
		},
		"crm": map[string]any{"circles": []string{"friends", "work"}, "how_we_met": "Conference"},
	})
	if err != nil {
		return nil, fmt.Errorf("create ada: %w", err)
	}
	grace, err := newContact(minimalContact("Grace", "Hopper", "grace@example.test"))
	if err != nil {
		return nil, fmt.Errorf("create grace: %w", err)
	}
	gone, err := newContact(minimalContact("Gina", "Gone", "gina@example.test"))
	if err != nil {
		return nil, fmt.Errorf("create gina: %w", err)
	}
	keep, err := newContact(minimalContact("Dup", "Person", "dup@example.test"))
	if err != nil {
		return nil, fmt.Errorf("create merge keeper: %w", err)
	}
	loser, err := newContact(minimalContact("Dup", "Person", "dup2@example.test"))
	if err != nil {
		return nil, fmt.Errorf("create merge loser: %w", err)
	}
	archived, err := newContact(minimalContact("Archie", "Vale", "archie@example.test"))
	if err != nil {
		return nil, fmt.Errorf("create archived: %w", err)
	}
	// Unicode + odd-but-legal values: the legacy-value class a transplant
	// never produces.
	if _, err := newContact(minimalContact("Zoë", "Ünïcode-O'Brien", "zoe@example.test")); err != nil {
		return nil, fmt.Errorf("create unicode contact: %w", err)
	}

	// Notes (one later soft-deleted) and activities (one later soft-deleted).
	var note1, note2 struct {
		Note struct {
			ID uint `json:"ID"`
		} `json:"note"`
	}
	if err := c.call(ctx, http.MethodPost, fmt.Sprintf("/contacts/%d/notes", ada.ID),
		map[string]any{"content": "Met at the engine demo", "date": "2026-03-01T10:00:00Z"}, &note1); err != nil {
		return nil, fmt.Errorf("note: %w", err)
	}
	if err := c.call(ctx, http.MethodPost, fmt.Sprintf("/contacts/%d/notes", ada.ID),
		map[string]any{"content": "Note that will be soft-deleted", "date": "2026-03-02T10:00:00Z"}, &note2); err != nil {
		return nil, fmt.Errorf("note: %w", err)
	}
	var act1, act2 struct {
		Activity struct {
			ID uint `json:"ID"`
		} `json:"activity"`
	}
	if err := c.call(ctx, http.MethodPost, "/activities", map[string]any{
		"title": "Coffee", "description": "Catch-up", "location": "Cafe", "date": "2026-03-02T10:00:00Z",
		"contact_ids": []uint{ada.ID, grace.ID}, "type": "meal",
	}, &act1); err != nil {
		return nil, fmt.Errorf("activity: %w", err)
	}
	if err := c.call(ctx, http.MethodPost, "/activities", map[string]any{
		"title": "Activity that will be soft-deleted", "date": "2026-03-03T10:00:00Z", "contact_ids": []uint{ada.ID},
	}, &act2); err != nil {
		return nil, fmt.Errorf("activity: %w", err)
	}

	// Graph + grouping entities.
	var circle struct {
		Circle struct {
			ID string `json:"id"`
		} `json:"circle"`
	}
	if err := c.call(ctx, http.MethodPost, "/circles", map[string]any{"name": "Book club"}, &circle); err != nil {
		return nil, fmt.Errorf("circle: %w", err)
	}
	if err := c.call(ctx, http.MethodPost, "/circles/"+circle.Circle.ID+"/members",
		map[string]any{"member_vcard_uid": ada.UID}, nil); err != nil {
		return nil, fmt.Errorf("circle member: %w", err)
	}
	var tag struct {
		Tag struct {
			ID string `json:"id"`
		} `json:"tag"`
	}
	if err := c.call(ctx, http.MethodPost, "/tags", map[string]any{"name": "vip"}, &tag); err != nil {
		return nil, fmt.Errorf("tag: %w", err)
	}
	if err := c.call(ctx, http.MethodPost, "/tags/"+tag.Tag.ID+"/contacts",
		map[string]any{"contact_vcard_uid": grace.UID}, nil); err != nil {
		return nil, fmt.Errorf("tag contact: %w", err)
	}
	if err := c.call(ctx, http.MethodPost, "/relationship-edges", map[string]any{
		"source_id": ada.UID, "target_id": grace.UID, "type": "friend_of",
	}, nil); err != nil {
		return nil, fmt.Errorf("relationship edge: %w", err)
	}
	if err := c.call(ctx, http.MethodPost, "/life-events", map[string]any{
		"entity_id": ada.UID, "type": "moved", "category": "home_living",
		"date": map[string]any{"year": 2024, "month": 6, "day": 1}, "description": "Moved house",
	}, nil); err != nil {
		return nil, fmt.Errorf("life event: %w", err)
	}

	// The on-disk piece: an attachment.
	if err := c.upload(ctx, fmt.Sprintf("/contacts/%d/attachments", ada.ID), "seed.txt", AttachmentBytes); err != nil {
		return nil, fmt.Errorf("attachment: %w", err)
	}

	// Favorite + archive state.
	if err := c.call(ctx, http.MethodPost, fmt.Sprintf("/contacts/%d/favorite", grace.ID), nil, nil); err != nil {
		return nil, fmt.Errorf("favorite: %w", err)
	}
	if err := c.call(ctx, http.MethodPost, fmt.Sprintf("/contacts/%d/archive", archived.ID), nil, nil); err != nil {
		return nil, fmt.Errorf("archive: %w", err)
	}

	// An UPDATE of a loaded contact, then the audit Undo of it: the T75 class
	// (a plain save of a loaded contact used to drop data with no flat home).
	if err := undoAnUpdate(ctx, c, ada.ID); err != nil {
		return nil, err
	}

	// Merge, then soft deletes (note, activity, contact) — the soft-delete
	// rows a retention/purge job and every unique index must tolerate.
	if err := c.call(ctx, http.MethodPost, "/contacts/merge",
		map[string]any{"keep_id": keep.ID, "merge_id": loser.ID, "resolutions": map[string]string{
			"email": "keep", "primary_email": "keep",
		}}, nil); err != nil {
		var ae *APIError
		if !errors.As(err, &ae) || ae.Status != http.StatusBadRequest {
			return nil, fmt.Errorf("merge: %w", err)
		}
		// Conflicting-field vocabulary differs across releases; an
		// unresolvable merge is skipped rather than failing the whole seed
		// (the soft-delete below still covers the tombstone shape).
	}
	if err := c.call(ctx, http.MethodDelete, fmt.Sprintf("/notes/%d", note2.Note.ID), nil, nil); err != nil {
		return nil, fmt.Errorf("delete note: %w", err)
	}
	if err := c.call(ctx, http.MethodDelete, fmt.Sprintf("/activities/%d", act2.Activity.ID), nil, nil); err != nil {
		return nil, fmt.Errorf("delete activity: %w", err)
	}
	if err := c.call(ctx, http.MethodDelete, fmt.Sprintf("/contacts/%d", gone.ID), nil, nil); err != nil {
		return nil, fmt.Errorf("delete contact: %w", err)
	}

	// API token (the plaintext is only ever returned here).
	var tok struct {
		Token string `json:"token"`
	}
	if err := c.call(ctx, http.MethodPost, "/api-tokens", map[string]any{"name": "realdata-seed"}, &tok); err != nil {
		return nil, fmt.Errorf("api token: %w", err)
	}
	if tok.Token == "" {
		return nil, errors.New("api token: response carried no plaintext token")
	}
	creds.APIToken = tok.Token

	return creds, nil
}

// minimalContact is a one-name, one-email contact body.
func minimalContact(given, surname, email string) map[string]any {
	return map[string]any{"card": map[string]any{
		"name": map[string]any{"components": []map[string]any{
			{"kind": "given", "value": given}, {"kind": "surname", "value": surname},
		}},
		"emails": []map[string]any{{"address": email}},
	}}
}

// undoAnUpdate edits a contact (producing an update audit event) and undoes it
// through the audit trail.
func undoAnUpdate(ctx context.Context, c *Client, id uint) error {
	path := fmt.Sprintf("/contacts/%d", id)
	var rec map[string]any
	if err := c.call(ctx, http.MethodGet, path, nil, &rec); err != nil {
		return fmt.Errorf("read contact for edit: %w", err)
	}
	uid, _ := rec["uid"].(string)
	edit := map[string]any{"card": rec["card"], "crm": rec["crm"], "gender": rec["gender"]}
	if m, ok := rec["passthrough"]; ok {
		edit["passthrough"] = m
	}
	if crm, ok := edit["crm"].(map[string]any); ok {
		crm["how_we_met"] = "Edited, then undone"
	}
	var before struct {
		AuditEvents []struct {
			ID uint `json:"id"`
		} `json:"audit_events"`
	}
	if err := c.call(ctx, http.MethodGet, "/audit?limit=1", nil, &before); err != nil {
		return fmt.Errorf("list audit: %w", err)
	}
	var newestBefore uint
	if len(before.AuditEvents) > 0 {
		newestBefore = before.AuditEvents[0].ID
	}
	if err := c.call(ctx, http.MethodPut, path, edit, nil); err != nil {
		return fmt.Errorf("edit contact: %w", err)
	}
	// Audit rows may be written asynchronously, so poll briefly for the update
	// event newer than the newest row that existed before the edit.
	for attempt := 0; attempt < 50; attempt++ {
		var events struct {
			AuditEvents []struct {
				ID        uint   `json:"id"`
				Operation string `json:"operation"`
			} `json:"audit_events"`
		}
		if err := c.call(ctx, http.MethodGet, "/audit?entity_type=contact&entity_id="+uid, nil, &events); err != nil {
			return fmt.Errorf("list audit: %w", err)
		}
		for _, e := range events.AuditEvents { // newest first
			if e.Operation == "update" && e.ID > newestBefore {
				if err := c.call(ctx, http.MethodPost, fmt.Sprintf("/audit/%d/undo", e.ID), nil, nil); err != nil {
					return fmt.Errorf("audit undo: %w", err)
				}
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return errors.New("audit: the contact edit produced no update event to undo")
}

// EnableTwoFactor enrols TOTP on the logged-in account and records the secret
// and the single-use recovery codes in creds. Call it after Capture: enrolment
// bumps token_version, invalidating every other session.
func EnableTwoFactor(ctx context.Context, c *Client, creds *Credentials) error {
	var setup struct {
		Secret string `json:"secret"`
	}
	if err := c.call(ctx, http.MethodPost, "/users/2fa/setup", nil, &setup); err != nil {
		return fmt.Errorf("2fa setup: %w", err)
	}
	code, err := totp.GenerateCode(setup.Secret, time.Now())
	if err != nil {
		return fmt.Errorf("2fa code: %w", err)
	}
	var confirm struct {
		RecoveryCodes []string `json:"recovery_codes"`
	}
	if err := c.call(ctx, http.MethodPost, "/users/2fa/confirm", map[string]any{"code": code}, &confirm); err != nil {
		return fmt.Errorf("2fa confirm: %w", err)
	}
	if len(confirm.RecoveryCodes) < 2 {
		return fmt.Errorf("2fa confirm returned %d recovery codes, want at least 2", len(confirm.RecoveryCodes))
	}
	creds.TOTPSecret = setup.Secret
	creds.RecoveryCodes = confirm.RecoveryCodes
	return nil
}

// upload sends one multipart file field named "file".
func (c *Client) upload(ctx context.Context, path, name string, data []byte) error {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, name))
	h.Set("Content-Type", "text/plain")
	part, err := mw.CreatePart(h)
	if err != nil { // # pragma: no cover — writing to a bytes.Buffer cannot fail
		return err
	}
	if _, err := part.Write(data); err != nil { // # pragma: no cover — bytes.Buffer
		return err
	}
	if err := mw.Close(); err != nil { // # pragma: no cover — bytes.Buffer
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/v1"+path, &buf)
	if err != nil { // # pragma: no cover — static method and a well-formed base
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &APIError{Method: http.MethodPost, Path: path, Status: resp.StatusCode, Body: readAll(resp)}
	}
	return nil
}

// Login logs the seeded account in on c, completing the second factor with a
// TOTP code one step ahead when 2FA is enrolled (the enrolment burned the
// current step; the server accepts +-1 step of skew, so no sleeping).
func Login(ctx context.Context, c *Client, creds *Credentials) error {
	var lr struct {
		TwoFactorRequired bool `json:"two_factor_required"`
	}
	if err := c.call(ctx, http.MethodPost, "/login", map[string]any{
		"identifier": creds.Username, "password": creds.Password,
	}, &lr); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	if !lr.TwoFactorRequired {
		return nil
	}
	code, err := totp.GenerateCode(creds.TOTPSecret, time.Now().Add(30*time.Second))
	if err != nil {
		return err
	}
	if err := c.call(ctx, http.MethodPost, "/login/2fa", map[string]any{"code": code}, nil); err != nil {
		return fmt.Errorf("login 2fa: %w", err)
	}
	return nil
}
