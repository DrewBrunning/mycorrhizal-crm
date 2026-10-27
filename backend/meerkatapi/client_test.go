package meerkatapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() { DisableRateLimitForTesting() }

const goodToken = "meerkat_token"

// fakeMeerkat serves the slice of the Meerkat v1.7 API the importer reads.
type fakeMeerkat struct {
	contacts   []map[string]any
	contactKey func(map[string]any) // lets a test reshape the contact object
	logouts    atomic.Int32
	loginHits  atomic.Int32
	pageHits   atomic.Int32
	ignorePage bool // serve page 1 whatever ?page= says
	pageShift  int  // pages after the first start this many rows early (offset drift)
}

func (f *fakeMeerkat) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "healthy", "version": "0.1.0", "database": map[string]any{"status": "healthy"}})
	})
	authed := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+goodToken {
				http.Error(w, `{"error":"no"}`, http.StatusUnauthorized)
				return
			}
			next(w, r)
		}
	}
	mux.HandleFunc("/api/v1/login", func(w http.ResponseWriter, r *http.Request) {
		f.loginHits.Add(1)
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in["identifier"] != "ada" || in["password"] != "pw" {
			http.Error(w, `{"error":"bad"}`, http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "auth_token", Value: goodToken})
		_, _ = w.Write([]byte(`{"language":"en"}`))
	})
	mux.HandleFunc("/api/v1/logout", authed(func(w http.ResponseWriter, r *http.Request) {
		f.logouts.Add(1)
		_, _ = w.Write([]byte(`{}`))
	}))
	mux.HandleFunc("/api/v1/users/me", authed(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":7,"username":"ada","email":"ada@example.com"}`))
	}))
	mux.HandleFunc("/api/v1/contacts", authed(func(w http.ResponseWriter, r *http.Request) {
		f.pageHits.Add(1)
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		page, _ := strconv.Atoi(q.Get("page"))
		if page < 1 || f.ignorePage {
			page = 1
		}
		start := (page - 1) * limit
		if page > 1 {
			start = max(start-f.pageShift, 0)
		}
		end := start + limit
		if start > len(f.contacts) {
			start = len(f.contacts)
		}
		if end > len(f.contacts) {
			end = len(f.contacts)
		}
		out := []map[string]any{}
		for _, c := range f.contacts[start:end] {
			if f.contactKey != nil {
				f.contactKey(c)
			}
			out = append(out, c)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"contacts": out, "total": len(f.contacts), "page": page, "limit": limit})
	}))
	mux.HandleFunc("/api/v1/contacts/1/profile_picture", authed(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("PNGDATA"))
	}))
	return mux
}

func contactObj(id int, first string) map[string]any {
	c := map[string]any{"ID": id, "firstname": first, "archived": false}
	for _, k := range requiredContactKeys {
		if _, ok := c[k]; !ok {
			c[k] = ""
		}
	}
	c["emails"] = []map[string]string{{"type": "work", "value": first + "@example.com"}}
	c["circles"] = []string{"Family"}
	c["custom_fields"] = map[string]string{"Pet": "Cat"}
	c["photo"] = ""
	return c
}

func newTestClient(t *testing.T, f *fakeMeerkat) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL, false)
	require.NoError(t, err)
	return c, srv
}

func TestNewClient_URLValidationAndNormalization(t *testing.T) {
	for _, bad := range []string{"", "ftp://x", "http://", "http://u:p@host", "not a url"} {
		_, err := NewClient(bad, false)
		assert.ErrorIs(t, err, ErrInvalidURL, bad)
	}
	for in, want := range map[string]string{
		"https://m.example.com/":       "https://m.example.com",
		"https://m.example.com/api/v1": "https://m.example.com",
		"https://m.example.com/api":    "https://m.example.com",
		"http://m.example.com/sub/":    "http://m.example.com/sub",
	} {
		c, err := NewClient(in, false)
		require.NoError(t, err, in)
		assert.Equal(t, want, c.baseURL.String(), in)
	}
}

