package realserver

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"mycorrhizal/i18n"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ntfyMessage is the subset of ntfy's published-message JSON the contract
// depends on (https://docs.ntfy.sh/subscribe/api/#json-message-format).
type ntfyMessage struct {
	Event   string `json:"event"`
	Topic   string `json:"topic"`
	Title   string `json:"title"`
	Message string `json:"message"`
}

// TestNtfy_DeliveredMessageIsReadableFromServer sends a notification through
// our real delivery code (services.TestNotificationChannel -> sendNtfyMessage)
// to a real ntfy server and reads it back from ntfy's own poll API. The fake
// ntfy in services tests only proves we send what we *think* ntfy wants; this
// proves ntfy accepts it and presents the title/message we meant.
func TestNtfy_DeliveredMessageIsReadableFromServer(t *testing.T) {
	base := serverURL(t, envNtfyURL)
	waitReady(t, base+"/v1/health", 60*time.Second)

	db, user := newUser(t)
	topic := fmt.Sprintf("mycorrhizal-contract-%d", time.Now().UnixNano())
	yes := true
	_, err := services.SaveNotificationConfig(db, testCfg().JWTSecretKey, user.ID, models.NotificationConfigInput{
		NtfyURL: base, NtfyTopic: topic, NotifyNtfy: &yes,
	})
	require.NoError(t, err)

	require.NoError(t, services.TestNotificationChannel(db, testCfg(), user, models.ChannelNtfy))

	resp, err := http.Get(base + "/" + topic + "/json?poll=1&since=all") //nolint:gosec,noctx // fixed local test server
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got []ntfyMessage
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		var m ntfyMessage
		require.NoError(t, json.Unmarshal(sc.Bytes(), &m), "ntfy poll line %q", sc.Text())
		if m.Event == "message" {
			got = append(got, m)
		}
	}
	require.Len(t, got, 1, "exactly one message must be delivered to the topic")
	assert.Equal(t, topic, got[0].Topic)
	assert.Equal(t, i18n.T(i18n.DefaultLanguage, "notifications.testTitle"), got[0].Title)
	assert.Equal(t, i18n.T(i18n.DefaultLanguage, "notifications.testBody"), got[0].Message)
}

// TestNtfy_RejectedRequestIsAnError proves the failure half of the contract
// against the real server: ntfy answers a request it cannot accept with a
// non-2xx, and our sender must turn that into an error rather than reporting
// success. A topic name ntfy reserves is the cheapest deterministic rejection.
func TestNtfy_RejectedRequestIsAnError(t *testing.T) {
	base := serverURL(t, envNtfyURL)
	waitReady(t, base+"/v1/health", 60*time.Second)

	db, user := newUser(t)
	_, err := services.SaveNotificationConfig(db, testCfg().JWTSecretKey, user.ID, models.NotificationConfigInput{
		NtfyURL: base, NtfyTopic: "docs",
	})
	require.NoError(t, err)

	err = services.TestNotificationChannel(db, testCfg(), user, models.ChannelNtfy)
	require.Error(t, err, "a non-2xx from ntfy must surface as an error")
	assert.Contains(t, err.Error(), "unexpected status")
}

type gotifyMessage struct {
	ID       int    `json:"id"`
	AppID    int    `json:"appid"`
	Title    string `json:"title"`
	Message  string `json:"message"`
	Priority int    `json:"priority"`
}

type gotifyApp struct {
	ID    int    `json:"id"`
	Token string `json:"token"`
}

func gotifyCreateApp(t *testing.T, base, user, pass string) gotifyApp {
	t.Helper()
	body := strings.NewReader(fmt.Sprintf(`{"name":"mycorrhizal-contract-%d"}`, time.Now().UnixNano()))
	req, err := http.NewRequest(http.MethodPost, base+"/application", body) //nolint:noctx // fixed local test server
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(user, pass)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(raw))
	var app gotifyApp
	require.NoError(t, json.Unmarshal(raw, &app))
	require.NotEmpty(t, app.Token)
	return app
}

// TestGotify_DeliveredMessageIsReadableFromServer sends through our Gotify
// path (X-Gotify-Key header + JSON body) to a real Gotify server and reads the
// message back from that application's message list.
func TestGotify_DeliveredMessageIsReadableFromServer(t *testing.T) {
	base := serverURL(t, envGotifyURL)
	adminUser, adminPass := requireEnv(t, envGotifyUser), requireEnv(t, envGotifyPass)
	waitReady(t, base+"/health", 60*time.Second)
	app := gotifyCreateApp(t, base, adminUser, adminPass)

	db, user := newUser(t)
	yes := true
	_, err := services.SaveNotificationConfig(db, testCfg().JWTSecretKey, user.ID, models.NotificationConfigInput{
		GotifyURL: base, GotifyToken: app.Token, NotifyGotify: &yes,
	})
	require.NoError(t, err)

	require.NoError(t, services.TestNotificationChannel(db, testCfg(), user, models.ChannelGotify))

	var page struct {
		Messages []gotifyMessage `json:"messages"`
	}
	getJSON(t, fmt.Sprintf("%s/application/%d/message", base, app.ID), adminUser, adminPass, &page)
	require.Len(t, page.Messages, 1, "exactly one message must be delivered to the application")
	m := page.Messages[0]
	assert.Equal(t, app.ID, m.AppID)
	assert.Equal(t, i18n.T(i18n.DefaultLanguage, "notifications.testTitle"), m.Title)
	assert.Equal(t, i18n.T(i18n.DefaultLanguage, "notifications.testBody"), m.Message)
	assert.Equal(t, 5, m.Priority)
}

// TestGotify_WrongTokenIsRejected proves the auth half: Gotify answers a bad
// X-Gotify-Key with 401 and our sender must surface it, not swallow it.
func TestGotify_WrongTokenIsRejected(t *testing.T) {
	base := serverURL(t, envGotifyURL)
	waitReady(t, base+"/health", 60*time.Second)

	db, user := newUser(t)
	_, err := services.SaveNotificationConfig(db, testCfg().JWTSecretKey, user.ID, models.NotificationConfigInput{
		GotifyURL: base, GotifyToken: "not-a-real-token",
	})
	require.NoError(t, err)

	err = services.TestNotificationChannel(db, testCfg(), user, models.ChannelGotify)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}
