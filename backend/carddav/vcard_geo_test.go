package carddav

// Issue #1434 — geo: coordinates over CardDAV. The vcard4 adapter emits a
// coordinate as the RFC 9554 ADR GEO parameter and fixes the bytes with a
// post-encode splice, but go-webdav re-encodes the vcard.Card we return, and
// go-vcard mangles a comma/colon parameter value into `GEO=geo:1;GEO=1` — which
// the next parse reads as the rest of the ADR value, corrupting the address.
// The workaround (vcard_geo.go) carries the coordinate as a standalone GEO
// property tagged with the ADR's PROP-ID, which round-trips correctly.
//
// The GET integration test is hand-verified to fail with
// adrGeoParamsToProperties reverted; the PUT test with takeGeoCoordinates /
// applyGeoCoordinates reverted.

import (
	"bytes"
	"context"
	"testing"

	"github.com/emersion/go-vcard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mycorrhizal/contactmodel"
	"mycorrhizal/models"
	"mycorrhizal/vcard4"
)

func TestNormalizeGeoCoordinate(t *testing.T) {
	for _, tc := range []struct {
		in, want string
	}{
		{"geo:48.2,16.3", "geo:48.2,16.3"},
		{`geo:48.2\,16.3`, "geo:48.2,16.3"}, // go-vcard's TEXT-style comma escape
		{"  geo:48.2,16.3  ", "geo:48.2,16.3"},
		{"GEO:1,2", "GEO:1,2"},
		{"", ""},
		{"48.2,16.3", ""},  // no scheme
		{"geo:1", ""},      // missing longitude
		{"geo:95,0", ""},   // latitude out of range (ParseGeoURI rejects)
		{"http://x/y", ""}, // wrong scheme
	} {
		assert.Equal(t, tc.want, normalizeGeoCoordinate(tc.in), "input %q", tc.in)
	}
}

func TestADRGeoParamsToProperties(t *testing.T) {
	card := vcard.Card{}
	card.SetValue(vcard.FieldVersion, "4.0")
	// The decimal split `["geo:48.2", "16.3"]` is exactly what go-vcard's
	// decoder produces from `GEO="geo:48.2,16.3"`.
	card[vcard4.PropAdr] = []*vcard.Field{{
		Value:  ";;1 Main St;;;;",
		Params: vcard.Params{vcard4.ParamGeo: []string{"geo:48.2", "16.3"}, vcard4.ParamPropID: []string{"addr-1"}},
	}}

	adrGeoParamsToProperties(card)

	adr := card[vcard4.PropAdr][0]
	assert.Empty(t, adr.Params[vcard4.ParamGeo], "the unrepresentable ADR GEO parameter is removed")
	require.Len(t, card[vcard4.PropGeo], 1)
	assert.Equal(t, "geo:48.2,16.3", card[vcard4.PropGeo][0].Value, "the split values are rejoined")
	assert.Equal(t, "addr-1", card[vcard4.PropGeo][0].Params.Get(vcard4.ParamPropID), "association is carried on PROP-ID")
}

func TestADRGeoParamsToProperties_UnparseableIsDropped(t *testing.T) {
	card := vcard.Card{}
	card.SetValue(vcard.FieldVersion, "4.0")
	card[vcard4.PropAdr] = []*vcard.Field{{Value: ";;x;;;;", Params: vcard.Params{vcard4.ParamGeo: []string{"not-a-uri"}}}}

	adrGeoParamsToProperties(card)

	assert.Empty(t, card[vcard4.PropAdr][0].Params[vcard4.ParamGeo])
	assert.Empty(t, card[vcard4.PropGeo], "an unparseable coordinate is dropped, not re-emitted")
}

func TestTakeGeoCoordinates(t *testing.T) {
	card := vcard.Card{}
	card.SetValue(vcard.FieldVersion, "4.0")
	card[vcard4.PropGeo] = []*vcard.Field{
		{Value: "geo:1,1", Params: vcard.Params{vcard4.ParamPropID: []string{"addr-1"}}},
		{Value: `geo:2\,2`, Params: vcard.Params{vcard4.ParamPropID: []string{"addr-2"}}},
	}
	card[vcard4.PropAdr] = []*vcard.Field{{
		Value:  ";;3 Third St;;;;",
		Params: vcard.Params{vcard4.ParamGeo: []string{"geo:3", "3"}, vcard4.ParamPropID: []string{"addr-3"}},
	}, {Value: ";;1;;;;", Params: vcard.Params{vcard4.ParamPropID: []string{"addr-1"}}},
		{Value: ";;2;;;;", Params: vcard.Params{vcard4.ParamPropID: []string{"addr-2"}}}}

	byAdr, byID, ordered := takeGeoCoordinates(card)

	assert.Equal(t, map[int]string{0: "geo:3,3"}, byAdr, "an ADR parameter is bound to its own ADR index")
	assert.Equal(t, map[string]string{"addr-1": "geo:1,1", "addr-2": "geo:2,2"}, byID)
	assert.Equal(t, []string{"geo:1,1", "geo:2,2"}, ordered)
	assert.Empty(t, card[vcard4.PropGeo], "the GEO properties are consumed")
	assert.Empty(t, card[vcard4.PropAdr][0].Params[vcard4.ParamGeo], "the ADR GEO parameter is consumed")
}

