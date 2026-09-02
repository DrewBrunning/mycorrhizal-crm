package services

// DB-03 (issue #494) — INV-A6: every sync operation eventually converges.
// Generative form: repeated CardDAV reconciliation of an unchanged remote
// reaches a fixed point (no writes, stable revision tokens), and a remote
// change applied twice is idempotent — never left half-merged.
//
// Driven directly against reconcileContactSync (the standalone,
// server-free reconciliation entry point) so the property does not need an
// HTTP CardDAV harness. Lives in package services because that function is
// unexported.

import (
	"fmt"
	"strings"
	"testing"

	"mycorrhizal/internal/contactgen"
	"mycorrhizal/models"

	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav/carddav"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"pgregory.net/rapid"
)

// syncCard builds a parsed vCard 4.0 the way a go-webdav response carries one.
func syncCard(t *rapid.T, uid, given, email string) vcard.Card {
	raw := fmt.Sprintf("BEGIN:VCARD\r\nVERSION:4.0\r\nUID:%s\r\nFN:%s\r\nN:%s;;;;\r\nEMAIL:%s\r\nEND:VCARD\r\n",
		uid, given, given, email)
	card, err := vcard.NewDecoder(strings.NewReader(raw)).Decode()
	require.NoError(t, err)
	return card
}

// syncStateSnapshot is the observable post-reconcile state: every contact's
// identity + revision + tombstone flag, and every sync link's content hash and
// etag. Two byte-identical snapshots mean the reconcile made no change.
func syncStateSnapshot(t *rapid.T, db *gorm.DB) string {
	var b strings.Builder

	var contacts []map[string]any
	require.NoError(t, db.Raw(
		`SELECT vcard_uid, firstname, email, revision, (deleted_at IS NOT NULL) AS deleted
		 FROM contacts ORDER BY vcard_uid`).Scan(&contacts).Error)
	for _, c := range contacts {
		fmt.Fprintf(&b, "C %v|%v|%v|%v|%v\n", c["vcard_uid"], c["firstname"], c["email"], c["revision"], c["deleted"])
	}

	var links []map[string]any
	require.NoError(t, db.Raw(
		`SELECT href, content_hash, etag, contact_id FROM contact_sync_links ORDER BY href`).Scan(&links).Error)
	for _, l := range links {
		fmt.Fprintf(&b, "L %v|%v|%v|%v\n", l["href"], l["content_hash"], l["etag"], l["contact_id"])
	}
	return b.String()
}

func TestDataInvariant_A6_ContactSyncConvergesToAFixpoint(t *testing.T) {
	t.Run("converges", rapid.MakeCheck(func(t *rapid.T) {
		db, _, err := contactgen.MigratedDB(t)
		require.NoError(t, err)
		user, err := contactgen.NewUser(db, "a6-sync")
		require.NoError(t, err)

		sub := models.ContactSubscription{UserID: user.ID, Name: "book", URL: "https://dav.example/book", SyncEnabled: true}
		require.NoError(t, db.Create(&sub).Error)

		m := drawInt(t, "remote_objects", 1, 6)
		book := make([]carddav.AddressObject, 0, m)
		for i := 0; i < m; i++ {
			uid := fmt.Sprintf("uid-%d", i)
			book = append(book, carddav.AddressObject{
				Path: fmt.Sprintf("/dav/%d.vcf", i),
				ETag: fmt.Sprintf("\"e-%d-1\"", i),
				Card: syncCard(t, uid, fmt.Sprintf("Given%d", i), fmt.Sprintf("user%d@example.com", i)),
			})
		}

		// Round 1: full apply.
		s1, err := reconcileContactSync(db, &sub, book, nil, true, "")
		require.NoError(t, err)
		require.Equal(t, m, s1.Created)
		snapAfter1 := syncStateSnapshot(t, db)

		// Round 2: the same unchanged remote must be a no-op — the fixed point.
		s2, err := reconcileContactSync(db, &sub, book, nil, true, "")
		require.NoError(t, err)
		require.Zero(t, s2.Created, "an unchanged remote must create nothing on re-sync")
		require.Zero(t, s2.Updated, "an unchanged remote must update nothing on re-sync")
		require.Zero(t, s2.Archived, "an unchanged remote must archive nothing on re-sync")
		require.Equal(t, snapAfter1, syncStateSnapshot(t, db),
			"re-syncing an unchanged remote must not change any row (revision tokens included)")

		// A remote change: bump one object's email + etag. Applied once, then
		// again unchanged — the second application must converge to the same
		// state, never a half-merge.
		k := drawInt(t, "changed_index", 0, m-1)
		changed := carddav.AddressObject{
			Path: fmt.Sprintf("/dav/%d.vcf", k),
			ETag: fmt.Sprintf("\"e-%d-2\"", k),
			Card: syncCard(t, fmt.Sprintf("uid-%d", k), fmt.Sprintf("Given%d", k), fmt.Sprintf("user%d+new@example.com", k)),
		}

		s3, err := reconcileContactSync(db, &sub, []carddav.AddressObject{changed}, nil, false, "")
		require.NoError(t, err)
		require.Equal(t, 1, s3.Updated, "the changed object must apply as an update")
		snapAfter3 := syncStateSnapshot(t, db)

		s4, err := reconcileContactSync(db, &sub, []carddav.AddressObject{changed}, nil, false, "")
		require.NoError(t, err)
		require.Zero(t, s4.Updated, "re-applying the same remote change must be a no-op")
		require.Equal(t, snapAfter3, syncStateSnapshot(t, db),
			"re-applying the same remote change must converge to an identical state")

	}))
}
