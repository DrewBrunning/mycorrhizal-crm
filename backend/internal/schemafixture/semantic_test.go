package schemafixture

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"

	"mycorrhizal/contactmodel"
	"mycorrhizal/database"
	"mycorrhizal/internal/canonicalfixture"
	"mycorrhizal/internal/semanticequal"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// MIG-03 (issue #438) — semantic migration validation.
//
// MIG-02 (#437, upgrade_test.go in this package) proves a migration RAN: the
// matrix of release fixtures migrates and every table keeps its row counts. A
// migration that renames a column and forgets the backfill passes every one of
// those checks while silently emptying a field — the `etag`/`e_tag` class of
// CLAUDE.md backend trap #1, and the `RecordFromContact` class of trap #3.
//
// This suite proves the migration PRESERVED MEANING:
//
//   - the default is "nothing changes": every cell of every fixture table is
//     compared before and after the migration at the column granularity, and
//     per-contact canonical Records are compared under TEST-03's semantic-
//     equivalence oracle (internal/semanticequal), the sanctioned notion of
//     "the same contact";
//   - a migration that intentionally transforms data declares exactly that in
//     NNNNNN_name.expect.yaml next to its SQL (see migration_expectation_test.go),
//     and the suite asserts the declared changeset equals the actual one in
//     both directions so the file cannot rot;
//   - soft-deleted rows are read Unscoped, so a migration that hard-deletes
//     what should survive (or a count that cannot distinguish "gone" from
//     "marked") fails here;
//   - the TEST-07 migrate(migrate(db)) non-destructiveness property
//     (internal/propertytest) is wired in there, not duplicated here.
//
// Every milestone-named criterion is covered with real data (assertCriteriaPopulated),
// so the checks are never vacuous 0==0 comparisons.
func TestMigrationsPreserveSemanticContent(t *testing.T) {
	latest, err := database.LatestMigrationVersion()
	require.NoError(t, err)
	expectDir := committedMigrationsDir(t)

	for _, from := range SupportedReleases {
		from := from
		t.Run(from.Tag, func(t *testing.T) {
			f := Load(t, from)
			seedMigrationScopeData(t, f)
			assertCriteriaPopulated(t, f)

			before := captureContentSnapshot(t, f.DB)
			beforeContacts := contactRecords(t, f.DB, f.Dataset)

			afterDB := migrateFixtureTo(t, f, latest)
			f.DB = afterDB

			after := captureContentSnapshot(t, f.DB)
			afterContacts := contactRecords(t, f.DB, f.Dataset)

			declared := loadExpectationFiles(t, expectDir, from.Version, latest)
			if len(declared) == 0 {
				// Default: nothing changed. The raw content comparison is
				// strict (catches renames, drops, reorders, hard deletes); the
				// semantic oracle is the issue's mandated comparison and names
				// the concept that was lost (trap #3's flat-column class).
				assertContentIdentical(t, from.Tag, before, after)
				assertContactsSemanticallyEqual(t, from.Tag, beforeContacts, afterContacts)
			} else {
				// A declared transform: the strict comparison runs against the
				// before state with every declared change applied, so both the
				// migration's behavior AND the file's honesty are asserted.
				assertContentMatchesExpectations(t, from.Tag, before, after, declared)
			}
		})
	}
}

