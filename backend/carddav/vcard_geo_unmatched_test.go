package carddav

// Unmatched GEO must be preserved, not consumed.
//
// RFC 6350 GEO is a contact-level property (cardinality *). takeGeoCoordinates
// used to delete every standalone GEO before the adapter import, and
// applyGeoCoordinates only re-applies those that match an address — so a GEO
// matching no address was silently lost. Before #1434 the adapter kept it in
// Record.Passthrough.VCard (GEO is not a mapped v4 property name).

import (
	"context"
	"testing"

	"github.com/emersion/go-vcard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mycorrhizal/models"
	"mycorrhizal/vcard4"
)

func v4GeoCard(uid string, adrs []*vcard.Field, geos []*vcard.Field) vcard.Card {
	card := vcard.Card{}
	card.SetValue(vcard.FieldVersion, "4.0")
	card.SetValue(vcard.FieldUID, uid)
	card.SetValue(vcard.FieldFormattedName, "Ada")
	if len(adrs) > 0 {
		card[vcard4.PropAdr] = adrs
	}
	if len(geos) > 0 {
		card[vcard4.PropGeo] = geos
	}
	return card
}

func geoValues(card vcard.Card) []string {
	var vals []string
	for _, g := range card[vcard4.PropGeo] {
		vals = append(vals, g.Value)
	}
	return vals
}

func TestCardDAV_PutV4_GeoWithNoAddress_Preserved(t *testing.T) {
	backend, _ := newTestBackend(t)
	ctx := ContextWithUser(context.Background(), 1, "tester", backend.db, backend.photoDir, "")
	uid := "geo-no-adr"
	urlPath := "/carddav/addressbooks/tester/contacts/" + uid + ".vcf"

	_, err := backend.PutAddressObject(ctx, urlPath, v4GeoCard(uid, nil, []*vcard.Field{{Value: "geo:10,20"}}), nil)
	require.NoError(t, err)

	obj, err := backend.GetAddressObject(ctx, urlPath, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"geo:10,20"}, geoValues(obj.Card), "a contact-level GEO with no address must survive")
}

func TestCardDAV_PutV4_UnmatchedPropIDGeo_Preserved(t *testing.T) {
	backend, _ := newTestBackend(t)
	ctx := ContextWithUser(context.Background(), 1, "tester", backend.db, backend.photoDir, "")
	uid := "geo-unmatched"
	urlPath := "/carddav/addressbooks/tester/contacts/" + uid + ".vcf"
	adr := func(id, street string) *vcard.Field {
		return &vcard.Field{Value: ";;" + street + ";;;;", Params: vcard.Params{vcard4.ParamPropID: []string{id}}}
	}
	geo := func(id, v string) *vcard.Field {
		return &vcard.Field{Value: v, Params: vcard.Params{vcard4.ParamPropID: []string{id}}}
	}
	card := v4GeoCard(uid,
		[]*vcard.Field{adr("a1", "1 Main St"), adr("a2", "2 Side St")},
		[]*vcard.Field{geo("a1", "geo:1,1"), geo("a2", "geo:2,2"), geo("nomatch", "geo:9,9")})

	_, err := backend.PutAddressObject(ctx, urlPath, card, nil)
	require.NoError(t, err)

	var stored models.Contact
	require.NoError(t, backend.db.Where("user_id = ? AND vcard_uid = ?", uint(1), uid).First(&stored).Error)
	byStreet := map[string]string{}
	for _, a := range stored.Addresses {
		byStreet[a.Street] = a.Coordinates
	}
	assert.Equal(t, "geo:1,1", byStreet["1 Main St"])
	assert.Equal(t, "geo:2,2", byStreet["2 Side St"])

	obj, err := backend.GetAddressObject(ctx, urlPath, nil)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"geo:1,1", "geo:2,2", "geo:9,9"}, geoValues(obj.Card),
		"the unmatched GEO must survive, and the matched ones must not be duplicated")
}

func TestCardDAV_PutV4_SurplusGeoBeyondAddresses_Preserved(t *testing.T) {
	backend, _ := newTestBackend(t)
	ctx := ContextWithUser(context.Background(), 1, "tester", backend.db, backend.photoDir, "")
	uid := "geo-surplus"
	urlPath := "/carddav/addressbooks/tester/contacts/" + uid + ".vcf"
	card := v4GeoCard(uid,
		[]*vcard.Field{{Value: ";;1 Main St;;;;"}},
		[]*vcard.Field{{Value: "geo:1,1"}, {Value: "geo:2,2"}})

	_, err := backend.PutAddressObject(ctx, urlPath, card, nil)
	require.NoError(t, err)

	obj, err := backend.GetAddressObject(ctx, urlPath, nil)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"geo:1,1", "geo:2,2"}, geoValues(obj.Card),
		"positional fallback attaches geo[0] to address[0]; the surplus GEO must not be dropped")
}
