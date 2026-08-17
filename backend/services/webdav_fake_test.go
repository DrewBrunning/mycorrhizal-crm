package services

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
)

// fakeWebDAVItem is the server-side state for one Nextcloud file/folder.
type fakeWebDAVItem struct {
	Name     string
	IsDir    bool
	Size     int64
	Modified string // HTTP date (RFC1123)
	FileID   string
}

// fakeWebDAVServer is a permanent, real-protocol test double for the
// Nextcloud/ownCloud WebDAV surface. It serves PROPFIND (Depth: 1) multistatus
// XML on the dav root, which is the entire surface this integration relies on.
type fakeWebDAVServer struct {
	t *testing.T

	Server      *httptest.Server
	Username    string
	AppPassword string
	// Items keyed by absolute href (e.g. "/remote.php/dav/files/test/").
	Items          map[string]*fakeWebDAVItem
	FailWithStatus int
	LastUser       string
	LastPass       string
}

func newFakeWebDAVServer(t *testing.T, username, appPassword string) *fakeWebDAVServer {
	f := &fakeWebDAVServer{
		t:           t,
		Username:    username,
		AppPassword: appPassword,
		Items:       make(map[string]*fakeWebDAVItem),
	}
	f.Server = httptest.NewServer(http.HandlerFunc(f.handle))
	return f
}

func (f *fakeWebDAVServer) Close() {
	f.Server.Close()
}

func (f *fakeWebDAVServer) URL() string {
	return f.Server.URL
}

func (f *fakeWebDAVServer) handle(w http.ResponseWriter, r *http.Request) {
	f.t.Helper()

	if f.FailWithStatus != 0 {
		w.WriteHeader(f.FailWithStatus)
		return
	}

	if f.Username != "" || f.AppPassword != "" {
		user, pass, ok := r.BasicAuth()
		f.LastUser = user
		f.LastPass = pass
		if !ok || user != f.Username || pass != f.AppPassword {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	}

	if r.Method != "PROPFIND" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusMultiStatus)

	target := strings.TrimRight(r.URL.Path, "/")
	if target == "" {
		target = "/"
	}

	var out strings.Builder
	out.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	out.WriteString(`<d:multistatus xmlns:d="DAV:" xmlns:oc="http://owncloud.org/ns">` + "\n")

	if item, ok := f.Items[target]; ok {
		f.writeResponse(&out, target, item)
	}

	// Children in deterministic href order — f.Items is a map.
	hrefs := make([]string, 0, len(f.Items))
	for href := range f.Items {
		hrefs = append(hrefs, href)
	}
	sort.Strings(hrefs)
	for _, href := range hrefs {
		trimmed := strings.TrimRight(href, "/")
		parent := trimmed
		if idx := strings.LastIndex(parent, "/"); idx >= 0 {
			parent = parent[:idx]
		}
		if parent == target && href != target {
			f.writeResponse(&out, trimmed, f.Items[href])
		}
	}

	out.WriteString(`</d:multistatus>`)
	_, _ = w.Write([]byte(out.String()))
}

func (f *fakeWebDAVServer) writeResponse(out *strings.Builder, href string, item *fakeWebDAVItem) {
	rt := `<d:resourcetype/>`
	if item.IsDir {
		rt = `<d:resourcetype><d:collection/></d:resourcetype>`
	}
	contentLength := ""
	if !item.IsDir {
		contentLength = fmt.Sprintf("<d:getcontentlength>%d</d:getcontentlength>", item.Size)
	}
	modified := ""
	if item.Modified != "" {
		modified = "<d:getlastmodified>" + item.Modified + "</d:getlastmodified>"
	}
	fileID := ""
	if item.FileID != "" {
		fileID = "<oc:fileid>" + item.FileID + "</oc:fileid>"
	}
	fmt.Fprintf(out, `  <d:response>
    <d:href>%s</d:href>
    <d:propstat>
      <d:prop>
        <d:displayname>%s</d:displayname>
        %s
        %s
        %s
        <d:getetag>"abc"</d:getetag>
        %s
      </d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
`, href, item.Name, rt, contentLength, modified, fileID)
}