func TestInsecureRemote(t *testing.T) {
	for url, want := range map[string]bool{
		"http://meerkat.example.com":         true,
		"http://8.8.8.8:8080":                true,
		"https://meerkat.example.com":        false,
		"http://localhost:8080":              false,
		"http://127.0.0.1:8080":              false,
		"http://192.168.1.5":                 false,
		"http://meerkat:8080":                false, // docker service name
		"http://box.lan":                     false,
		"http://[2606:4700:4700::1111]:8080": true, // public IPv6 literal (no dots)
		"http://[::1]:8080":                  false,
		"http://[fd00::1]":                   false,
	} {
		c, err := NewClient(url, false)
		require.NoError(t, err)
		assert.Equal(t, want, c.InsecureRemote(), url)
	}
}

func TestLogin_SuccessUsesSessionAndCloseLogsOut(t *testing.T) {
	f := &fakeMeerkat{}
	c, _ := newTestClient(t, f)
	require.NoError(t, c.Login(context.Background(), "ada", "pw"))
	me, err := c.Me(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(7), me.ID)

	c.Close(context.Background())
	assert.EqualValues(t, 1, f.logouts.Load(), "password sessions are ended on the Meerkat side")
	assert.Empty(t, c.token(), "credential wiped")
	c.Close(context.Background())
	assert.EqualValues(t, 1, f.logouts.Load(), "Close is idempotent")
}

func TestLogin_BadCredentials(t *testing.T) {
	c, _ := newTestClient(t, &fakeMeerkat{})
	assert.ErrorIs(t, c.Login(context.Background(), "ada", "wrong"), ErrUnauthorized)
}

func TestLogin_LockedAccountIsNotRetried(t *testing.T) {
	f := &fakeMeerkat{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.loginHits.Add(1)
		w.Header().Set("Retry-After", "0")
		http.Error(w, "{}", http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c, _ := NewClient(srv.URL, false)
	assert.ErrorIs(t, c.Login(context.Background(), "ada", "pw"), ErrLocked)
	assert.EqualValues(t, 1, f.loginHits.Load(), "a lockout response must not be hammered")
}

func TestTokenAuthDoesNotLogOutOnClose(t *testing.T) {
	f := &fakeMeerkat{}
	c, _ := newTestClient(t, f)
	c.SetToken(goodToken)
	_, err := c.Me(context.Background())
	require.NoError(t, err)
	c.Close(context.Background())
	assert.Zero(t, f.logouts.Load(), "API tokens belong to the user and are not revoked")
	assert.Empty(t, c.token())
}

func TestUnauthorizedToken(t *testing.T) {
	c, _ := newTestClient(t, &fakeMeerkat{})
	c.SetToken("nope")
	_, err := c.Me(context.Background())
	assert.ErrorIs(t, err, ErrUnauthorized)
}

func TestDetect(t *testing.T) {
	t.Run("supported v1.5-v1.7 shape", func(t *testing.T) {
		f := &fakeMeerkat{contacts: []map[string]any{contactObj(1, "Ada")}}
		c, _ := newTestClient(t, f)
		c.SetToken(goodToken)
		v, err := c.Detect(context.Background())
		require.NoError(t, err)
		assert.Equal(t, VersionInfo{Generation: GenerationV15toV17, Supported: true}, v)
	})
	t.Run("empty account is accepted", func(t *testing.T) {
		c, _ := newTestClient(t, &fakeMeerkat{})
		c.SetToken(goodToken)
		v, err := c.Detect(context.Background())
		require.NoError(t, err)
		assert.True(t, v.Supported)
	})
	t.Run("older shape is unknown", func(t *testing.T) {
		old := contactObj(1, "Ada")
		delete(old, "emails")
		c, _ := newTestClient(t, &fakeMeerkat{contacts: []map[string]any{old}})
		c.SetToken(goodToken)
		v, err := c.Detect(context.Background())
		require.NoError(t, err)
		assert.Equal(t, GenerationUnknown, v.Generation)
		assert.False(t, v.Supported)
		assert.Contains(t, v.Reason, "older")
		// Wire value: area-prefixed per ADR 0037 / #2095, so #2100 can register it unchanged.
		assert.Equal(t, "meerkat_version_older_than_supported", v.ReasonCode)
	})
	t.Run("unknown extra field means newer, untested", func(t *testing.T) {
		newer := contactObj(1, "Ada")
		newer["pronouns"] = "she/her"
		c, _ := newTestClient(t, &fakeMeerkat{contacts: []map[string]any{newer}})
		c.SetToken(goodToken)
		v, err := c.Detect(context.Background())
		require.NoError(t, err)
		assert.False(t, v.Supported)
		assert.Contains(t, v.Reason, "newer")
		assert.Equal(t, "meerkat_version_newer_than_tested", v.ReasonCode)
	})
	t.Run("non-Meerkat host is rejected before any credential is sent", func(t *testing.T) {
		var sawAuth atomic.Bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "" {
				sawAuth.Store(true)
			}
			_, _ = w.Write([]byte(`<html>hi</html>`))
		}))
		defer srv.Close()
		c, _ := NewClient(srv.URL, false)
		c.SetToken(goodToken)
		_, err := c.Detect(context.Background())
		assert.ErrorIs(t, err, ErrNotMeerkat)
		assert.False(t, sawAuth.Load(), "the token must not reach a host that fails the /health signature")
	})
}

