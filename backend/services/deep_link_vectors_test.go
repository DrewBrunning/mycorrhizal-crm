package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// deepLinkVectors mirrors testdata/deep-links/vectors.json (ADR 0029 §5,
// issue #1267). It is the one hand-authored table the Android parser
// (NotificationDeepLinkRouteTest), the web service-worker resolver and this
// backend conformance test all consume, so a route added to one client without
// a vector — or a vector one client fails — breaks that client's build.
type deepLinkVectors struct {
	Comment []string         `json:"_comment"`
	Vectors []deepLinkVector `json:"vectors"`
}

type deepLinkVector struct {
	URI          string  `json:"uri"`
	AndroidRoute *string `json:"android_route"`
	WebPath      *string `json:"web_path"`
	Why          string  `json:"why"`
}

// accepted reports whether at least one client resolves the URI. A vector is
// "rejected" (by both clients) exactly when both renderings are null; a client
// may legitimately degrade to a nearest parent rather than reject, which is
// still a non-null rendering (see the vector's `why`).
func (v deepLinkVector) accepted() bool {
	return v.AndroidRoute != nil || v.WebPath != nil
}

func loadDeepLinkVectors(t *testing.T) deepLinkVectors {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "deep-links", "vectors.json"))
	require.NoError(t, err, "read shared deep-link vectors")
	var table deepLinkVectors
	require.NoError(t, json.Unmarshal(raw, &table), "parse shared deep-link vectors")
	require.NotEmpty(t, table.Vectors, "shared deep-link vectors are empty")
	return table
}

// TestDeepLinkVectorsStructure sanity-checks the shared table itself, so a
// malformed vector fails the backend suite rather than silently weakening the
// emitter check below — for that check alone, an accidentally all-null entry is
// indistinguishable from a deliberate rejection.
func TestDeepLinkVectorsStructure(t *testing.T) {
	t.Parallel()
	table := loadDeepLinkVectors(t)

	seen := make(map[string]bool, len(table.Vectors))
	accepted := 0
	for i, v := range table.Vectors {
		require.NotEmptyf(t, v.URI, "vector %d has no uri", i)
		require.NotEmptyf(t, v.Why, "vector %q has no why (every vector must say what it pins)", v.URI)
		require.Falsef(t, seen[v.URI], "duplicate vector uri %q", v.URI)
		seen[v.URI] = true
		if v.accepted() {
			accepted++
		}
	}
	require.NotZero(t, accepted, "the table must contain accepted vectors, not only rejections")
}

// TestDeepLinkVectorsBackendEmitter pins every `mycorrhizal://` navigation URI
// the backend emits against the shared table: the emitter must use a URI the
// clients accept, so the backend cannot start sending a link the clients reject.
// contactDeepLink is the single format string (fcmReminderData calls it), so
// these are "every URI the backend emits" today; a new emitter belongs in this
// test.
func TestDeepLinkVectorsBackendEmitter(t *testing.T) {
	t.Parallel()
	table := loadDeepLinkVectors(t)

	accepted := make(map[string]deepLinkVector, len(table.Vectors))
	for _, v := range table.Vectors {
		if v.accepted() {
			accepted[v.URI] = v
		}
	}

	// The reminder push's `data.deep_link` is the live emitter (issues #152,
	// #679). Cover the id bounds so the emitter's grammar cannot drift past the
	// vectors' coverage edge.
	for _, id := range []uint{1, 42, 2147483647} {
		contactID := id
		data := fcmReminderData(models.Reminder{ContactID: &contactID})
		link := data["deep_link"]

		_, ok := accepted[link]
		assert.Truef(t, ok, "fcmReminderData deep_link %q (contact %d) is not an accepted vector", link, id)
		assert.Equalf(t, contactDeepLink(id), link, "fcmReminderData must emit the link through contactDeepLink")
	}
}
