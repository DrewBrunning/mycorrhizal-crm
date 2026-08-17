package controllers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// fakePaperlessDoc is the server-side state for one Paperless document.
type fakePaperlessDoc struct {
	Title    string
	FileName string
	Created  string
	Added    string
}

// fakePaperlessController is a real-protocol test double for the Paperless-ngx
// REST API (the controller-package copy — the services package keeps its own;
// both exercise the real HTTP protocol against the same wire shapes).
type fakePaperlessController struct {
	t      *testing.T
	Server *httptest.Server
	// Token, when non-empty, makes the server reject requests without a
	// matching "Authorization: Token <token>" header (401).
	Token string
	Docs  map[int]*fakePaperlessDoc
	// Me is the identity returned by GET /api/auth/me/. Defaults to a fixed
	// test account when unset.
	Me             map[string]any
	FailWithStatus int
	LastToken      string
}

// newPaperlessTestServer builds a started fake Paperless instance.
func newPaperlessTestServer(t *testing.T, token string) *fakePaperlessController {
	f := &fakePaperlessController{
		t:     t,
		Token: token,
		Docs:  make(map[int]*fakePaperlessDoc),
	}
	f.Server = httptest.NewServer(http.HandlerFunc(f.handle))
	return f
}

func (f *fakePaperlessController) Close() {
	f.Server.Close()
}

func (f *fakePaperlessController) URL() string {
	return f.Server.URL
}

func (f *fakePaperlessController) addDoc(id int, title, fileName, created, added string) {
	f.Docs[id] = &fakePaperlessDoc{Title: title, FileName: fileName, Created: created, Added: added}
}

func (f *fakePaperlessController) handle(w http.ResponseWriter, r *http.Request) {
	f.t.Helper()

	if f.FailWithStatus != 0 {
		w.WriteHeader(f.FailWithStatus)
		return
	}

	// The root is reachable without a token (Test Connection stage 1).
	if r.URL.Path == "/api/" {
		writeControllerJSON(w, map[string]any{"version": "2.14.0"})
		return
	}

	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Token ")
	f.LastToken = token
	if f.Token != "" && token != f.Token {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	switch {
	case r.URL.Path == "/api/auth/me/":
		me := f.Me
		if me == nil {
			me = map[string]any{"user_name": "admin", "id": 1}
		}
		writeControllerJSON(w, me)
	case r.URL.Path == "/api/documents/":
		f.handleListDocuments(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/documents/") && strings.HasSuffix(r.URL.Path, "/"):
		idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/documents/"), "/")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		doc, ok := f.Docs[id]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeControllerJSON(w, map[string]any{"id": id, "title": doc.Title, "file_name": doc.FileName, "created": doc.Created, "added": doc.Added})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *fakePaperlessController) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize < 1 {
		pageSize = 100
	}

	// Deterministic order (ascending id) — f.Docs is a map, whose iteration
	// order is random; without an explicit sort the same test can flake.
	ids := make([]int, 0, len(f.Docs))
	for id := range f.Docs {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	results := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		doc := f.Docs[id]
		if query != "" && !strings.Contains(strings.ToLower(doc.Title), strings.ToLower(query)) {
			continue
		}
		results = append(results, map[string]any{
			"id": id, "title": doc.Title, "file_name": doc.FileName,
			"created": doc.Created, "added": doc.Added,
		})
	}

	start := (page - 1) * pageSize
	if start > len(results) {
		start = len(results)
	}
	end := start + pageSize
	if end > len(results) {
		end = len(results)
	}
	writeControllerJSON(w, map[string]any{
		"count":   len(results),
		"next":    fmt.Sprintf("?page=%d", page+1),
		"results": results[start:end],
	})
}