func TestFetchSnapshot_PagesAndAdaptsEverything(t *testing.T) {
	var contacts []map[string]any
	for i := 1; i <= 205; i++ {
		contacts = append(contacts, contactObj(i, fmt.Sprintf("C%d", i)))
	}
	contacts[0]["photo"] = "abc.jpg"
	contacts[0]["relationships"] = []map[string]any{{"ID": 11, "name": "Bob", "type": "Father", "contact_id": 1, "related_contact_id": 2}}
	contacts[0]["notes"] = []map[string]any{{"ID": 21, "content": "hello", "date": "2024-01-02T03:04:05Z", "contact_id": 1}}
	act := map[string]any{"ID": 31, "title": "Lunch", "date": "2024-02-02T12:00:00Z"}
	contacts[0]["activities"] = []map[string]any{act}
	contacts[1]["activities"] = []map[string]any{act}
	contacts[0]["reminders"] = []map[string]any{{"ID": 41, "message": "Call", "remind_at": "2024-03-03T00:00:00Z", "recurrence": "yearly", "by_mail": true, "completed": true, "contact_id": 1}}
	contacts[2]["archived"] = true

	f := &fakeMeerkat{contacts: contacts}
	c, _ := newTestClient(t, f)
	c.SetToken(goodToken)

	var lastDone, lastTotal int
	snap, err := c.FetchSnapshot(context.Background(), Me{ID: 7, Username: "ada", Email: "a@x"}, Limits{}, func(d, tot int) { lastDone, lastTotal = d, tot })
	require.NoError(t, err)

	assert.Len(t, snap.Contacts, 205)
	assert.Equal(t, 205, lastDone)
	assert.Equal(t, 205, lastTotal)
	assert.EqualValues(t, 3, f.pageHits.Load(), "100 per page")
	assert.True(t, snap.PhotosFetchable)
	require.NotNil(t, snap.SourceUserID)
	assert.EqualValues(t, 7, *snap.SourceUserID)

	assert.EqualValues(t, 1, *snap.Contacts[2].Archived, "archived flag carried")
	require.Len(t, snap.Relationships, 1)
	assert.Equal(t, "Father", *snap.Relationships[0].Type)
	assert.EqualValues(t, 2, *snap.Relationships[0].RelatedContact)
	require.Len(t, snap.Notes, 1)
	assert.Equal(t, "hello", *snap.Notes[0].Content)
	require.Len(t, snap.Activities, 1, "an activity shared by two contacts is one activity")
	assert.Len(t, snap.ActivityContacts, 2)
	require.Len(t, snap.Reminders, 1)
	assert.EqualValues(t, 1, *snap.Reminders[0].ByMail)
	assert.EqualValues(t, 1, *snap.Reminders[0].Completed)

	c0 := snap.Contacts[0]
	assert.JSONEq(t, `[{"type":"work","value":"C1@example.com"}]`, *c0.EmailsJSON)
	assert.JSONEq(t, `["Family"]`, *c0.CirclesJSON)
	assert.JSONEq(t, `{"Pet":"Cat"}`, *c0.CustomFields)
}

