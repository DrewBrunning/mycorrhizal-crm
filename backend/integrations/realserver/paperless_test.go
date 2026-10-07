package realserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"testing"
	"time"

	"mycorrhizal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	envPaperlessURL  = "MYCORRHIZAL_RS_PAPERLESS_URL"
	envPaperlessUser = "MYCORRHIZAL_RS_PAPERLESS_ADMIN_USER"
	envPaperlessPass = "MYCORRHIZAL_RS_PAPERLESS_ADMIN_PASSWORD"
)

// paperlessToken exchanges the admin credentials for an API token the same way
// a user does in the Paperless UI ("My Profile -> API token").
func paperlessToken(t *testing.T, base, user, pass string) string {
	t.Helper()
	resp, err := http.PostForm(base+"/api/token/", url.Values{"username": {user}, "password": {pass}}) //nolint:noctx // fixed local test server
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(raw))
	var tok struct {
		Token string `json:"token"`
	}
	require.NoError(t, json.Unmarshal(raw, &tok))
	require.NotEmpty(t, tok.Token)
	return tok.Token
}

// paperlessUpload posts a plain-text document through Paperless's own consume
// endpoint and returns once the consumption task has finished and the document
// exists. Seeding uses net/http directly so it does not depend on the code
// under test.
func paperlessUpload(t *testing.T, base, token, title, body string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	require.NoError(t, mw.WriteField("title", title))
	fw, err := mw.CreateFormFile("document", title+".txt")
	require.NoError(t, err)
	_, err = fw.Write([]byte(body))
	require.NoError(t, err)
	require.NoError(t, mw.Close())

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, base+"/api/documents/post_document/", &buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Token "+token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, string(raw))
	var taskID string
	require.NoError(t, json.Unmarshal(raw, &taskID), "post_document returns the task id as a JSON string: %s", raw)

	deadline := time.Now().Add(4 * time.Minute)
	for time.Now().Before(deadline) {
		var tasks []struct {
			Status string `json:"status"`
			Result string `json:"result"`
		}
		treq, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, base+"/api/tasks/?task_id="+url.QueryEscape(taskID), nil)
		treq.Header.Set("Authorization", "Token "+token)
		tresp, err := http.DefaultClient.Do(treq)
		require.NoError(t, err)
		traw, _ := io.ReadAll(tresp.Body)
		_ = tresp.Body.Close()
		if json.Unmarshal(traw, &tasks) == nil && len(tasks) == 1 {
			switch tasks[0].Status {
			case "SUCCESS":
				return
			case "FAILURE":
				t.Fatalf("Paperless failed to consume the seed document: %s", tasks[0].Result)
			}
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("Paperless did not finish consuming %q in time", title)
}

func newPaperlessClient(t *testing.T) (c *services.PaperlessClient, base, token string) {
	t.Helper()
	base = serverURL(t, envPaperlessURL)
	user, pass := requireEnv(t, envPaperlessUser), requireEnv(t, envPaperlessPass)
	waitReady(t, base+"/accounts/login/", 240*time.Second)
	token = paperlessToken(t, base, user, pass)
	c, err := services.NewPaperlessClient(base, token, false)
	require.NoError(t, err)
	return c, base, token
}

// TestPaperless_PickerFlowAgainstRealServer runs the L1 picker/link/test-
// connection calls (Ping, GetMe, ListDocuments with and without a search
// query, GetDocument) against real Paperless-ngx with a real, consumed
// document. The fakes in services tests hand-write the `fields=` projection
// and the response shape; only the real server can say they still exist.
func TestPaperless_PickerFlowAgainstRealServer(t *testing.T) {
	c, base, token := newPaperlessClient(t)

	require.NoError(t, c.Ping(), "Test Connection stage 1")
	me, err := c.GetMe()
	require.NoError(t, err, "Test Connection stage 2 (token resolves to an account)")
	assert.Equal(t, "admin", me.UserName)
	assert.Positive(t, me.ID)

	marker := fmt.Sprintf("zorblax%d", time.Now().UnixNano())
	title := "Contract invoice " + marker
	paperlessUpload(t, base, token, title, "Invoice for contract testing. Unique marker: "+marker+"\n")

	all, err := c.ListDocuments("")
	require.NoError(t, err)
	var docID int
	for _, d := range all {
		if d.Title == title {
			docID = d.ID
			assert.NotEmpty(t, d.FileName, "file_name drives the picker label; the real server must still serve it")
			assert.NotEmpty(t, d.Created, "created")
			assert.NotEmpty(t, d.Added, "added")
		}
	}
	require.NotZero(t, docID, "the consumed document must be listed (got %d documents)", len(all))

	// Paperless's full-text index is updated asynchronously after consumption.
	var found bool
	for i := 0; i < 30 && !found; i++ {
		hits, err := c.ListDocuments(marker)
		require.NoError(t, err)
		for _, d := range hits {
			found = found || d.ID == docID
		}
		if !found {
			time.Sleep(2 * time.Second)
		}
	}
	assert.True(t, found, "a full-text search for the document's unique marker must find it")

	doc, err := c.GetDocument(docID)
	require.NoError(t, err)
	assert.Equal(t, docID, doc.ID)
	assert.Equal(t, title, doc.Title)
}

// TestPaperless_ErrorsMapToSentinels pins the real server's failure statuses:
// a bad token is ErrPaperlessUnauthorized (not a generic failure), and a
// missing document is ErrPaperlessNotFound.
func TestPaperless_ErrorsMapToSentinels(t *testing.T) {
	c, base, _ := newPaperlessClient(t)

	_, err := c.GetDocument(987654321)
	require.ErrorIs(t, err, services.ErrPaperlessNotFound)

	bad, err := services.NewPaperlessClient(base, "0000000000000000000000000000000000000000", false)
	require.NoError(t, err)
	_, err = bad.GetMe()
	require.ErrorIs(t, err, services.ErrPaperlessUnauthorized)
}