func TestApplyGeoCoordinates(t *testing.T) {
	record := &contactmodel.Record{Card: contactmodel.Card{Addresses: []contactmodel.Address{
		{ID: "addr-1"}, {ID: "addr-2"},
	}}}

	t.Run("by ID", func(t *testing.T) {
		rec := record
		applyGeoCoordinates(rec, nil, map[string]string{"addr-2": "geo:2,2"}, nil)
		assert.Empty(t, rec.Card.Addresses[0].Coordinates)
		assert.Equal(t, "geo:2,2", rec.Card.Addresses[1].Coordinates)
	})

	t.Run("positionally when no ID matched", func(t *testing.T) {
		rec := &contactmodel.Record{Card: contactmodel.Card{Addresses: []contactmodel.Address{{ID: ""}, {ID: ""}}}}
		applyGeoCoordinates(rec, nil, map[string]string{}, []string{"geo:1,1", "geo:2,2"})
		assert.Equal(t, "geo:1,1", rec.Card.Addresses[0].Coordinates)
		assert.Equal(t, "geo:2,2", rec.Card.Addresses[1].Coordinates)
	})

	t.Run("ADR parameter index wins over ID and position", func(t *testing.T) {
		rec := &contactmodel.Record{Card: contactmodel.Card{Addresses: []contactmodel.Address{{ID: "a"}, {ID: "b"}}}}
		applyGeoCoordinates(rec, map[int]string{1: "geo:5,5"}, map[string]string{"a": "geo:1,1", "b": "geo:9,9"}, nil)
		assert.Equal(t, "geo:1,1", rec.Card.Addresses[0].Coordinates)
		assert.Equal(t, "geo:5,5", rec.Card.Addresses[1].Coordinates)
	})

	t.Run("no coordinates is a no-op", func(t *testing.T) {
		rec := &contactmodel.Record{Card: contactmodel.Card{Addresses: []contactmodel.Address{{ID: "addr-1"}}}}
		applyGeoCoordinates(rec, nil, nil, nil)
		assert.Empty(t, rec.Card.Addresses[0].Coordinates)
	})
}

// --- integration: real CardDAV GET -> (wire) -> PUT --------------------------

// newCoordinateContact seeds a contact (owned by the context user 1) with the
// given addresses.
func newCoordinateContact(t *testing.T, uid string, addrs []contactmodel.Address) (*Backend, models.Contact) {
	t.Helper()
	backend, db := newTestBackend(t)
	record := &contactmodel.Record{Card: contactmodel.Card{
		UID:       uid,
		Name:      &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Ada"}}},
		Addresses: addrs,
	}}
	contact := models.Contact{UserID: 1}
	models.ApplyRecordToContact(&contact, record, "")
	require.NoError(t, db.Create(&contact).Error)
	return backend, contact
}

// wireRoundTrip encodes then decodes a card, exactly the go-vcard round trip a
// CardDAV client performs on the served bytes.
func wireRoundTrip(t *testing.T, card vcard.Card) vcard.Card {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, vcard.NewEncoder(&buf).Encode(card))
	decoded, err := vcard.NewDecoder(bytes.NewReader(buf.Bytes())).Decode()
	require.NoError(t, err)
	return decoded
}

