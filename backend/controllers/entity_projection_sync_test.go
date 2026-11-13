package controllers

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"mycorrhizal/middleware"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #2565 (ADR 0043 amendment): link and entity writes that change a
// contact's projected ORG/ADR make the contact visible to the ?since= change
// feed and move its ETag, through the REST routes the apps use.

func newSyncEnv(t *testing.T) *linkEnv {
	t.Helper()
	e := newLinkEnv(t)
	e.router.PUT("/entities/:id", middleware.ValidateJSONMiddleware(&models.EntityInput{}), UpdateEntity)
	return e
}

func (e *linkEnv) stamp(t *testing.T, c models.Contact) (etag string, rev int64) {
	t.Helper()
	var row struct {
		ETag     string `gorm:"column:etag"`
		Revision int64
	}
	require.NoError(t, e.db.Table("contacts").Select("etag", "revision").Where("id = ?", c.ID).Scan(&row).Error)
	return row.ETag, row.Revision
}

// feedIDs returns the contact ids the ?since= feed lists after cursor.
func (e *linkEnv) feedIDs(t *testing.T, cursor string) map[uint]bool {
	t.Helper()
	w := e.do(http.MethodGet, "/contacts?since="+cursor, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body struct {
		Contacts []struct {
			ID uint `json:"id"`
		} `json:"contacts"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	ids := map[uint]bool{}
	for _, c := range body.Contacts {
		ids[c.ID] = true
	}
	return ids
}

// settle makes every contact old and returns a feed cursor after that moment.
func (e *linkEnv) settle(t *testing.T) string {
	t.Helper()
	old := time.Now().Add(-48 * time.Hour)
	require.NoError(t, e.db.Table("contacts").Where("user_id = ?", e.userID).UpdateColumn("updated_at", old).Error)
	return EncodeCursor(old.Add(time.Hour), 0)
}

func TestEntityProjectionSync_LinkCreateRenameDeleteReachTheFeedAndETag(t *testing.T) {
	e := newSyncEnv(t)
	c := e.contact(t, nil)
	bystander := e.contact(t, nil)
	nonPrimary := e.contact(t, nil)
	org := e.org(t, "Acme")

	// 1. Adding a primary organisation link.
	cursor := e.settle(t)
	etag0, rev0 := e.stamp(t, c)
	e.addLink(t, nonPrimary, map[string]any{"entity_id": org.ID, "role": "employee", "is_primary": false})
	link := e.addLink(t, c, map[string]any{"entity_id": org.ID, "role": "employee", "is_primary": true})
	feed := e.feedIDs(t, cursor)
	assert.True(t, feed[c.ID], "link create: the contact appears in the since feed")
	assert.False(t, feed[nonPrimary.ID], "a non-primary link projects nothing")
	assert.False(t, feed[bystander.ID], "an unrelated contact is not listed")
	etag1, rev1 := e.stamp(t, c)
	assert.NotEqual(t, etag0, etag1)
	assert.Equal(t, rev0+1, rev1)

	// 2. Renaming the entity (PUT with the entity's current revision).
	cursor = e.settle(t)
	w := e.do(http.MethodPut, "/entities/"+org.ID, map[string]any{"name": "Acme Holdings",
		"core": map[string]any{"organization": map[string]any{"name": "Acme Holdings"}}}, "If-Match", "1")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	feed = e.feedIDs(t, cursor)
	assert.True(t, feed[c.ID], "entity rename: the linked contact appears in the feed")
	assert.False(t, feed[nonPrimary.ID])
	assert.False(t, feed[bystander.ID])
	etag2, _ := e.stamp(t, c)
	assert.NotEqual(t, etag1, etag2)

	// A save that changes nothing the card shows leaves the contact alone.
	cursor = e.settle(t)
	w = e.do(http.MethodPut, "/entities/"+org.ID, map[string]any{"name": "Acme Holdings",
		"core": map[string]any{"organization": map[string]any{"name": "Acme Holdings"}, "notes": "private note"}}, "If-Match", "2")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Empty(t, e.feedIDs(t, cursor), "a notes-only edit changes no card")
	etag2b, _ := e.stamp(t, c)
	assert.Equal(t, etag2, etag2b)

	// 3. Deleting the link.
	cursor = e.settle(t)
	w = e.do(http.MethodDelete, linkPath(c, link.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.True(t, e.feedIDs(t, cursor)[c.ID], "link delete: the contact appears in the feed")
	etag3, _ := e.stamp(t, c)
	assert.NotEqual(t, etag2, etag3)

	// 4. Deleting the entity while a primary link projects it.
	e.addLink(t, c, map[string]any{"entity_id": org.ID, "role": "employee", "is_primary": true})
	cursor = e.settle(t)
	etag4, _ := e.stamp(t, c)
	w = e.do(http.MethodDelete, "/entities/"+org.ID, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	feed = e.feedIDs(t, cursor)
	assert.True(t, feed[c.ID], "entity delete: the contact appears in the feed")
	assert.False(t, feed[bystander.ID])
	etag5, _ := e.stamp(t, c)
	assert.NotEqual(t, etag4, etag5)
}
