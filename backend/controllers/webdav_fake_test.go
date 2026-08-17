package controllers

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

// fakeWebDAVController is a real-protocol test double for the Nextcloud/
// ownCloud WebDAV surface. It serves PROPFIND (Depth: 1) multistatus XML on
// the dav root, which is the entire surface this integration relies on.
type fakeWebDAVController struct {
	t      *testing.T
	Server *httptest.Server
	// Username/AppPassword, when non-empty, make the server require HTTP Basic
	// matching both.
	Username    string
	AppPassword string
	// Items keyed by absolute href (e.g. "/remote.php/dav/files/test/").
	Items          map[string]*fakeWebDAVItem
	FailWithStatus int
	LastUser       string
	LastPass       string
	LastDepth      string
}

// newWebDAVTestServer builds a started fake Nextcloud instance.
func newWebDAVTestServer(t *testing.T, username, appPassword string) *fakeWebDAVController {
	f := &fakeWebDAVController{
		t:           t,
		Username:    username,
		AppPassword: appPassword,
		Items:       make(map[string]*fakeWebDAVItem),
	}
	f.Server = httptest.NewServer(http.HandlerFunc(f.handle))
	return f
}

func (f *fakeWebDAVController) Close() {
	f.Server.Close()
}

func (f *fakeWebDAVController) URL() string {
	return f.Server.URL
}

func (f *fakeWebDAVController) handle(w http.ResponseWriter, r *http.Request) {
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
	f.LastDepth = r.Header.Get("Depth")
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusMultiStatus)

	target := r.URL.Path
	target = strings.TrimRight(target, "/")
	if target == "" {
		target = "/"
	}

	var out strings.Builder
	out.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	out.WriteString(`<d:multistatus xmlns:d="DAV:" xmlns:oc="http://owncloud.org/ns">` + "\n")

	// The directory itself first (Depth:1 returns the target plus children).
	if item, ok := f.Items[strings.TrimRight(target, "/")]; ok {
		f.writeResponse(&out, strings.TrimRight(target, "/"), item)
	} else if item, ok := f.Items[target]; ok {
		f.writeResponse(&out, target, item)
	}

	// Children (immediate descendants) in deterministic href order — f.Items is
	// a map, whose iteration order is random.
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
		if parent == strings.TrimRight(target, "/") && href != strings.TrimRight(target, "/") && href != target {
			f.writeResponse(&out, trimmed, f.Items[href])
		}
	}

	out.WriteString(`</d:multistatus>`)
	_, _ = w.Write([]byte(out.String()))
}

func (f *fakeWebDAVController) writeResponse(out *strings.Builder, href string, item *fakeWebDAVItem) {
	rt := ""
	if item.IsDir {
		rt = `<d:resourcetype><d:collection/></d:resourcetype>`
	} else {
		rt = `<d:resourcetype/>`
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