func TestCardDAV_GeoCoordinate_SurvivesGetThenPut(t *testing.T) {
	backend, contact := newCoordinateContact(t, "geo-roundtrip", []contactmodel.Address{{
		ID:          "addr-1",
		Components:  []contactmodel.AddressComponent{{Kind: "name", Value: "1 Main St"}},
		Full:        "1 Main St",
		Coordinates: "geo:48.2,16.3",
	}})
	ctx := ContextWithUser(context.Background(), 1, "tester", backend.db, backend.photoDir, "")
	urlPath := "/carddav/addressbooks/tester/contacts/" + contact.VCardUID + ".vcf"

	obj, err := backend.GetAddressObject(ctx, urlPath, nil)
	require.NoError(t, err)

	// Served as a standalone GEO property; no ADR GEO parameter (the form that
	// corrupts the line).
	require.Len(t, obj.Card["GEO"], 1)
	assert.Equal(t, "geo:48.2,16.3", obj.Card["GEO"][0].Value)
	require.Len(t, obj.Card["ADR"], 1)
	assert.Empty(t, obj.Card["ADR"][0].Params["GEO"], "the ADR must not carry the unrepresentable GEO parameter")

	// Simulate the client wire round trip, then an edit-PUT.
	clientCard := wireRoundTrip(t, obj.Card)
	require.Equal(t, "geo:48.2,16.3", clientCard["GEO"][0].Value, "the coordinate survives go-vcard's encode/decode")
	clientCard.SetValue(vcard.FieldFormattedName, "Ada Edited")
	_, err = backend.PutAddressObject(ctx, urlPath, clientCard, nil)
	require.NoError(t, err)

	var stored models.Contact
	require.NoError(t, backend.db.Where("user_id = ? AND vcard_uid = ?", uint(1), contact.VCardUID).First(&stored).Error)
	require.Len(t, stored.Addresses, 1)
	assert.Equal(t, "1 Main St", stored.Addresses[0].Street, "the address must not be mangled by the GEO round trip")
	assert.Equal(t, "geo:48.2,16.3", stored.Addresses[0].Coordinates, "the coordinate must survive the round trip")
	assert.Equal(t, "Ada Edited", stored.FN, "the client's ordinary edit still applies")
}

// TestCardDAV_GeoCoordinate_VCard30RoundTrip pins that the vCard 3.0 path —
// which already used a standalone GEO property in the `lat;lon` form and never
// hit the parameter bug — is untouched by the 4.0 workaround.
func TestCardDAV_GeoCoordinate_VCard30RoundTrip(t *testing.T) {
	backend, contact := newCoordinateContact(t, "geo-v3", []contactmodel.Address{{
		ID:          "addr-1",
		Components:  []contactmodel.AddressComponent{{Kind: "name", Value: "1 Main St"}},
		Full:        "1 Main St",
		Coordinates: "geo:48.2,16.3",
	}})
	ctx := ContextWithUser(context.Background(), 1, "tester", backend.db, backend.photoDir, "text/vcard; version=3.0")
	urlPath := "/carddav/addressbooks/tester/contacts/" + contact.VCardUID + ".vcf"

	obj, err := backend.GetAddressObject(ctx, urlPath, nil)
	require.NoError(t, err)
	require.Equal(t, "3.0", obj.Card.Value(vcard.FieldVersion))
	require.Len(t, obj.Card["GEO"], 1)
	assert.Equal(t, "48.2;16.3", obj.Card["GEO"][0].Value, "vCard 3.0 GEO is the lat;lon property form")

	_, err = backend.PutAddressObject(ctx, urlPath, wireRoundTrip(t, obj.Card), nil)
	require.NoError(t, err)

	var stored models.Contact
	require.NoError(t, backend.db.Where("user_id = ? AND vcard_uid = ?", uint(1), contact.VCardUID).First(&stored).Error)
	require.Len(t, stored.Addresses, 1)
	assert.Equal(t, "geo:48.2,16.3", stored.Addresses[0].Coordinates)
}

// TestCardDAV_GeoCoordinate_MultiAddressAssociation pins that each coordinate
// stays with its own address across a GET->wire->PUT, via PROP-ID.
func TestCardDAV_GeoCoordinate_MultiAddressAssociation(t *testing.T) {
	backend, contact := newCoordinateContact(t, "geo-multi", []contactmodel.Address{
		{ID: "addr-1", Components: []contactmodel.AddressComponent{{Kind: "name", Value: "1 Main St"}}, Full: "1 Main St", Coordinates: "geo:1,1"},
		{ID: "addr-2", Components: []contactmodel.AddressComponent{{Kind: "name", Value: "2 Side St"}}, Full: "2 Side St", Coordinates: "geo:2,2"},
	})
	ctx := ContextWithUser(context.Background(), 1, "tester", backend.db, backend.photoDir, "")
	urlPath := "/carddav/addressbooks/tester/contacts/" + contact.VCardUID + ".vcf"

	obj, err := backend.GetAddressObject(ctx, urlPath, nil)
	require.NoError(t, err)
	require.Len(t, obj.Card["GEO"], 2)

	_, err = backend.PutAddressObject(ctx, urlPath, wireRoundTrip(t, obj.Card), nil)
	require.NoError(t, err)

	var stored models.Contact
	require.NoError(t, backend.db.Where("user_id = ? AND vcard_uid = ?", uint(1), contact.VCardUID).First(&stored).Error)
	byStreet := map[string]string{}
	for _, a := range stored.Addresses {
		byStreet[a.Street] = a.Coordinates
	}
	assert.Equal(t, "geo:1,1", byStreet["1 Main St"])
	assert.Equal(t, "geo:2,2", byStreet["2 Side St"])
}
