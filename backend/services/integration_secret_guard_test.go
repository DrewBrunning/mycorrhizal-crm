package services

import (
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An update that leaves the secret empty ("keep the stored one") while moving
// the base URL to a different origin must be refused and change nothing; a
// path-only change on the same origin keeps the secret; a fresh secret makes
// the move legal. Shared by Immich, Paperless, Seafile and Nextcloud.
func TestUpsertIntegrationConfig_OriginChangeRequiresSecret(t *testing.T) {
	const secret = "secret-guard-test-filler-secret-guard-test-filler"
	db := dbtest.New(t)
	user := models.User{Username: "secret-guard", Password: "password123!A", Email: "secret-guard@example.com"}
	require.NoError(t, db.Create(&user).Error)

	type upsert func(baseURL, secretValue string) (storedURL, storedSecretEnc string, err error)
	cases := []struct {
		name     string
		sentinel error
		upsert   upsert
	}{
		{"immich", ErrImmichSecretRequired, func(u, s string) (string, string, error) {
			c, err := UpsertImmichConfig(db, secret, user.ID, models.ImmichConfigInput{BaseURL: u, APIKey: s})
			if err != nil {
				return "", "", err
			}
			return c.BaseURL, c.APIKeyEncrypted, nil
		}},
		{"paperless", ErrPaperlessSecretRequired, func(u, s string) (string, string, error) {
			c, err := UpsertPaperlessConfig(db, secret, user.ID, models.PaperlessConfigInput{BaseURL: u, APIToken: s})
			if err != nil {
				return "", "", err
			}
			return c.BaseURL, c.APITokenEncrypted, nil
		}},
		{"seafile", ErrSeafileSecretRequired, func(u, s string) (string, string, error) {
			c, err := UpsertSeafileConfig(db, secret, user.ID, models.SeafileConfigInput{BaseURL: u, APIToken: s})
			if err != nil {
				return "", "", err
			}
			return c.BaseURL, c.APITokenEncrypted, nil
		}},
		{"nextcloud", ErrWebDAVSecretRequired, func(u, s string) (string, string, error) {
			c, err := UpsertWebDAVConfig(db, secret, user.ID, models.WebDAVConfigInput{BaseURL: u, Username: "alice", AppPassword: s})
			if err != nil {
				return "", "", err
			}
			return c.BaseURL, c.AppPasswordEncrypted, nil
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			url, enc, err := tc.upsert("https://svc.example", "first-secret")
			require.NoError(t, err)
			assert.Equal(t, "https://svc.example", url)

			// Same origin, new path: secret kept.
			url, enc2, err := tc.upsert("https://SVC.example:443/sub", "")
			require.NoError(t, err)
			assert.Equal(t, "https://SVC.example:443/sub", url)
			assert.Equal(t, enc, enc2)

			// Different origin with no secret: refused, nothing changes.
			for _, other := range []string{"https://svc2.example", "http://svc.example", "https://svc.example:8443", "https://svc.example.evil.test"} {
				_, _, err = tc.upsert(other, "")
				assert.ErrorIs(t, err, tc.sentinel, other)
			}
			url, enc3, err := tc.upsert("https://SVC.example:443/sub", "")
			require.NoError(t, err)
			assert.Equal(t, "https://SVC.example:443/sub", url)
			assert.Equal(t, enc, enc3)

			// Fresh secret: legal.
			url, enc4, err := tc.upsert("https://svc2.example", "second-secret")
			require.NoError(t, err)
			assert.Equal(t, "https://svc2.example", url)
			assert.NotEqual(t, enc, enc4)
			plain, err := DecryptCredential(secret, enc4)
			require.NoError(t, err)
			assert.Equal(t, "second-secret", plain)
		})
	}
}
