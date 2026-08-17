package controllers

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
)

// fakeSeafileLibrary is the server-side state for one Seafile library.
type fakeSeafileLibrary struct {
	Name string
}

// fakeSeafileItem is the server-side state for one file/dir entry in a
// library folder listing.
type fakeSeafileItem struct {
	Name  string
	Type  string // file | dir
	Size  int64
	MTime int64
}

// fakeSeafileController is a real-protocol test double for the Seafile Web
// API (the controller-package copy — the services package keeps its own; both
// exercise the real HTTP protocol against the same wire shapes).
type fakeSeafileController struct {
	t      *testing.T
	Server *httptest.Server
	// Token, when non-empty, makes authenticated endpoints reject requests
	// without a matching "Authorization: Token <token>" header (401).
	Token string
	// Libs are the libraries served by /api2/repos/, keyed by repo id.
	Libs map[string]*fakeSeafileLibrary
	// Dir are the folder listings served by /api2/repos/:id/dir/, keyed by
	// repo id + ":" + dir path.
	Dir            map[string][]*fakeSeafileItem
	FailWithStatus int
	LastToken      string
}

// newSeafileTestServer builds a started fake Seafile instance.
func newSeafileTestServer(t *testing.T, token string) *fakeSeafileController {
	f := &fakeSeafileController{
		t:     t,
		Token: token,
		Libs:  make(map[string]*fakeSeafileLibrary),
		Dir:   make(map[string][]*fakeSeafileItem),
	}
	f.Server = httptest.NewServer(http.HandlerFunc(f.handle))
	return f
}

func (f *fakeSeafileController) Close() {
	f.Server.Close()
}

func (f *fakeSeafileController) URL() string {
	return f.Server.URL
}

func (f *fakeSeafileController) addLib(id, name string) {
	f.Libs[id] = &fakeSeafileLibrary{Name: name}
}

func (f *fakeSeafileController) addDirItem(repoID, dir, name, itemType string, size, mtime int64) {
	f.Dir[repoID+":"+dir] = append(f.Dir[repoID+":"+dir], &fakeSeafileItem{Name: name, Type: itemType, Size: size, MTime: mtime})
}

func (f *fakeSeafileController) handle(w http.ResponseWriter, r *http.Request) {
	f.t.Helper()

	if f.FailWithStatus != 0 {
		w.WriteHeader(f.FailWithStatus)
		return
	}

	// /api2/ping/ is unauthenticated (Test Connection stage 1).
	if r.URL.Path == "/api2/ping/" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`"pong"`))
		return
	}

	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Token ")
	f.LastToken = token
	if f.Token != "" && token != f.Token {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	switch {
	case r.URL.Path == "/api2/auth/ping/":
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`"pong"`))
	case r.URL.Path == "/api2/repos/":
		// Deterministic order (ascending repo id) — f.Libs is a map.
		ids := make([]string, 0, len(f.Libs))
		for id := range f.Libs {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		libs := make([]map[string]any, 0, len(ids))
		for _, id := range ids {
			libs = append(libs, map[string]any{"id": id, "name": f.Libs[id].Name, "type": "library"})
		}
		writeControllerJSON(w, libs)
	case strings.HasPrefix(r.URL.Path, "/api2/repos/") && strings.HasSuffix(r.URL.Path, "/dir/"):
		repoID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api2/repos/"), "/dir/")
		dir := r.URL.Query().Get("p")
		items := f.Dir[repoID+":"+dir]
		if items == nil {
			writeControllerJSON(w, []any{})
			return
		}
		out := make([]map[string]any, 0, len(items))
		for _, it := range items {
			out = append(out, map[string]any{
				"id": it.Name, "name": it.Name, "type": it.Type,
				"size": it.Size, "mtime": it.MTime, "parent_dir": dir,
			})
		}
		writeControllerJSON(w, out)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}
