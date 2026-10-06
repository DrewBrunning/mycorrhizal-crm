package realserver

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"mycorrhizal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Nextcloud WebDAV (the file picker behind services/webdav_client.go). Env is
// provisioned by .github/scripts/carddav-reference/provision-nextcloud.sh.
const (
	envNextcloudURL  = "MYCORRHIZAL_RS_NEXTCLOUD_URL"
	envNextcloudUser = "MYCORRHIZAL_RS_NEXTCLOUD_USER"
	envNextcloudPass = "MYCORRHIZAL_RS_NEXTCLOUD_PASSWORD"
)

// davSeed issues one WebDAV write (MKCOL/PUT) against the dav root as the test
// user. Seeding goes through plain net/http on purpose: it must not depend on
// the code under test.
func davSeed(t *testing.T, base, user, pass, method, relPath, body string) {
	t.Helper()
	escaped := (&url.URL{Path: relPath}).EscapedPath()
	req, err := http.NewRequestWithContext(context.Background(), method, base+"/remote.php/dav/files/"+user+escaped, strings.NewReader(body))
	require.NoError(t, err)
	req.SetBasicAuth(user, pass)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Less(t, resp.StatusCode, 300, "%s %s", method, relPath)
}

func newNextcloudClient(t *testing.T) (c *services.WebDAVClient, base, user, pass string) {
	t.Helper()
	base = serverURL(t, envNextcloudURL)
	user = requireEnv(t, envNextcloudUser)
	pass = requireEnv(t, envNextcloudPass)
	waitReady(t, base+"/status.php", 120*time.Second)
	c, err := services.NewWebDAVClient(base, user, pass, false)
	require.NoError(t, err)
	return c, base, user, pass
}

// TestNextcloudWebDAV_BrowseListsRealFilesAndFolders drives the picker's two
// calls — Ping (Test Connection) and ListDir — against a real Nextcloud: the
// PROPFIND body we send must elicit the displayname / getcontentlength /
// getlastmodified / resourcetype / oc:fileid properties we parse, a folder must
// come back as a dir, and a non-ASCII + space filename must round-trip through
// href percent-encoding to the decoded path we store as external_id.
func TestNextcloudWebDAV_BrowseListsRealFilesAndFolders(t *testing.T) {
	c, base, user, pass := newNextcloudClient(t)
	require.NoError(t, c.Ping(), "Test Connection against a real Nextcloud")

	folder := "Contract " + time.Now().Format("150405.000")
	davSeed(t, base, user, pass, "MKCOL", "/"+folder, "")
	davSeed(t, base, user, pass, "PUT", "/"+folder+"/Résumé & notes.txt", "hello nextcloud")
	davSeed(t, base, user, pass, "MKCOL", "/"+folder+"/Sub", "")

	root, err := c.ListDir("/")
	require.NoError(t, err)
	var found *services.WebDAVItem
	for i := range root {
		if root[i].Name == folder {
			found = &root[i]
		}
	}
	require.NotNil(t, found, "seeded folder must appear in the root listing (got %d items)", len(root))
	assert.Equal(t, "dir", found.Type)
	assert.Equal(t, "/"+folder+"/", found.Path)
	assert.NotEmpty(t, found.FileID, "Nextcloud exposes oc:fileid; the deep link depends on it")

	items, err := c.ListDir("/" + folder)
	require.NoError(t, err)
	require.Len(t, items, 2, "the folder's own entry must be excluded, leaving the file and Sub")
	byName := map[string]services.WebDAVItem{}
	for _, it := range items {
		byName[it.Name] = it
	}
	f, ok := byName["Résumé & notes.txt"]
	require.True(t, ok, "non-ASCII/space filename must decode: %+v", items)
	assert.Equal(t, "file", f.Type)
	assert.Equal(t, "/"+folder+"/Résumé & notes.txt", f.Path)
	assert.Equal(t, int64(len("hello nextcloud")), f.Size)
	assert.NotEmpty(t, f.FileID)
	_, err = time.Parse(time.RFC3339, f.ModifiedAt)
	assert.NoError(t, err, "last-modified must normalise to RFC3339, got %q", f.ModifiedAt)
	assert.Equal(t, "dir", byName["Sub"].Type)
}

// TestNextcloudWebDAV_ErrorsMapToSentinels pins the failure half against the
// real server's actual status codes: bad credentials -> ErrWebDAVUnauthorized
// (not a generic request failure), a missing folder -> ErrWebDAVNotFound.
func TestNextcloudWebDAV_ErrorsMapToSentinels(t *testing.T) {
	c, base, user, _ := newNextcloudClient(t)

	_, err := c.ListDir("/definitely-not-a-real-folder-" + time.Now().Format("150405"))
	require.ErrorIs(t, err, services.ErrWebDAVNotFound)

	bad, err := services.NewWebDAVClient(base, user, "wrong-app-password", false)
	require.NoError(t, err)
	require.ErrorIs(t, bad.Ping(), services.ErrWebDAVUnauthorized)
}
