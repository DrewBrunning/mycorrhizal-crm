package realserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"mycorrhizal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	envSeafileURL   = "MYCORRHIZAL_RS_SEAFILE_URL"
	envSeafileEmail = "MYCORRHIZAL_RS_SEAFILE_ADMIN_EMAIL"
	envSeafilePass  = "MYCORRHIZAL_RS_SEAFILE_ADMIN_PASSWORD"
)

// seafileForm issues one form-encoded Seafile Web API write with the token,
// independent of the code under test, and returns the response body.
func seafileForm(t *testing.T, base, token, path string, form url.Values) []byte {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, base+path, strings.NewReader(form.Encode()))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Token "+token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	require.Less(t, resp.StatusCode, 300, "POST %s: %s", path, raw)
	return raw
}

type seafileFixture struct {
	client  *services.SeafileClient
	base    string
	repoID  string
	library string
}

func seedSeafile(t *testing.T) *seafileFixture {
	t.Helper()
	base := serverURL(t, envSeafileURL)
	email, pass := requireEnv(t, envSeafileEmail), requireEnv(t, envSeafilePass)
	waitReady(t, base+"/api2/ping/", 300*time.Second)

	var tok struct {
		Token string `json:"token"`
	}
	require.NoError(t, json.Unmarshal(seafileForm(t, base, "", "/api2/auth-token/", url.Values{"username": {email}, "password": {pass}}), &tok))
	require.NotEmpty(t, tok.Token)

	library := fmt.Sprintf("Contract Library %d", time.Now().UnixNano())
	var repo struct {
		RepoID string `json:"repo_id"`
	}
	require.NoError(t, json.Unmarshal(seafileForm(t, base, tok.Token, "/api2/repos/", url.Values{"name": {library}}), &repo))
	require.NotEmpty(t, repo.RepoID)

	seafileForm(t, base, tok.Token, "/api2/repos/"+repo.RepoID+"/dir/?p="+url.QueryEscape("/Docs"), url.Values{"operation": {"mkdir"}})
	seafileForm(t, base, tok.Token, "/api2/repos/"+repo.RepoID+"/file/?p="+url.QueryEscape("/Docs/hello.txt"), url.Values{"operation": {"create"}})

	c, err := services.NewSeafileClient(base, tok.Token, false)
	require.NoError(t, err)
	return &seafileFixture{client: c, base: base, repoID: repo.RepoID, library: library}
}

// TestSeafile_ConnectionAndBrowseAgainstRealServer runs Test Connection
// (Ping + PingAuth) and the library/folder picker (ListLibraries, ListDir)
// against a real Seafile server with a real library, folder and file.
func TestSeafile_ConnectionAndBrowseAgainstRealServer(t *testing.T) {
	f := seedSeafile(t)

	require.NoError(t, f.client.Ping(), "Test Connection stage 1 (unauthenticated)")
	require.NoError(t, f.client.PingAuth(), "Test Connection stage 2 (token)")

	libs, err := f.client.ListLibraries()
	require.NoError(t, err)
	var lib *services.SeafileLibrary
	for i := range libs {
		if libs[i].ID == f.repoID {
			lib = &libs[i]
		}
	}
	require.NotNil(t, lib, "the seeded library must be listed (got %d)", len(libs))
	assert.Equal(t, f.library, lib.Name)

	root, err := f.client.ListDir(f.repoID, "/")
	require.NoError(t, err)
	require.Len(t, root, 1)
	assert.Equal(t, "Docs", root[0].Name)
	assert.Equal(t, "dir", root[0].Type)

	docs, err := f.client.ListDir(f.repoID, "/Docs")
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "hello.txt", docs[0].Name)
	assert.Equal(t, "file", docs[0].Type)
	// parent_dir is only served by recursive listings (we request recursive=0),
	// and nothing consumes it: deliberately not asserted.
	assert.Positive(t, docs[0].MTime, "mtime is Unix seconds")
	assert.NotEmpty(t, docs[0].ID)
}

// TestSeafile_ErrorsMapToSentinels pins the real server's failure statuses: a
// bad token is ErrSeafileUnauthorized, an unknown library ErrSeafileNotFound.
func TestSeafile_ErrorsMapToSentinels(t *testing.T) {
	f := seedSeafile(t)

	_, err := f.client.ListDir("00000000-0000-4000-8000-000000000000", "/")
	require.ErrorIs(t, err, services.ErrSeafileNotFound)

	bad, err := services.NewSeafileClient(f.base, "0000000000000000000000000000000000000000", false)
	require.NoError(t, err)
	require.ErrorIs(t, bad.PingAuth(), services.ErrSeafileUnauthorized)
}
