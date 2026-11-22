package controllers

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func entityNamePage(t *testing.T, e *entityEnv, query string) (int, models.EntityListResponse, string) {
	t.Helper()
	w := e.do(http.MethodGet, "/entities?"+query, nil)
	var page models.EntityListResponse
	_ = json.Unmarshal(w.Body.Bytes(), &page)
	return w.Code, page, w.Body.String()
}

func seedEntities(t *testing.T, e *entityEnv, userID uint, kind string, names ...string) []models.Entity {
	t.Helper()
	out := make([]models.Entity, 0, len(names))
	for _, n := range names {
		row := models.Entity{UserID: userID, Kind: kind, Name: n}
		require.NoError(t, e.db.Create(&row).Error)
		out = append(out, row)
	}
	return out
}

// Issue #2744 (item 2): GET /entities?sort=name orders case-insensitively by
// name (not by last-modified), with an id tiebreak; the default is unchanged.
func TestListEntitiesSortNameIsCaseInsensitiveAndIgnoresLastModified(t *testing.T) {
	e := newEntityEnv(t)
	rows := seedEntities(t, e, e.userID, models.EntityKindOrganisation, "zeta Corp", "Alpha Ltd", "beta Inc", "Äpfel GmbH")
	// "Edit" the alphabetically-first row last so updated_at disagrees.
	require.NoError(t, e.db.Model(&rows[1]).Update("updated_at", time.Now().Add(time.Hour)).Error)

	code, page, body := entityNamePage(t, e, "sort=name&order=asc")
	require.Equal(t, http.StatusOK, code, body)
	var names []string
	for _, r := range page.Entities {
		names = append(names, r.Name)
	}
	assert.Equal(t, []string{"Alpha Ltd", "beta Inc", "zeta Corp", "Äpfel GmbH"}, names,
		"lower- and upper-case names interleave (no diacritic folding, ADR 0016)")

	_, desc, _ := entityNamePage(t, e, "sort=name&order=desc")
	require.Len(t, desc.Entities, 4)
	assert.Equal(t, "Äpfel GmbH", desc.Entities[0].Name)

	// Default order is unchanged: last modified first.
	_, def, _ := entityNamePage(t, e, "")
	require.Len(t, def.Entities, 4)
	assert.Equal(t, "Alpha Ltd", def.Entities[0].Name)
}

// Page boundaries on equal names (the id tiebreak) return every row once, in
// both directions, and the filters still apply under the new order.
func TestListEntitiesSortNamePaginationNoDupesNoGaps(t *testing.T) {
	e := newEntityEnv(t)
	seedEntities(t, e, e.userID, models.EntityKindOrganisation, "Coop", "coop", "COOP", "Bank", "bank", "Zed")
	seedEntities(t, e, e.userID, models.EntityKindPlace, "Cafe")

	for _, order := range []string{"asc", "desc"} {
		seen := map[string]bool{}
		var keys []string
		cursor := ""
		for pages := 0; pages < 10; pages++ {
			q := "sort=name&kind=organisation&limit=2&order=" + order
			if cursor != "" {
				q += "&cursor=" + cursor
			}
			code, page, body := entityNamePage(t, e, q)
			require.Equal(t, http.StatusOK, code, body)
			for _, r := range page.Entities {
				require.False(t, seen[r.ID], "duplicate %s", r.ID)
				seen[r.ID] = true
				keys = append(keys, models.EntityNameKey(r.Name))
			}
			cursor = page.NextCursor
			if cursor == "" {
				break
			}
		}
		require.Len(t, keys, 6, "%s: no gaps", order)
		for i := 1; i < len(keys); i++ {
			if order == "asc" {
				assert.LessOrEqual(t, keys[i-1], keys[i], "asc at %d", i)
			} else {
				assert.GreaterOrEqual(t, keys[i-1], keys[i], "desc at %d", i)
			}
		}
	}
}

func TestListEntitiesSortNameIsOwnerScoped(t *testing.T) {
	e := newEntityEnv(t)
	other := models.User{Username: "nameother", Password: "password123", Email: "nameother@example.com"}
	require.NoError(t, e.db.Create(&other).Error)
	seedEntities(t, e, other.ID, models.EntityKindOrganisation, "AAA Theirs")
	seedEntities(t, e, e.userID, models.EntityKindOrganisation, "Mine")

	_, page, _ := entityNamePage(t, e, "sort=name&order=asc")
	require.Len(t, page.Entities, 1)
	assert.Equal(t, "Mine", page.Entities[0].Name)
}

func TestListEntitiesSortValidation(t *testing.T) {
	e := newEntityEnv(t)
	seedEntities(t, e, e.userID, models.EntityKindOrganisation, "A", "B")

	code, _, body := entityNamePage(t, e, "sort=bogus")
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, body, `"reason_code":"validation_sort_invalid"`)

	code, _, _ = entityNamePage(t, e, "sort=updated_at")
	assert.Equal(t, http.StatusOK, code)

	// A cursor minted under one sort is rejected under the other.
	_, first, _ := entityNamePage(t, e, "sort=name&limit=1")
	require.NotEmpty(t, first.NextCursor)
	code, _, _ = entityNamePage(t, e, "cursor="+first.NextCursor)
	assert.Equal(t, http.StatusBadRequest, code, "name cursor under the default sort")
	_, def, _ := entityNamePage(t, e, "limit=1")
	require.NotEmpty(t, def.NextCursor)
	code, _, _ = entityNamePage(t, e, "sort=name&cursor="+def.NextCursor)
	assert.Equal(t, http.StatusBadRequest, code, "updated_at cursor under sort=name")
}

// A name made only of punctuation has an empty name key, and a name can look
// like a timestamp; neither may break paging under sort=name.
func TestListEntitiesSortNamePagesThroughAwkwardKeys(t *testing.T) {
	e := newEntityEnv(t)
	seedEntities(t, e, e.userID, models.EntityKindOrganisation, "???", "!!!", "2026-01-01T00:00:00Z", "a|b", "Real")

	seen := map[string]bool{}
	cursor := ""
	for pages := 0; pages < 10; pages++ {
		q := "sort=name&order=asc&limit=1"
		if cursor != "" {
			q += "&cursor=" + cursor
		}
		code, page, body := entityNamePage(t, e, q)
		require.Equal(t, http.StatusOK, code, body)
		for _, r := range page.Entities {
			seen[r.ID] = true
		}
		cursor = page.NextCursor
		if cursor == "" {
			break
		}
	}
	assert.Len(t, seen, 5, "every row is reachable by paging")
}
