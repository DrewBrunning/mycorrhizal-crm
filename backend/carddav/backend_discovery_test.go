package carddav

import (
	"context"
	"path/filepath"
	"testing"

	"mycorrhizal/database"

	webdavcarddav "github.com/emersion/go-webdav/carddav"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// newDiscoveryBackend builds a Backend over a REAL migrated schema (CLAUDE.md
// backend trap 1 — QueryAddressObjects persists via PutAddressObject, and
// RecordForContact/ApplyRecordToContact read the relationship_edges/
// contact_tags/preferences/field_values tables, so an AutoMigrate-only
// contacts table would produce the real tables' absence as logged errors).
func newDiscoveryBackend(t *testing.T) (*Backend, *gorm.DB) {
	t.Helper()
	db, err := database.InitDB(filepath.Join(t.TempDir(), "carddav-discovery.db"))
	require.NoError(t, err)
	return NewBackend(db, t.TempDir()), db
}

func TestCurrentUserPrincipal(t *testing.T) {
	backend, _ := newDiscoveryBackend(t)
	ctx := ContextWithUser(context.Background(), 1, "alice", backend.db, backend.photoDir, "")

	principal, err := backend.CurrentUserPrincipal(ctx)
	require.NoError(t, err)
	assert.Equal(t, "/carddav/principals/alice/", principal)

	// No username in context: error.
	_, err = backend.CurrentUserPrincipal(context.Background())
	require.Error(t, err)
}

func TestAddressBookHomeSetPath(t *testing.T) {
	backend, _ := newDiscoveryBackend(t)
	ctx := ContextWithUser(context.Background(), 1, "alice", backend.db, backend.photoDir, "")

	home, err := backend.AddressBookHomeSetPath(ctx)
	require.NoError(t, err)
	assert.Equal(t, "/carddav/addressbooks/alice/", home)

	_, err = backend.AddressBookHomeSetPath(context.Background())
	require.Error(t, err)
}

func TestListAddressBooks(t *testing.T) {
	backend, _ := newDiscoveryBackend(t)
	ctx := ContextWithUser(context.Background(), 1, "alice", backend.db, backend.photoDir, "")

	books, err := backend.ListAddressBooks(ctx)
	require.NoError(t, err)
	require.Len(t, books, 1)
	assert.Equal(t, "/carddav/addressbooks/alice/contacts/", books[0].Path)
	assert.Equal(t, "Contacts", books[0].Name)
	// Both vCard versions are advertised as supported capabilities.
	require.Len(t, books[0].SupportedAddressData, 2)
	assert.Equal(t, "3.0", books[0].SupportedAddressData[0].Version)
	assert.Equal(t, "4.0", books[0].SupportedAddressData[1].Version)

	_, err = backend.ListAddressBooks(context.Background())
	require.Error(t, err)
}

func TestGetAddressBook(t *testing.T) {
	backend, _ := newDiscoveryBackend(t)
	ctx := ContextWithUser(context.Background(), 1, "alice", backend.db, backend.photoDir, "")

	// Exact path works.
	book, err := backend.GetAddressBook(ctx, "/carddav/addressbooks/alice/contacts/")
	require.NoError(t, err)
	require.NotNil(t, book)
	assert.Equal(t, "Contacts", book.Name)

	// Missing trailing slash also works.
	book, err = backend.GetAddressBook(ctx, "/carddav/addressbooks/alice/contacts")
	require.NoError(t, err)
	require.NotNil(t, book)

	// Any other path is not found.
	_, err = backend.GetAddressBook(ctx, "/carddav/addressbooks/alice/other/")
	require.Error(t, err)

	// Another user's path is not found.
	_, err = backend.GetAddressBook(ctx, "/carddav/addressbooks/bob/contacts/")
	require.Error(t, err)

	// No username: error.
	_, err = backend.GetAddressBook(context.Background(), "/carddav/addressbooks/alice/contacts/")
	require.Error(t, err)
}

func TestCreateDeleteAddressBookUnsupported(t *testing.T) {
	backend, _ := newDiscoveryBackend(t)
	ctx := ContextWithUser(context.Background(), 1, "alice", backend.db, backend.photoDir, "")

	require.Error(t, backend.CreateAddressBook(ctx, &webdavcarddav.AddressBook{Path: "/carddav/addressbooks/alice/other/"}))
	require.Error(t, backend.DeleteAddressBook(ctx, "/carddav/addressbooks/alice/other/"))
}

func TestQueryAddressObjects(t *testing.T) {
	backend, db := newDiscoveryBackend(t)
	ctx := ContextWithUser(context.Background(), 1, "alice", db, backend.photoDir, "")

	// Seed two contacts via the address book PUT path.
	for _, c := range []struct{ uid, fn string }{
		{"q-uid-1", "Query One"},
		{"q-uid-2", "Query Two"},
	} {
		_, err := backend.PutAddressObject(ctx, "/carddav/addressbooks/alice/contacts/"+c.uid+".vcf",
			mustParseVCard(t, simpleVCard4(c.uid, c.fn)), nil)
		require.NoError(t, err)
	}

	// No filter: everything comes back. (FilterAllOf with no prop filters is
	// the library's "match all" query — an empty FilterAnyOf matches nothing.)
	all, err := backend.QueryAddressObjects(ctx, "/carddav/addressbooks/alice/contacts/", &webdavcarddav.AddressBookQuery{
		DataRequest: webdavcarddav.AddressDataRequest{AllProp: true},
		FilterTest:  webdavcarddav.FilterAllOf,
	})
	require.NoError(t, err)
	require.Len(t, all, 2)

	// A property filter narrows the set. Filter for FN = "Query One".
	filtered, err := backend.QueryAddressObjects(ctx, "/carddav/addressbooks/alice/contacts/", &webdavcarddav.AddressBookQuery{
		DataRequest: webdavcarddav.AddressDataRequest{AllProp: true},
		PropFilters: []webdavcarddav.PropFilter{{
			Name: "FN",
			TextMatches: []webdavcarddav.TextMatch{
				{Text: "Query One", MatchType: webdavcarddav.MatchEquals},
			},
		}},
	})
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	assert.Equal(t, "Query One", filtered[0].Card.Value("FN"))

	// A filter matching nothing returns nothing.
	none, err := backend.QueryAddressObjects(ctx, "/carddav/addressbooks/alice/contacts/", &webdavcarddav.AddressBookQuery{
		DataRequest: webdavcarddav.AddressDataRequest{AllProp: true},
		PropFilters: []webdavcarddav.PropFilter{{
			Name: "FN",
			TextMatches: []webdavcarddav.TextMatch{
				{Text: "Nope", MatchType: webdavcarddav.MatchEquals},
			},
		}},
	})
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestQueryAddressObjects_Unauthenticated(t *testing.T) {
	backend, _ := newDiscoveryBackend(t)
	_, err := backend.QueryAddressObjects(context.Background(), "/carddav/addressbooks/alice/contacts/", &webdavcarddav.AddressBookQuery{})
	require.Error(t, err)
}