// seedMigrationScopeData writes the two milestone criteria the canonical
// manifest does not cover — audit history and CardDAV sync state (the
// `etag`/`e_tag` trap) — into the fixture so the migration chain actually
// carries them. The manifest is a contact dataset; these are the system- and
// sync-generated rows MIG-03 exists to protect, and a fixture without them
// would make "audit history preserved" and "etag preserved" vacuous.
//
// Rows are inserted with exactly the columns the release schema has (raw SQL,
// no GORM — a historical schema may predate model fields), so migration
// 000034's audit_events rebuild and 000039's sync-health backfill run against
// real rows.
func seedMigrationScopeData(t *testing.T, f *Fixture) {
	t.Helper()
	userID := f.Dataset.User.ID
	ada := f.Dataset.Contacts["ada"]
	bob := f.Dataset.Contacts["bob"]
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

	// Audit history. hash/prev_hash exist only from migration 000034
	// (v0.6.1+); a v0.6.0 fixture carries the pre-hash-chain shape, which is
	// exactly the state 000034 must carry through its table rebuild.
	auditCols := []string{"created_at", "updated_at", "entity_type", "entity_id", "operation", "user_id", "before_snapshot"}
	auditVals := func() []any { return []any{now, now} }
	if f.Release.Version >= 34 {
		auditCols = append(auditCols, "hash", "prev_hash")
	}
	auditQ := "INSERT INTO audit_events (" + strings.Join(auditCols, ", ") + ") VALUES (" + strings.TrimSuffix(strings.Repeat("?, ", len(auditCols)), ", ") + ")"

	create := append(auditVals(), "contact", ada.VCardUID, "create", userID, nil)
	if f.Release.Version >= 34 {
		create = append(create, "", "")
	}
	require.NoError(t, f.DB.Exec(auditQ, create...).Error, "seeding an audit create event")

	update := append(auditVals(), "contact", bob.VCardUID, "update", userID, `{"firstname":"Bob"}`)
	if f.Release.Version >= 34 {
		update = append(update, "", "")
	}
	require.NoError(t, f.DB.Exec(auditQ, update...).Error, "seeding an audit update event")

	// CardDAV sync state: one subscription and two sync links carrying
	// non-empty etags — the exact columns a rename without a backfill would
	// empty (CLAUDE.md backend trap #1's `etag`/`e_tag` shape).
	require.NoError(t, f.DB.Exec(`
		INSERT INTO contact_subscriptions (created_at, updated_at, user_id, name, url, username, sync_enabled, sync_token)
		VALUES (?, ?, ?, 'CardDAV Fixture', 'https://example.com/carddav', 'fixture-user', 1, '')`,
		now, now, userID).Error, "seeding a contact subscription")

	var subscriptionID uint
	require.NoError(t, f.DB.Raw("SELECT id FROM contact_subscriptions WHERE user_id = ?", userID).Scan(&subscriptionID).Error)

	links := []struct {
		href        string
		contactID   uint
		etag        string
		contentHash string
	}{
		{"/dav/ada.vcf", ada.ID, `W/"fixture-etag-1"`, "sha256:ada"},
		{"/dav/bob.vcf", bob.ID, `W/"fixture-etag-2"`, "sha256:bob"},
	}
	for _, l := range links {
		require.NoError(t, f.DB.Exec(`
			INSERT INTO contact_sync_links (created_at, updated_at, subscription_id, user_id, href, contact_id, etag, content_hash)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			now, now, subscriptionID, userID, l.href, l.contactID, l.etag, l.contentHash).Error, "seeding a contact sync link")
	}
}

// assertCriteriaPopulated proves the milestone's named criteria are covered
// with real rows, not vacuous 0==0 comparisons: contacts, relationships
// (direction lives in the source_id/target_id/type/directional columns the
// content snapshot compares), custom fields, notes, life events, gifts, files,
// circles, tags, external references, and audit history. A future fixture
// change that stops populating one of them fails here.
func assertCriteriaPopulated(t *testing.T, f *Fixture) {
	t.Helper()
	criteria := map[string]string{
		"contact counts":      "contacts",
		"relationships":       "relationship_edges",
		"custom fields":       "field_values",
		"notes":               "notes",
		"life events":         "life_events",
		"gifts":               "gifts",
		"files (attachments)": "attachments",
		"circles":             "circles",
		"tags":                "tags",
		"external references": "external_identities",
		"audit history":       "audit_events",
	}
	for name, table := range criteria {
		var n int64
		require.NoError(t, f.DB.Raw("SELECT count(*) FROM "+quoteIdentForSnapshot(table)).Scan(&n).Error)
		assert.Positivef(t, n, "%s criterion: table %s must be populated in the %s fixture so the migration suite exercises it", name, table, f.Release.Tag)
	}
}

// contactRecord pairs a manifest contact name with its canonical Record as
// read from the database (RecordForContact — the authoritative read, CLAUDE.md
// backend trap #3; RecordFromContact is never used here because it rebuilds
// from flat fields and drops exactly the data this suite protects).
type contactRecord struct {
	name   string
	record *contactmodel.Record
}

// contactRecords reads every fixture contact's canonical Record, Unscoped so
// a soft-deleted contact is still read (a migration that hard-deletes it must
// fail this suite, not vanish from it).
func contactRecords(t *testing.T, db *gorm.DB, ds *canonicalfixture.Dataset) []contactRecord {
	t.Helper()
	names := make([]string, 0, len(ds.Contacts))
	for name := range ds.Contacts {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]contactRecord, 0, len(names))
	for _, name := range names {
		out = append(out, contactRecordFromDB(t, db, ds.Contacts[name].ID, name))
	}
	return out
}

// contactRecordFromDB reads one contact's canonical Record through
// RecordForContact — the mandated read, never RecordFromContact (CLAUDE.md
// backend trap #3). The historical fixtures store DATETIME columns as TEXT
// (the schema-copy serializes them losslessly but not with the driver's time
// affinity), so a full gorm.Model scan fails on CreatedAt; the row is read
// column-by-column instead and the neutral card/crm/passthrough columns are
// deserialized exactly the way the storage serializer does, so RecordForContact
// sees the same Card the app sees. The explicit column tags are load-bearing
// (backend trap #1): GORM's scan would otherwise derive `v_card_uid`/`e_tag`
// and silently read neither.
func contactRecordFromDB(t *testing.T, db *gorm.DB, id uint, name string) contactRecord {
	t.Helper()
	var row struct {
		ID           uint
		VCardUID     string `gorm:"column:vcard_uid"`
		ETag         string `gorm:"column:etag"`
		CardJSON     string `gorm:"column:card"`
		CRMJSON      string `gorm:"column:crm"`
		PassJSON     string `gorm:"column:passthrough"`
		Firstname    string
		Lastname     string
		Organization string
		Email        string
		Phone        string
		Birthday     string
	}
	require.NoError(t, db.Unscoped().Raw(`
		SELECT id, vcard_uid, etag, card, crm, passthrough,
		       firstname, lastname, organization, email, phone, birthday
		FROM contacts WHERE id = ?`, id).Scan(&row).Error)
	require.NotEmptyf(t, row.VCardUID, "contact %s: the stored row must carry its vcard_uid (GORM column-mapping regression?)", name)

	var c models.Contact
	c.ID = row.ID
	c.VCardUID = row.VCardUID
	c.ETag = row.ETag
	c.Firstname = row.Firstname
	c.Lastname = row.Lastname
	c.Organization = row.Organization
	c.Email = row.Email
	c.Phone = row.Phone
	c.Birthday = row.Birthday
	if row.CardJSON != "" {
		require.NoError(t, json.Unmarshal([]byte(row.CardJSON), &c.Card), "contact %s: card column is not valid neutral JSON", name)
	}
	if row.CRMJSON != "" {
		require.NoError(t, json.Unmarshal([]byte(row.CRMJSON), &c.CRM), "contact %s: crm column is not valid neutral JSON", name)
	}
	if row.PassJSON != "" {
		require.NoError(t, json.Unmarshal([]byte(row.PassJSON), &c.Passthrough), "contact %s: passthrough column is not valid neutral JSON", name)
	}

	rec := models.RecordForContact(&c, "", nil)
	require.NotNil(t, rec, "RecordForContact must never return nil for contact %s", name)
	return contactRecord{name: name, record: rec}
}

// assertContactsSemanticallyEqual is the issue's decided oracle: every
// contact's canonical Record must be semantically equivalent before and after
// the migration, per TEST-03's comparison (internal/semanticequal). A
// migration that silently drops a field with no flat-column home (trap #3)
// fails here NAMING the concept (email, adr, name.full, ...).
func assertContactsSemanticallyEqual(t *testing.T, fromTag string, before, after []contactRecord) {
	t.Helper()
	for i := range before {
		rep := semanticequal.Compare(before[i].record, after[i].record)
		if !rep.Equal() {
			t.Errorf("%s: contact %s is not semantically equivalent after the migration:\n%s", fromTag, before[i].name, rep.DiffText())
		}
	}
}
