package caldav

// DATA-04 (issue #444) — CalDAV as an export surface.
//
// CalDAV serves calendar events only: every Activity and every month/day
// LifeEvent, per ListCalendarObjects. It never serializes a contact card, so
// none of the sensitivity-bearing contact surfaces (RelationshipEdge,
// Preference, FieldValue) can reach it — there is no opt-in because there is
// no contact data to opt into. The standing documented position lives at
// backend.go:234-238: Activities and LifeEvents carry no sensitivity field
// today, and a future one must be filtered here (the calendar leaves the
// instance). This test pins the boundary against a fixture that carries
// private/secret data by construction.

import (
	"context"
	"strings"
	"testing"

	"mycorrhizal/internal/canonicalfixture"
	"mycorrhizal/internal/dbtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCalDAV_NeverCarriesContactSensitivityData drives the real
// ListCalendarObjects path over the TEST-02 canonical fixture and asserts that
// the served ics payloads contain none of the fixture's sensitive contact
// markers — nor any contact-card data at all.
func TestCalDAV_NeverCarriesContactSensitivityData(t *testing.T) {
	db := dbtest.New(t)
	m, err := canonicalfixture.Read()
	require.NoError(t, err)
	ds, err := canonicalfixture.Populate(db, m)
	require.NoError(t, err)

	b := NewBackend(db)
	ctx := ContextWithUser(context.Background(), ds.User.ID, ds.User.Username, db)

	objects, err := b.ListCalendarObjects(ctx, "/caldav/calendars/"+ds.User.Username+"/interactions/", nil)
	require.NoError(t, err)

	var raw strings.Builder
	for _, obj := range objects {
		require.NotNil(t, obj.Data)
		for _, event := range obj.Data.Events() {
			for _, props := range event.Props {
				for _, p := range props {
					raw.WriteString(p.Value)
				}
			}
		}
	}
	body := raw.String()

	// Sensitive contact markers the fixture carries by construction:
	//   - ada's secret vcard-projected custom field value,
	//   - bob's private hobby preference value,
	//   - hugo<->ida's private spouse relationship.
	for _, marker := range []string{
		"stormy", "X-PRIVATE-NICK",
		"chess",
		"spouse",
		"X-FAVORITE-COFFEE", // normal control: even normal contact data has no CalDAV home
	} {
		assert.NotContains(t, body, marker,
			"CalDAV payload must never carry contact-card data (%q)", marker)
	}

	// The fixture's life events and activities ARE served — the calendar is
	// not empty, the assertion above is selective rather than vacuous.
	require.NotEmpty(t, objects, "the fixture's activities/life events must be served")
}