func TestFetchSnapshot_Caps(t *testing.T) {
	var contacts []map[string]any
	for i := 1; i <= 5; i++ {
		contacts = append(contacts, contactObj(i, "C"))
	}
	c, _ := newTestClient(t, &fakeMeerkat{contacts: contacts})
	c.SetToken(goodToken)
	_, err := c.FetchSnapshot(context.Background(), Me{ID: 1}, Limits{MaxContacts: 4}, nil)
	assert.ErrorIs(t, err, ErrTooLarge)
	_, err = c.FetchSnapshot(context.Background(), Me{ID: 1}, Limits{MaxEntities: 3}, nil)
	assert.ErrorIs(t, err, ErrTooLarge)
}

func manyContacts(n int) []map[string]any {
	var contacts []map[string]any
	for i := 1; i <= n; i++ {
		contacts = append(contacts, contactObj(i, fmt.Sprintf("C%d", i)))
	}
	return contacts
}

func TestFetchSnapshot_ServerIgnoringPageIsAnErrorNotDuplicates(t *testing.T) {
	contacts := manyContacts(150)
	contacts[0]["notes"] = []map[string]any{{"ID": 21, "content": "hello", "contact_id": 1}}
	f := &fakeMeerkat{contacts: contacts, ignorePage: true}
	c, _ := newTestClient(t, f)
	c.SetToken(goodToken)
	snap, err := c.FetchSnapshot(context.Background(), Me{ID: 1}, Limits{}, nil)
	assert.ErrorIs(t, err, ErrInvalidData)
	assert.Nil(t, snap, "a partial snapshot is never returned as success")
	assert.EqualValues(t, 2, f.pageHits.Load(), "gives up as soon as a page adds nothing new")
}

func TestFetchSnapshot_RepeatedContactsAcrossPagesAreDropped(t *testing.T) {
	contacts := manyContacts(150)
	contacts[95]["notes"] = []map[string]any{{"ID": 22, "content": "dup-prone", "contact_id": 96}}
	contacts[95]["relationships"] = []map[string]any{{"ID": 12, "name": "Bob", "type": "Friend", "contact_id": 96, "related_contact_id": 1}}
	contacts[95]["reminders"] = []map[string]any{{"ID": 42, "message": "Call", "contact_id": 96}}
	contacts[95]["activities"] = []map[string]any{{"ID": 32, "title": "Lunch"}}
	// Page 2 starts 10 rows early, so contacts 91-100 are served twice.
	c, _ := newTestClient(t, &fakeMeerkat{contacts: contacts, pageShift: 10})
	c.SetToken(goodToken)
	snap, err := c.FetchSnapshot(context.Background(), Me{ID: 1}, Limits{}, nil)
	require.NoError(t, err)
	require.Len(t, snap.Contacts, 150)
	seen := map[int64]bool{}
	for _, ct := range snap.Contacts {
		assert.False(t, seen[ct.ID], "contact %d appears once", ct.ID)
		seen[ct.ID] = true
	}
	assert.Len(t, snap.Notes, 1, "a repeated contact's nested rows are not repeated")
	assert.Len(t, snap.Relationships, 1)
	assert.Len(t, snap.Reminders, 1)
	assert.Len(t, snap.Activities, 1)
	assert.Len(t, snap.ActivityContacts, 1)
}

func TestFetchSnapshot_ExhaustingMaxPagesIsAnError(t *testing.T) {
	old := maxPages
	maxPages = 1
	t.Cleanup(func() { maxPages = old })
	c, _ := newTestClient(t, &fakeMeerkat{contacts: manyContacts(150)})
	c.SetToken(goodToken)
	snap, err := c.FetchSnapshot(context.Background(), Me{ID: 1}, Limits{}, nil)
	assert.ErrorIs(t, err, ErrInvalidData)
	assert.Nil(t, snap)
}

