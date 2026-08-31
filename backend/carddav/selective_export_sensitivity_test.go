package carddav

// DATA-04 (issue #444) — CardDAV as an export surface.
//
// External sync is the higher-risk path for sensitive data to escape: it does
// not go through the export controller, has no `include_sensitive` query
// parameter, and serves contact cards over a protocol whose whole point is
// pushing the data to another party's device. The rule to pin: CardDAV has NO
// opt-in at all — contactToAddressObject goes through models.RecordForContact
// (the nil-selection default), so the sensitivity filter is applied in the
// projection queries, and a private/secret edge, hobby preference, or
// vcard-projected custom field must never leave through this surface.
//
// This is also the structural "exclusion in the projection, not per-caller"
// proof: if the filter were moved into the export handlers (the per-caller
// design the ticket warns about), CardDAV — which has no selection plumbing to
// re-add it — would silently leak.

import (
	"context"
	"strings"
	"testing"

	"mycorrhizal/internal/canonicalfixture"
	"mycorrhizal/internal/dbtest"

	"github.com/emersion/go-vcard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCardDAV_NeverServesSensitiveContactData drives the real CardDAV
// GetAddressObject path over the TEST-02 canonical fixture (which carries
// private/secret edges, hobby preferences, and vcard-projected custom fields
// by construction) and asserts none of them reach the served vCard — while the
// normal controls still do, proving the filtering is selective rather than a
// wholesale blank-out.
func TestCardDAV_NeverServesSensitiveContactData(t *testing.T) {
	db := dbtest.New(t)
	m, err := canonicalfixture.Read()
	require.NoError(t, err)
	ds, err := canonicalfixture.Populate(db, m)
	require.NoError(t, err)

	backend := NewBackend(db, t.TempDir())
	// No Accept header: the default vCard 4.0 version is served.
	ctx := ContextWithUser(context.Background(), ds.User.ID, "tester", db, "", "")

	ada := ds.Contacts["ada"]
	hugo := ds.Contacts["hugo"]
	ida := ds.Contacts["ida"]
	bob := ds.Contacts["bob"]

	adaObj, err := backend.GetAddressObject(ctx, "/carddav/addressbooks/tester/contacts/"+ada.VCardUID+".vcf", nil)
	require.NoError(t, err)
	hugoObj, err := backend.GetAddressObject(ctx, "/carddav/addressbooks/tester/contacts/"+hugo.VCardUID+".vcf", nil)
	require.NoError(t, err)
	idaObj, err := backend.GetAddressObject(ctx, "/carddav/addressbooks/tester/contacts/"+ida.VCardUID+".vcf", nil)
	require.NoError(t, err)
	bobObj, err := backend.GetAddressObject(ctx, "/carddav/addressbooks/tester/contacts/"+bob.VCardUID+".vcf", nil)
	require.NoError(t, err)

	adaCard, hugoCard, idaCard, bobCard := adaObj.Card, hugoObj.Card, idaObj.Card, bobObj.Card

	// --- Sensitive surfaces: excluded, with no opt-in available. ---
	// ada's secret vcard-projected custom field (X-PRIVATE-NICK).
	assertNoCardProp(t, adaCard, "X-PRIVATE-NICK",
		"ada's secret custom field must not reach a CardDAV GET")
	// hugo<->ida's private spouse edge.
	assertNoRelation(t, hugoCard, "urn:uuid:"+ida.VCardUID,
		"hugo's private spouse edge must not reach a CardDAV GET")
	assertNoRelation(t, idaCard, "urn:uuid:"+hugo.VCardUID,
		"ida's private spouse edge must not reach a CardDAV GET")
	// bob's private hobby preference (PersonalInfo).
	assertNoHobby(t, bobCard, "chess",
		"bob's private hobby must not reach a CardDAV GET")

	// --- Normal controls: the same surfaces, at normal sensitivity, DO. ---
	assertRelation(t, adaCard, "urn:uuid:"+bob.VCardUID, "co-worker",
		"ada's normal coworker edge must reach a CardDAV GET")
	assertCardProp(t, adaCard, "X-FAVORITE-COFFEE",
		"ada's normal custom field must reach a CardDAV GET")
	assertHobby(t, adaCard, "sailing",
		"ada's normal hobby must reach a CardDAV GET")

	// Identity still resolves: the served card is ada's, not a blank shell.
	assert.Equal(t, ada.VCardUID, adaCard.Value(vcard.FieldUID))
	assert.NotEmpty(t, adaCard.Value(vcard.FieldFormattedName))
}

// TestCardDAV_AllSectionsPathStillFilters pins the foot-gun guard on the
// CardDAV surface: even a "give me everything" request — which on CardDAV is
// simply the default, since there is no section picker — filters sensitivity
// in the projection. This is the "opt-in cannot be implied" rule applied to a
// surface that has no opt-in plumbing at all.
func TestCardDAV_AllSectionsPathStillFilters(t *testing.T) {
	db := dbtest.New(t)
	m, err := canonicalfixture.Read()
	require.NoError(t, err)
	ds, err := canonicalfixture.Populate(db, m)
	require.NoError(t, err)

	backend := NewBackend(db, t.TempDir())
	ctx := ContextWithUser(context.Background(), ds.User.ID, "tester", db, "", "")

	ada := ds.Contacts["ada"]
	bob := ds.Contacts["bob"]

	// The CardDAV surface has no ?sections= param and no ?include_sensitive=
	// param — the request cannot express an opt-in. The full default must
	// therefore be the filtered default.
	obj, err := backend.GetAddressObject(ctx, "/carddav/addressbooks/tester/contacts/"+ada.VCardUID+".vcf", nil)
	require.NoError(t, err)

	assertNoCardProp(t, obj.Card, "X-PRIVATE-NICK",
		"a CardDAV request cannot opt into the secret custom field")
	assertRelation(t, obj.Card, "urn:uuid:"+bob.VCardUID, "co-worker",
		"the normal coworker edge still serves")
}

// --- tiny vCard assertion helpers -----------------------------------------

func assertNoCardProp(t *testing.T, card vcard.Card, prop, msg string) {
	t.Helper()
	assert.Empty(t, card[strings.ToUpper(prop)], "%s (%s)", msg, prop)
}

func assertCardProp(t *testing.T, card vcard.Card, prop, msg string) {
	t.Helper()
	assert.NotEmpty(t, card[strings.ToUpper(prop)], "%s (%s)", msg, prop)
}

func assertNoRelation(t *testing.T, card vcard.Card, target, msg string) {
	t.Helper()
	for _, f := range card["RELATED"] {
		if f.Value == target {
			t.Errorf("%s: RELATED to %s must not be served", msg, target)
		}
	}
}

func assertRelation(t *testing.T, card vcard.Card, target, typeTag, msg string) {
	t.Helper()
	for _, f := range card["RELATED"] {
		if f.Value == target && (typeTag == "" || f.Params.HasType(typeTag)) {
			return
		}
	}
	t.Errorf("%s: expected RELATED %s type=%s to be served", msg, target, typeTag)
}

func assertNoHobby(t *testing.T, card vcard.Card, value, msg string) {
	t.Helper()
	for _, f := range card["HOBBY"] {
		if f.Value == value {
			t.Errorf("%s: HOBBY %q must not be served", msg, value)
		}
	}
}

func assertHobby(t *testing.T, card vcard.Card, value, msg string) {
	t.Helper()
	for _, f := range card["HOBBY"] {
		if f.Value == value {
			return
		}
	}
	t.Errorf("%s: expected HOBBY %q to be served", msg, value)
}
