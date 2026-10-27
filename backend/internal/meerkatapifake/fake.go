// Package meerkatapifake is an httptest stand-in for the slice of the Meerkat
// v1.5–v1.7 REST API the live importer reads (issue #2013). Test-only: it is
// never linked into the server binary.
package meerkatapifake

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const (
	// Token is the API token the fake accepts.
	Token = "mk_fake_token" // #nosec G101 -- test fixture, not a credential
	// Username / Password are the login the fake accepts.
	Username = "ada"
	Password = "correct horse"
)

// Server is a running fake Meerkat.
type Server struct {
	*httptest.Server
	mu sync.Mutex
	// Contacts are served from /api/v1/contacts. Mutate before the first request.
	Contacts []map[string]any
	// Pictures maps contact id → PNG bytes served at /contacts/:id/profile_picture.
	Pictures map[int64][]byte
	// Reshape, when set, rewrites each served contact (e.g. to fake another version).
	Reshape func(map[string]any)
	// BreakSignature makes /health answer like a non-Meerkat host.
	BreakSignature bool
	// FailMe makes /users/me answer 500.
	FailMe bool

	logouts  int
	Requests []string // "METHOD path" log, for assertions
	AuthSeen []string // Authorization header of every authenticated request
}

// New starts a fake with no contacts.
func New(t testing.TB) *Server {
	t.Helper()
	s := &Server{Pictures: map[int64][]byte{}}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

// Logouts reports how many times /logout was called.
func (s *Server) Logouts() int { s.mu.Lock(); defer s.mu.Unlock(); return s.logouts }

// Contact builds a contact object carrying every v1.5–v1.7 key.
func Contact(id int, first string) map[string]any {
	return map[string]any{
		"ID": id, "firstname": first, "lastname": "Test", "nickname": "", "gender": "", "email": "", "phone": "",
		"birthday": "", "photo": "", "address": "", "how_we_met": "", "food_preference": "", "work_information": "",
		"contact_information": "", "circles": []string{}, "custom_fields": map[string]string{},
		"emails": []map[string]string{{"type": "home", "value": strings.ToLower(first) + "@example.com"}},
		"phones": []any{}, "addresses": []any{}, "urls": []any{}, "impps": []any{},
		"prefix": "", "middle_name": "", "suffix": "", "organization": "", "department": "", "job_title": "",
		"role": "", "anniversary": "", "archived": false, "photo_thumbnail": "",
	}
}

// PNG returns a small valid PNG.
func PNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for x := 0; x < 8; x++ {
		for y := 0; y < 8; y++ {
			img.Set(x, y, color.RGBA{R: 200, G: 30, B: 30, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.Requests = append(s.Requests, r.Method+" "+r.URL.Path)
	s.mu.Unlock()
	path := r.URL.Path
	if path == "/health" {
		if s.BreakSignature {
			_, _ = w.Write([]byte("<html>not meerkat</html>"))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "healthy", "version": "0.1.0", "database": map[string]any{"status": "healthy"}})
		return
	}
	if path == "/api/v1/login" {
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in["identifier"] != Username || in["password"] != Password {
			http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "auth_token", Value: Token, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: true})
		_, _ = w.Write([]byte(`{"language":"en"}`))
		return
	}
	auth := r.Header.Get("Authorization")
	s.mu.Lock()
	s.AuthSeen = append(s.AuthSeen, auth)
	s.mu.Unlock()
	if auth != "Bearer "+Token {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	switch {
	case path == "/api/v1/logout":
		s.mu.Lock()
		s.logouts++
		s.mu.Unlock()
		_, _ = w.Write([]byte(`{}`))
	case path == "/api/v1/users/me" && s.FailMe:
		http.Error(w, "boom", http.StatusInternalServerError)
	case path == "/api/v1/users/me":
		_, _ = w.Write([]byte(`{"id":7,"username":"ada","email":"ada@example.com"}`))
	case path == "/api/v1/contacts":
		s.serveContacts(w, r)
	case strings.HasPrefix(path, "/api/v1/contacts/") && strings.HasSuffix(path, "/profile_picture"):
		id, _ := strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(path, "/api/v1/contacts/"), "/profile_picture"), 10, 64)
		pic, ok := s.Pictures[id]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pic)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) serveContacts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	page, _ := strconv.Atoi(q.Get("page"))
	page = max(page, 1)
	start, end := (page-1)*limit, page*limit
	n := len(s.Contacts)
	if start > n {
		start = n
	}
	if end > n {
		end = n
	}
	out := []map[string]any{}
	for _, c := range s.Contacts[start:end] {
		if s.Reshape != nil {
			s.Reshape(c)
		}
		out = append(out, c)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"contacts": out, "total": n, "page": page, "limit": limit})
}
