package services

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fcmTestEndpoints points the FCM token + send endpoints at the given handlers
// for the duration of the test.
func fcmTestEndpoints(t *testing.T, token, send http.HandlerFunc) {
	t.Helper()
	tokenSrv := httptest.NewServer(token)
	t.Cleanup(tokenSrv.Close)
	sendSrv := httptest.NewServer(send)
	t.Cleanup(sendSrv.Close)
	origToken, origSend := fcmTokenEndpoint, fcmSendEndpoint
	fcmTokenEndpoint, fcmSendEndpoint = tokenSrv.URL, sendSrv.URL
	t.Cleanup(func() { fcmTokenEndpoint, fcmSendEndpoint = origToken, origSend })
}

func okToken(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"access_token":"tok"}`))
}

func TestFCMAccessToken_PrivateKeyErrors(t *testing.T) {
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	ecDER, err := x509.MarshalPKCS8PrivateKey(ecKey)
	require.NoError(t, err)

	cases := []struct {
		name, key, wantErr string
	}{
		{"not PEM", "this is not a pem block", "failed to decode FCM service account private key"},
		{"PEM but not PKCS8", string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("garbage")})), "failed to parse FCM service account private key"},
		{"PKCS8 but not RSA", string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: ecDER})), "FCM service account private key is not an RSA key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sa := newFCMServiceAccount(t)
			sa.PrivateKey = tc.key
			token, err := fcmAccessToken(config.Config{}, sa)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.Empty(t, token)
		})
	}
}

func TestFCMAccessToken_TokenEndpointFailures(t *testing.T) {
	sa := newFCMServiceAccount(t)
	cases := []struct {
		name    string
		handler http.HandlerFunc
		wantErr string
	}{
		{"non-200", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(" invalid_grant "))
		}, "unexpected status 401 from Google token endpoint: invalid_grant"},
		{"undecodable body", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("not json"))
		}, "failed to decode Google token response"},
		{"empty access token", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"access_token":""}`))
		}, "Google token response contained no access_token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fcmTestEndpoints(t, tc.handler, func(http.ResponseWriter, *http.Request) {})
			token, err := fcmAccessToken(config.Config{}, sa)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.Empty(t, token)
		})
	}
}

func TestSendFCMMessage_Failures(t *testing.T) {
	sa := newFCMServiceAccount(t)

	t.Run("send endpoint returns 500", func(t *testing.T) {
		fcmTestEndpoints(t, okToken, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(" backend down "))
		})
		stale, err := sendFCMMessage(config.Config{}, sa, "device", "T", "B", nil)
		require.EqualError(t, err, "unexpected status 500 from FCM: backend down")
		assert.False(t, stale, "a 5xx is a delivery error, not a stale registration")
	})

	t.Run("token mint failure aborts before sending", func(t *testing.T) {
		sent := false
		fcmTestEndpoints(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}, func(http.ResponseWriter, *http.Request) { sent = true })
		stale, err := sendFCMMessage(config.Config{}, sa, "device", "T", "B", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected status 403 from Google token endpoint")
		assert.False(t, stale)
		assert.False(t, sent, "no message may be sent without an access token")
	})

	t.Run("send transport failure", func(t *testing.T) {
		fcmTestEndpoints(t, okToken, func(http.ResponseWriter, *http.Request) {})
		fcmSendEndpoint = "http://127.0.0.1:1" // nothing listens; restored by the helper's cleanup
		stale, err := sendFCMMessage(config.Config{}, sa, "device", "T", "B", nil)
		require.Error(t, err)
		assert.False(t, stale)
	})
}
