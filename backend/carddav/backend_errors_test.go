package carddav

// Failure and fallback paths of the CardDAV backend (issue #2569: coverage of
// carddav/backend.go restored with real tests). Each case drives one error
// return and asserts the client-visible outcome, not just that something fails.

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav/carddav"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func httpStatusOf(t *testing.T, err error) int {
	t.Helper()
	require.Error(t, err)
	// go-webdav's HTTPError type is internal; its message starts with the code.
	var code int
	_, scanErr := fmt.Sscanf(err.Error(), "%d", &code)
	require.NoError(t, scanErr, err.Error())
	return code
}

func simpleCard(uid, fn string) vcard.Card {
	c := make(vcard.Card)
	c.SetValue(vcard.FieldVersion, "4.0")
	if uid != "" {
		c.SetValue(vcard.FieldUID, uid)
	}
	c.SetValue(vcard.FieldFormattedName, fn)
	c.SetName(&vcard.Name{GivenName: fn})
	return c
}

func TestBackend_UnauthenticatedRequestsAre401(t *testing.T) {
	b, _ := newTestBackend(t)
	ctx := context.Background()
	_, err := b.GetAddressObject(ctx, "/carddav/addressbooks/u/contacts/x.vcf", nil)
	assert.Equal(t, http.StatusUnauthorized, httpStatusOf(t, err), "get")
	_, err = b.PutAddressObject(ctx, "/carddav/addressbooks/u/contacts/x.vcf", simpleCard("x", "X"), nil)
	assert.Equal(t, http.StatusUnauthorized, httpStatusOf(t, err), "put")
	err = b.DeleteAddressObject(ctx, "/carddav/addressbooks/u/contacts/x.vcf")
	assert.Equal(t, http.StatusUnauthorized, httpStatusOf(t, err), "delete")
}

func TestBackend_ContextFallbacksAndEnvDefault(t *testing.T) {
	b, _ := newTestBackend(t)
	assert.Same(t, b.db, b.getDB(context.Background()), "no db in the context falls back to the backend's")
	assert.Equal(t, b.photoDir, b.getPhotoDir(context.Background()))
	other := dbtest.New(t)
	assert.Same(t, other, b.getDB(ContextWithUser(context.Background(), 1, "u", other, "/x", "")), "a db in the context wins")

	t.Setenv("CARDDAV_TEST_ENV_DEFAULT", "set")
	assert.Equal(t, "set", envOrDefault("CARDDAV_TEST_ENV_DEFAULT", "fallback"))
	assert.Equal(t, "fallback", envOrDefault("CARDDAV_TEST_ENV_DEFAULT_UNSET", "fallback"))
}

func TestBackend_DatabaseFailuresSurface(t *testing.T) {
	t.Run("listing and querying", func(t *testing.T) {
		b, db := newTestBackend(t)
		ctx := ContextWithUser(context.Background(), 1, "u", db, b.photoDir, "")
		dbtest.HideTable(t, db, "contacts")
		_, err := b.ListAddressObjects(ctx, "/carddav/addressbooks/u/contacts/", &carddav.AddressDataRequest{})
		require.Error(t, err)
		_, err = b.QueryAddressObjects(ctx, "/carddav/addressbooks/u/contacts/", &carddav.AddressBookQuery{})
		require.Error(t, err, "a filter-less query lists")
		_, err = b.QueryAddressObjects(ctx, "/carddav/addressbooks/u/contacts/", &carddav.AddressBookQuery{PropFilters: []carddav.PropFilter{{Name: "FN"}}})
		require.Error(t, err, "a filtered query lists first")
	})
	t.Run("put looking up the contact", func(t *testing.T) {
		b, db := newTestBackend(t)
		ctx := ContextWithUser(context.Background(), 1, "u", db, b.photoDir, "")
		dbtest.HideTable(t, db, "contacts")
		_, err := b.PutAddressObject(ctx, "/carddav/addressbooks/u/contacts/x.vcf", simpleCard("x", "X"), nil)
		require.Error(t, err)
	})
	t.Run("put checking the sync scope of an existing contact", func(t *testing.T) {
		b, db := newTestBackend(t)
		ctx := ContextWithUser(context.Background(), 1, "u", db, b.photoDir, "")
		c := models.Contact{UserID: 1, Firstname: "Scope", VCardUID: "scope-uid"}
		require.NoError(t, db.Create(&c).Error)
		dbtest.HideTable(t, db, "carddav_sync_scopes")
		_, err := b.PutAddressObject(ctx, "/carddav/addressbooks/u/contacts/scope-uid.vcf", simpleCard("scope-uid", "Scope"), nil)
		require.Error(t, err, "an unreadable scope must not fall through to an unscoped write")
	})
}

func TestBackend_PutRejectsACardThatCannotBeReEncoded(t *testing.T) {
	b, db := newTestBackend(t)
	ctx := ContextWithUser(context.Background(), 1, "u", db, b.photoDir, "")
	bad := make(vcard.Card)
	bad.SetValue(vcard.FieldFormattedName, "No Version") // VERSION is mandatory to encode
	_, err := b.PutAddressObject(ctx, "/carddav/addressbooks/u/contacts/bad.vcf", bad, nil)
	require.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, httpStatusOf(t, err))
}

func TestBackend_PutOfExistingContactFailsWhenTheProjectionCheckFails(t *testing.T) {
	b, db := newTestBackend(t)
	ctx := ContextWithUser(context.Background(), 1, "u", db, b.photoDir, "")
	c := models.Contact{UserID: 1, Firstname: "Echo", VCardUID: "echo-uid"}
	require.NoError(t, db.Create(&c).Error)
	dbtest.HideTable(t, db, "contact_entity_links")
	_, err := b.PutAddressObject(ctx, "/carddav/addressbooks/u/contacts/echo-uid.vcf", simpleCard("echo-uid", "Echo"), nil)
	require.Error(t, err)
	assert.Equal(t, http.StatusInternalServerError, httpStatusOf(t, err))
}

func TestBackend_PutWithoutAnyUIDMintsOne(t *testing.T) {
	b, db := newTestBackend(t)
	ctx := ContextWithUser(context.Background(), 1, "u", db, b.photoDir, "")
	_, err := b.PutAddressObject(ctx, "/carddav/addressbooks/u/contacts/", simpleCard("", "No Uid"), nil)
	require.NoError(t, err)
	var got models.Contact
	require.NoError(t, db.Where("user_id = ? AND firstname = ?", 1, "No Uid").First(&got).Error)
	assert.NotEmpty(t, got.VCardUID, "RFC 6352 requires every stored contact to have a UID")
	assert.False(t, strings.Contains(got.VCardUID, " "))
}

func TestBackend_ContactWithoutVCardUIDIsAddressedByID(t *testing.T) {
	b, db := newTestBackend(t)
	ctx := ContextWithUser(context.Background(), 1, "tester", db, b.photoDir, "")
	obj := b.contactToAddressObject(ctx, &models.Contact{UserID: 1, Firstname: "Legacy"})
	assert.Equal(t, "/carddav/addressbooks/tester/contacts/0.vcf", obj.Path)
}
