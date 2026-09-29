package embedded

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/require"
)

func startForRevokeTest(t *testing.T, cfg *config.Config, sockName string) (*Server, *http.Client) {
	t.Helper()
	ln, socket := listenUnix(t, sockName)
	srv, err := Start(context.Background(), cfg, Options{Listener: ln, CatchUpDelay: time.Hour})
	require.NoError(t, err)
	return srv, unixHTTPClient(socket)
}

func statusWithToken(t *testing.T, client *http.Client, token string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "http://unix/api/v1/contacts", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	return resp.StatusCode
}

func assertOneLiveSession(t *testing.T, srv *Server) {
	t.Helper()
	var live int64
	require.NoError(t, srv.DB().Model(&models.Session{}).
		Where("user_id = ? AND revoked_at IS NULL", srv.LocalUserID()).Count(&live).Error)
	require.EqualValues(t, 1, live)
}

// Issue #1340: each Start revokes the previous starts' sessions, so exactly
// one live session (and one valid token) exists at a time.
func TestEmbeddedStart_RevokesPreviousSessions(t *testing.T) {
	hc := hostConfigForTest(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(hc.DBPath), 0o700))
	cfg, err := hc.Config()
	require.NoError(t, err)

	srv, client := startForRevokeTest(t, cfg, "r1.sock")
	first := srv.LocalSessionToken()
	require.Equal(t, http.StatusOK, statusWithToken(t, client, first))
	assertOneLiveSession(t, srv)
	require.NoError(t, srv.Stop(context.Background()))

	srv, client = startForRevokeTest(t, cfg, "r2.sock")
	second := srv.LocalSessionToken()
	require.NotEqual(t, first, second)
	require.Equal(t, http.StatusUnauthorized, statusWithToken(t, client, first))
	require.Equal(t, http.StatusOK, statusWithToken(t, client, second))
	assertOneLiveSession(t, srv)

	// Repeated restarts never accumulate live sessions.
	tokens := []string{first, second}
	for i := 0; i < 5; i++ {
		require.NoError(t, srv.Stop(context.Background()))
		srv, client = startForRevokeTest(t, cfg, fmt.Sprintf("rr%d.sock", i))
		tokens = append(tokens, srv.LocalSessionToken())
		assertOneLiveSession(t, srv)
	}
	defer func() { require.NoError(t, srv.Stop(context.Background())) }()
	for _, old := range tokens[:len(tokens)-1] {
		require.Equal(t, http.StatusUnauthorized, statusWithToken(t, client, old))
	}
	require.Equal(t, http.StatusOK, statusWithToken(t, client, tokens[len(tokens)-1]))
}

// A failing revoke must abort provisioning rather than mint a second live
// session alongside the ones it could not revoke.
func TestEmbeddedProvision_RevokeFailureAborts(t *testing.T) {
	hc := hostConfigForTest(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(hc.DBPath), 0o700))
	cfg, err := hc.Config()
	require.NoError(t, err)
	srv, _ := startForRevokeTest(t, cfg, "rf.sock")
	defer func() { require.NoError(t, srv.Stop(context.Background())) }()

	dbtest.HideTable(t, srv.DB(), "sessions")
	err = srv.provisionLocalUser()
	require.ErrorContains(t, err, "revoke previous local sessions")
}