func TestFetchSnapshot_EmptyPageEndsWalkWhenContactsDeletedMidWalk(t *testing.T) {
	f := &fakeMeerkat{contacts: manyContacts(100)}
	// Total says 150 on page 1, but the extra rows vanish before page 2.
	var hits atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := httptest.NewRecorder()
		f.handler().ServeHTTP(rec, r)
		body := rec.Body.Bytes()
		if r.URL.Path == "/api/v1/contacts" && hits.Add(1) == 1 {
			body = []byte(strings.Replace(string(body), `"total":100`, `"total":150`, 1))
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(proxy.Close)
	pc, err := NewClient(proxy.URL, false)
	require.NoError(t, err)
	pc.SetToken(goodToken)
	snap, err := pc.FetchSnapshot(context.Background(), Me{ID: 1}, Limits{}, nil)
	require.NoError(t, err)
	assert.Len(t, snap.Contacts, 100)
}

func TestFetchSnapshot_ContextCancel(t *testing.T) {
	c, _ := newTestClient(t, &fakeMeerkat{contacts: []map[string]any{contactObj(1, "A")}})
	c.SetToken(goodToken)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.FetchSnapshot(ctx, Me{ID: 1}, Limits{}, nil)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestFetchProfilePicture(t *testing.T) {
	c, _ := newTestClient(t, &fakeMeerkat{})
	c.SetToken(goodToken)
	data, mt, err := c.FetchProfilePicture(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, "PNGDATA", string(data))
	assert.Equal(t, "image/png", mt)
	_, _, err = c.FetchProfilePicture(context.Background(), 2)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestFetchProfilePicture_TooLarge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", MaxPhotoBytes+10)))
	}))
	defer srv.Close()
	c, _ := NewClient(srv.URL, false)
	_, _, err := c.FetchProfilePicture(context.Background(), 1)
	assert.ErrorIs(t, err, ErrInvalidData)
}

func TestRedirectsAreNotFollowed(t *testing.T) {
	var hits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		assert.Empty(t, r.Header.Get("Authorization"))
	}))
	defer target.Close()
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusFound)
	}))
	defer redir.Close()
	c, _ := NewClient(redir.URL, false)
	c.SetToken(goodToken)
	_, err := c.Me(context.Background())
	assert.ErrorIs(t, err, ErrInvalidData)
	assert.Zero(t, hits.Load(), "a redirect must never be followed with the bearer attached")
}

func TestPrivateAddressBlockedWhenConfigured(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close() // listens on 127.0.0.1
	c, err := NewClient(srv.URL, true)
	require.NoError(t, err)
	c.SetToken(goodToken)
	_, err = c.Me(context.Background())
	assert.ErrorIs(t, err, ErrPrivateAddress)
}

func TestServerErrorsRetryThenFail(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Retry-After", "0")
		http.Error(w, "x", http.StatusBadGateway)
	}))
	defer srv.Close()
	c, _ := NewClient(srv.URL, false)
	c.SetToken(goodToken)
	_, err := c.Me(context.Background())
	assert.ErrorIs(t, err, ErrUnreachable)
	assert.EqualValues(t, maxRetries+1, hits.Load())
}

func TestInvalidJSONAndOversizeBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<<not json"))
	}))
	defer srv.Close()
	c, _ := NewClient(srv.URL, false)
	_, err := c.Me(context.Background())
	assert.ErrorIs(t, err, ErrInvalidData)
}

func TestErrorsNeverContainCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "x", 418) }))
	defer srv.Close()
	c, _ := NewClient(srv.URL, false)
	c.SetToken("SECRET-TOKEN-VALUE")
	_, err := c.Me(context.Background())
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "SECRET-TOKEN-VALUE")
	err = c.Login(context.Background(), "ada", "SECRET-PASSWORD")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "SECRET-PASSWORD")
}
