package controllers

// TEST-05 (issue #433): golden tests for contact merging.
//
// Contact merge is the highest-risk destructive operation in the product: it
// repoints every association off the loser onto the keeper and then deletes
// the loser. A regression that drops an association is permanent and silent.
// The tests here formulate the post-merge state as a *golden snapshot*: every
// association table is captured — counted *and* content-checked — into one
// comparable struct, and the expected struct is written out in full. A dropped
// row changes a count or a content entry and testify's diff names exactly
// which table diverged.
//
// Unlike the existing contact_merge_real_db_test.go (which spot-checks the
// association the test author was thinking about), the keeper and loser here
// are drawn from the TEST-02 canonical fixture (#430,
// testdata/canonical-fixture/manifest.json), so every association type is
// present by construction rather than by arrangement. Scenario-specific rows
// the fixture does not create (reminders, reminder completions, conversation
// agenda items, external activities, reach-out suggestions, cadence policies,
// CardDAV sync links, a reciprocal edge) are added on top of the fixture in
// the test itself, exactly the way the fixture's own README permits; the
// keeper/loser contacts and their manifest-declared associations stay
// canonical.
//
// Hand-verification of each golden (per the issue's "How to verify"):
//   - delete one association type from RepointContactAssociations -> the
//     golden fails and the diff names the orphan table;
//   - break one union* dedup key -> a duplicate appears in the snapshot diff;
//   - the audit note is asserted for every count the merge claims, so a
//     repoint that silently stops moving something fails the note too.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/canonicalfixture"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// cadenceGolden is the comparable form of a CadencePolicy: the interval plus
// the qualifying types (sorted, so insertion order never reads as a change).
type cadenceGolden struct {
	Interval   int
	Qualifying string
}

// goldenMergeState is the complete post-merge state of a keeper/loser pair.
// Every association table the merge touches is captured. User-authored content
// (which soft-deletes) is captured Unscoped with a "[deleted] " tombstone
// marker so a soft-deleted row that must survive (or stay behind with the
// loser's tombstone) is part of the golden, not invisible to it; join/edge
// rows (which hard-delete) are captured with plain queries.
type goldenMergeState struct {
	// Scalar outcome on the keeper.
	Firstname          string
	Lastname           string
	MiddleName         string
	Prefix             string
	Suffix             string
	Nickname           string
	Gender             string
	Birthday           string
	Anniversary        string
	Organization       string
	Department         string
	JobTitle           string
	Role               string
	HowWeMet           string
	WorkInformation    string
	ContactInformation string

	// Multi-valued unions on the keeper ("type|value" — the same key each
	// union function dedups on, so a broken dedup shows up here directly).
	Emails    []string
	Phones    []string
	Addresses []string
	URLs      []string
	IMPPs     []string
	Circles   []string

	// Association content sets (sorted; every slice is sorted by the
	// capture, and the expected side is written sorted too).
	Notes               []string
	Reminders           []string
	ReminderCompletions []string
	Activities          []string
	RelationshipEdges   []string
	Households          []string
	CircleMemberships   []string
	Tags                []string
	LifeEvents          []string
	ConversationAgenda  []string
	Gifts               []string
	FieldValues         map[string]string
	Attachments         []string
	Preferences         []string
	ExternalIdentities  []string
	ExternalActivities  []string
	ReachOutSuggestions []string
	CadencePolicy       *cadenceGolden

	// Loser-side. LoserLiveOrphans is every association table's count of
	// *live* rows still pointing at the loser after the merge — the precise
	// invariant that distinguishes "repointed" from "merely marked", since a
	// plain Count() passes whether a row is gone or merely tombstoned
	// (CLAUDE.md backend trap #6). For soft-delete tables the scope excludes
	// the loser's own tombstoned rows, which legitimately stay with it.
	LoserSoftDeleted bool
	LoserLiveOrphans map[string]int64
}

// sortStrings is a tiny alias so call sites read as intent.
func sortStrings(s []string) { sort.Strings(s) }

// captureGoldenMergeState reads the live database and produces the golden
// state for a post-merge keeper/loser pair. It is the only DB-reading surface
// the goldens use; the expected side is written out by hand. names maps a
// Contact.VCardUID to its fixture name so the golden reads "ada|celine|..."
// instead of "10000000-...-0001|10000000-...-0003|..."; uids without a name
// (e.g. a tombstoned contact) render as themselves.
func captureGoldenMergeState(t *testing.T, db *gorm.DB, userID uint, keeper, loser models.Contact, names map[string]string) goldenMergeState {
	t.Helper()
	resolve := func(uid string) string {
		if name, ok := names[uid]; ok {
			return name
		}
		return uid
	}
	var g goldenMergeState

	var k models.Contact
	require.NoError(t, db.First(&k, keeper.ID).Error)
	g.Firstname = k.Firstname
	g.Lastname = k.Lastname
	g.MiddleName = k.MiddleName
	g.Prefix = k.Prefix
	g.Suffix = k.Suffix
	g.Nickname = k.Nickname
	g.Gender = k.Gender
	g.Birthday = k.Birthday
	g.Anniversary = k.Anniversary
	g.Organization = k.Organization
	g.Department = k.Department
	g.JobTitle = k.JobTitle
	g.Role = k.Role
	g.HowWeMet = k.HowWeMet
	g.WorkInformation = k.WorkInformation
	g.ContactInformation = k.ContactInformation

	for _, e := range k.Emails {
		g.Emails = append(g.Emails, e.Type+"|"+e.Value)
	}
	for _, p := range k.Phones {
		g.Phones = append(g.Phones, p.Type+"|"+p.Value)
	}
	for _, a := range k.Addresses {
		g.Addresses = append(g.Addresses, a.Type+"|"+models.FormatAddress(a))
	}
	for _, u := range k.URLs {
		g.URLs = append(g.URLs, u.Type+"|"+u.Value)
	}
	for _, i := range k.IMPPs {
		g.IMPPs = append(g.IMPPs, i.Type+"|"+i.Value)
	}
	g.Circles = append([]string(nil), k.Circles...)

	// Notes: Unscoped, tombstone-marked, audit note set aside.
	var notes []models.Note
	require.NoError(t, db.Unscoped().Where("contact_id = ? AND user_id = ?", keeper.ID, userID).Find(&notes).Error)
	for _, n := range notes {
		content := n.Content
		if strings.Contains(content, "Merged contact #") && strings.Contains(content, "into this record.") {
			continue // the merge audit note — asserted separately, not among user notes
		}
		if n.DeletedAt.Valid {
			content = "[deleted] " + content
		}
		g.Notes = append(g.Notes, content)
	}

	// Reminders / reminder completions.
	var reminders []models.Reminder
	require.NoError(t, db.Unscoped().Where("contact_id = ? AND user_id = ?", keeper.ID, userID).Find(&reminders).Error)
	for _, r := range reminders {
		content := r.Message
		if r.DeletedAt.Valid {
			content = "[deleted] " + content
		}
		g.Reminders = append(g.Reminders, content)
	}
	var completions []models.ReminderCompletion
	require.NoError(t, db.Unscoped().Where("contact_id = ? AND user_id = ?", keeper.ID, userID).Find(&completions).Error)
	for _, c := range completions {
		content := c.Message
		if c.DeletedAt.Valid {
			content = "[deleted] " + content
		}
		g.ReminderCompletions = append(g.ReminderCompletions, content)
	}

	// Activities via the activity_contacts join. Raw join query because
	// activity_contacts has no Go model; Unscoped-on-scan keeps soft-deleted
	// activities visible so a repoint into a tombstoned activity is pinned.
	var activities []models.Activity
	require.NoError(t, db.Raw(
		"SELECT a.* FROM activities a JOIN activity_contacts ac ON ac.activity_id = a.id WHERE ac.contact_id = ? AND a.user_id = ?",
		keeper.ID, userID,
	).Scan(&activities).Error)
	for _, a := range activities {
		title := a.Title
		if a.DeletedAt.Valid {
			title = "[deleted] " + title
		}
		g.Activities = append(g.Activities, title)
	}

	// Relationship edges: hard-delete, plain scope, the whole user graph (a
	// merge can change edges that never touched the keeper — e.g. an edge
	// repointed from the loser onto a third contact). Captured as the exact
	// directed tuple by contact name so the direction rule (Type describes
	// the *source's* role) is pinned in the golden, not just the count.
	var edges []models.RelationshipEdge
	require.NoError(t, db.Where("user_id = ?", userID).Find(&edges).Error)
	for _, e := range edges {
		g.RelationshipEdges = append(g.RelationshipEdges,
			fmt.Sprintf("%s|%s|%s|%s|%s|%v", resolve(e.SourceID), resolve(e.TargetID), e.Type, e.Status, e.Sensitivity, e.Confidence))
	}

	// Households / circles / tags (join rows).
	var households []models.Household
	require.NoError(t, db.Model(&models.Household{}).
		Joins("JOIN household_members hm ON hm.household_id = households.id AND hm.member_vcard_uid = ? AND hm.user_id = ?", k.VCardUID, userID).
		Where("households.user_id = ?", userID).Find(&households).Error)
	for _, h := range households {
		g.Households = append(g.Households, h.Name)
	}
	var circles []models.Circle
	require.NoError(t, db.Model(&models.Circle{}).
		Joins("JOIN circle_members cm ON cm.circle_id = circles.id AND cm.member_vcard_uid = ? AND cm.user_id = ?", k.VCardUID, userID).
		Where("circles.user_id = ?", userID).Find(&circles).Error)
	for _, c := range circles {
		g.CircleMemberships = append(g.CircleMemberships, c.Name)
	}
	var tags []models.Tag
	require.NoError(t, db.Model(&models.Tag{}).
		Joins("JOIN contact_tags ct ON ct.tag_id = tags.id AND ct.contact_vcard_uid = ? AND ct.user_id = ?", k.VCardUID, userID).
		Where("tags.user_id = ?", userID).Find(&tags).Error)
	for _, tg := range tags {
		g.Tags = append(g.Tags, tg.Name)
	}

	// Life events: Unscoped (soft-delete), entity_id == keeper uid.
	var lifeEvents []models.LifeEvent
	require.NoError(t, db.Unscoped().Where("entity_id = ? AND user_id = ?", k.VCardUID, userID).Find(&lifeEvents).Error)
	for _, le := range lifeEvents {
		related := make([]string, 0, len(le.RelatedEntityIDs))
		for _, id := range le.RelatedEntityIDs {
			related = append(related, resolve(id))
		}
		sort.Strings(related)
		content := fmt.Sprintf("%s|%s|[%s]", le.Type, le.Description, strings.Join(related, ","))
		if le.DeletedAt.Valid {
			content = "[deleted] " + content
		}
		g.LifeEvents = append(g.LifeEvents, content)
	}

	// Conversation agenda items (Unscoped, soft-delete, entity_id key).
	var agenda []models.ConversationAgenda
	require.NoError(t, db.Unscoped().Where("entity_id = ? AND user_id = ?", k.VCardUID, userID).Find(&agenda).Error)
	for _, a := range agenda {
		content := a.Content
		if a.DeletedAt.Valid {
			content = "[deleted] " + content
		}
		g.ConversationAgenda = append(g.ConversationAgenda, content)
	}

	// Gifts (Unscoped, soft-delete, entity_id key).
	var gifts []models.Gift
	require.NoError(t, db.Unscoped().Where("entity_id = ? AND user_id = ?", k.VCardUID, userID).Find(&gifts).Error)
	for _, gift := range gifts {
		content := fmt.Sprintf("%s|%s|%s|%d|%s", gift.Description, gift.Status, gift.Occasion, gift.ValueCents, gift.Currency)
		if gift.DeletedAt.Valid {
			content = "[deleted] " + content
		}
		g.Gifts = append(g.Gifts, content)
	}

	// Custom field values, keyed by field definition key. Value is a
	// json.RawMessage; decode to the plain value for the golden.
	var values []struct {
		Key   string
		Value string
	}
	require.NoError(t, db.Raw(
		"SELECT fd.key, fv.value FROM field_values fv JOIN field_definitions fd ON fd.id = fv.field_definition_id "+
			"WHERE fv.entity_id = ? AND fv.user_id = ?", k.VCardUID, userID,
	).Scan(&values).Error)
	g.FieldValues = map[string]string{}
	for _, v := range values {
		var decoded string
		if err := json.Unmarshal([]byte(v.Value), &decoded); err == nil {
			g.FieldValues[v.Key] = decoded
		} else {
			g.FieldValues[v.Key] = v.Value
		}
	}

	// Attachments (Unscoped, soft-delete).
	var attachments []models.Attachment
	require.NoError(t, db.Unscoped().Where("contact_vcard_uid = ? AND user_id = ?", k.VCardUID, userID).Find(&attachments).Error)
	for _, a := range attachments {
		content := fmt.Sprintf("%s|%s|%s|%d", a.StoredName, a.OriginalName, a.ContentType, a.SizeBytes)
		if a.DeletedAt.Valid {
			content = "[deleted] " + content
		}
		g.Attachments = append(g.Attachments, content)
	}

	// Preferences (Unscoped, soft-delete).
	var prefs []models.Preference
	require.NoError(t, db.Unscoped().Where("entity_id = ? AND user_id = ?", k.VCardUID, userID).Find(&prefs).Error)
	for _, p := range prefs {
		content := fmt.Sprintf("%s|%s|%s|%s", p.Category, p.Key, p.Value, p.Sensitivity)
		if p.DeletedAt.Valid {
			content = "[deleted] " + content
		}
		g.Preferences = append(g.Preferences, content)
	}

	// External identities / external activities (hard-delete).
	var identities []models.ExternalIdentity
	require.NoError(t, db.Where("entity_id = ? AND user_id = ?", k.VCardUID, userID).Find(&identities).Error)
	for _, idn := range identities {
		g.ExternalIdentities = append(g.ExternalIdentities, fmt.Sprintf("%s|%s|%s", idn.System, idn.ExternalID, idn.SyncStatus))
	}
	var externalActivities []models.ExternalActivity
	require.NoError(t, db.Where("entity_id = ? AND user_id = ?", k.VCardUID, userID).Find(&externalActivities).Error)
	for _, ea := range externalActivities {
		g.ExternalActivities = append(g.ExternalActivities, fmt.Sprintf("%s|%s|%s", ea.SourceSystem, ea.ExternalID, ea.Type))
	}

	// Reach-out suggestions (hard-delete).
	var suggestions []models.ReachOutSuggestion
	require.NoError(t, db.Where("contact_vcard_uid = ? AND user_id = ?", k.VCardUID, userID).Find(&suggestions).Error)
	for _, s := range suggestions {
		g.ReachOutSuggestions = append(g.ReachOutSuggestions, fmt.Sprintf("%s|%s", s.Kind, s.Status))
	}

	// Cadence policy (one per contact; nil when absent).
	var cp models.CadencePolicy
	err := db.Where("entity_id = ? AND user_id = ?", k.VCardUID, userID).First(&cp).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		g.CadencePolicy = nil
	} else {
		require.NoError(t, err)
		qualifying := append([]string(nil), cp.QualifyingTypes...)
		sort.Strings(qualifying)
		g.CadencePolicy = &cadenceGolden{Interval: cp.TargetIntervalDays, Qualifying: strings.Join(qualifying, ",")}
	}

	// Loser-side.
	var loserLive, loserUnscoped int64
	require.NoError(t, db.Model(&models.Contact{}).Where("id = ? AND user_id = ?", loser.ID, userID).Count(&loserLive).Error)
	require.NoError(t, db.Unscoped().Model(&models.Contact{}).Where("id = ? AND user_id = ?", loser.ID, userID).Count(&loserUnscoped).Error)
	g.LoserSoftDeleted = loserLive == 0 && loserUnscoped == 1

	g.LoserLiveOrphans = captureLoserLiveOrphans(t, db, userID, loser)

	// Normalize empties: a golden must never distinguish "absent" from "empty"
	// (the exact ambiguity the fixture's own trap list warns about), so every
	// slice that gathered nothing is recorded as a clean []string{}, never nil.
	for _, s := range []*[]string{
		&g.Emails, &g.Phones, &g.Addresses, &g.URLs, &g.IMPPs, &g.Circles,
		&g.Notes, &g.Reminders, &g.ReminderCompletions, &g.Activities,
		&g.RelationshipEdges, &g.Households, &g.CircleMemberships, &g.Tags,
		&g.LifeEvents, &g.ConversationAgenda, &g.Gifts, &g.Attachments,
		&g.Preferences, &g.ExternalIdentities, &g.ExternalActivities, &g.ReachOutSuggestions,
	} {
		if *s == nil {
			*s = []string{}
		}
		sortStrings(*s)
	}
	return g
}

// captureLoserLiveOrphans counts every association table's *live* rows still
// pointing at the loser. Soft-delete tables are counted with their default
// scope (tombstoned rows are the loser's own recoverable content and
// legitimately stay); hard-delete tables have nothing to hide behind, so a
// nonzero there is a hard orphan.
func captureLoserLiveOrphans(t *testing.T, db *gorm.DB, userID uint, loser models.Contact) map[string]int64 {
	t.Helper()
	uid, id := loser.VCardUID, loser.ID
	count := func(model any, where string, args ...any) int64 {
		t.Helper()
		var n int64
		require.NoError(t, db.Model(model).Where(where, args...).Count(&n).Error)
		return n
	}
	m := map[string]int64{
		"notes":                 count(&models.Note{}, "contact_id = ? AND user_id = ?", id, userID),
		"reminders":             count(&models.Reminder{}, "contact_id = ? AND user_id = ?", id, userID),
		"reminder_completions":  count(&models.ReminderCompletion{}, "contact_id = ? AND user_id = ?", id, userID),
		"relationship_edges":    count(&models.RelationshipEdge{}, "user_id = ? AND (source_id = ? OR target_id = ?)", userID, uid, uid),
		"household_members":     count(&models.HouseholdMember{}, "member_vcard_uid = ? AND user_id = ?", uid, userID),
		"circle_members":        count(&models.CircleMember{}, "member_vcard_uid = ? AND user_id = ?", uid, userID),
		"contact_tags":          count(&models.ContactTag{}, "contact_vcard_uid = ? AND user_id = ?", uid, userID),
		"life_events":           count(&models.LifeEvent{}, "entity_id = ? AND user_id = ?", uid, userID),
		"conversation_agenda":   count(&models.ConversationAgenda{}, "entity_id = ? AND user_id = ?", uid, userID),
		"gifts":                 count(&models.Gift{}, "entity_id = ? AND user_id = ?", uid, userID),
		"field_values":          count(&models.FieldValue{}, "entity_id = ? AND user_id = ?", uid, userID),
		"attachments":           count(&models.Attachment{}, "contact_vcard_uid = ? AND user_id = ?", uid, userID),
		"preferences":           count(&models.Preference{}, "entity_id = ? AND user_id = ?", uid, userID),
		"external_identities":   count(&models.ExternalIdentity{}, "entity_id = ? AND user_id = ?", uid, userID),
		"external_activities":   count(&models.ExternalActivity{}, "entity_id = ? AND user_id = ?", uid, userID),
		"cadence_policies":      count(&models.CadencePolicy{}, "entity_id = ? AND user_id = ?", uid, userID),
		"reach_out_suggestions": count(&models.ReachOutSuggestion{}, "contact_vcard_uid = ? AND user_id = ?", uid, userID),
		"contact_sync_links":    count(&models.ContactSyncLink{}, "contact_id = ? AND user_id = ?", id, userID),
	}
	var act int64
	require.NoError(t, db.Raw(
		"SELECT COUNT(*) FROM activity_contacts ac JOIN activities a ON a.id = ac.activity_id WHERE ac.contact_id = ? AND a.user_id = ?",
		id, userID,
	).Scan(&act).Error)
	m["activity_contacts"] = act
	return m
}

// zeroOrphanCounts is the expected LoserLiveOrphans value of a clean merge:
// nothing — live — may still reference the loser.
func zeroOrphanCounts() map[string]int64 {
	return map[string]int64{
		"notes": 0, "reminders": 0, "reminder_completions": 0,
		"relationship_edges": 0, "household_members": 0, "circle_members": 0,
		"contact_tags": 0, "life_events": 0, "conversation_agenda": 0, "gifts": 0,
		"field_values": 0, "attachments": 0, "preferences": 0,
		"external_identities": 0, "external_activities": 0, "cadence_policies": 0,
		"reach_out_suggestions": 0, "contact_sync_links": 0, "activity_contacts": 0,
	}
}

// goldenMergeRouter builds the two merge endpoints on a real migrated DB the
// way the app wires them, scoped to userID.
func goldenMergeRouter(t *testing.T, db *gorm.DB, userID uint) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", userID)
		c.Set("cfg", config.Config{})
		c.Next()
	})
	r.POST("/contacts/merge/preview", withValidated(func() any { return &models.ContactMergeRequest{} }), PreviewContactMerge)
	r.POST("/contacts/merge", withValidated(func() any { return &models.ContactMergeRequest{} }), CommitContactMerge)
	return r
}

// doGoldenMerge drives one endpoint call against the golden router.
func doGoldenMerge(t *testing.T, router *gin.Engine, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(body))
	req, _ := http.NewRequest("POST", path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// richPairScenario is the shared setup for the ada+bob golden: the populated
// fixture plus every association type the fixture does not itself create,
// placed on either side of the merge so the golden covers the whole
// RepointContactAssociations surface.
type richPairScenario struct {
	db          *gorm.DB
	router      *gin.Engine
	user        models.User
	ds          *canonicalfixture.Dataset
	ada, bob    models.Contact
	adaNoteText string
	bobNoteText string
	resolutions map[string]string
}

func setupRichPairScenario(t *testing.T) *richPairScenario {
	t.Helper()
	db := dbtest.New(t)
	closeTestDBAtTeardown(t, db)

	m, err := canonicalfixture.Read()
	require.NoError(t, err)
	ds, err := canonicalfixture.Populate(db, m)
	require.NoError(t, err)

	ada := ds.Contacts["ada"]
	bob := ds.Contacts["bob"]

	// Association rows the fixture does not create, all on the loser side so
	// the merge has to move them (or, for the sync link, deliberately discard
	// them).
	subscription := models.ContactSubscription{UserID: ds.User.ID, Name: "Golden CardDAV", URL: "https://example.com/dav"}
	require.NoError(t, db.Create(&subscription).Error)
	require.NoError(t, db.Create(&models.ContactSyncLink{
		SubscriptionID: subscription.ID, UserID: ds.User.ID, Href: "/dav/bob-golden.vcf",
		ContactID: bob.ID, ContentHash: "abc123",
	}).Error)

	reminder := models.Reminder{
		UserID: ds.User.ID, ContactID: &bob.ID, Message: "Follow up re tax docs",
		Recurrence: "once", RemindAt: time.Now().Add(24 * time.Hour),
	}
	require.NoError(t, db.Create(&reminder).Error)
	require.NoError(t, db.Create(&models.ReminderCompletion{
		UserID: ds.User.ID, ContactID: bob.ID, ReminderID: &reminder.ID,
		Message: "done", CompletedAt: time.Now(),
	}).Error)

	require.NoError(t, db.Create(&models.ConversationAgenda{
		UserID: ds.User.ID, EntityID: bob.VCardUID, Content: "Ask about Berlin move",
	}).Error)
	require.NoError(t, db.Create(&models.ExternalActivity{
		UserID: ds.User.ID, EntityID: bob.VCardUID, SourceSystem: "immich",
		ExternalID: "asset-bob-golden", Type: "photo-appearance", OccurredAt: time.Now(),
	}).Error)
	require.NoError(t, db.Create(&models.ReachOutSuggestion{
		UserID: ds.User.ID, ContactVCardUID: bob.VCardUID, Kind: "title",
		OldValue: "Accountant", NewValue: "Treasurer", AuditEventID: 1, Status: models.ReachOutStatusPending,
	}).Error)

	// Cadence policies on both sides: a genuine one-per-contact conflict (the
	// fixture creates none, so this is the scenario row that makes the golden
	// cover the cadence-policy conflict branch).
	require.NoError(t, db.Create(&models.CadencePolicy{UserID: ds.User.ID, EntityID: ada.VCardUID, TargetIntervalDays: 30}).Error)
	require.NoError(t, db.Create(&models.CadencePolicy{
		UserID: ds.User.ID, EntityID: bob.VCardUID, TargetIntervalDays: 60,
		QualifyingTypes: []string{"call", "visit"},
	}).Error)

	// A reciprocal edge: eve->ada child_of (suggested, household-inferred)
	// mirrors ada->eve parent_of (confirmed). When bob merges in, both
	// reference ada, so the inverse-pair dedup must keep exactly the
	// confirmed parent_of and drop the suggested child_of — the direction
	// rule pinned in the golden.
	require.NoError(t, db.Create(&models.RelationshipEdge{
		UserID: ds.User.ID, SourceID: ds.Contacts["eve"].VCardUID, TargetID: ada.VCardUID, Type: "child_of",
		Source: models.RelationshipSourceHouseholdInferred, Confidence: 0.5,
		Status: models.RelationshipStatusSuggested, Sensitivity: models.RelationshipSensitivityNormal,
	}).Error)

	// users.self_contact_vcard_uid ("Me") pointed at the loser: the merge
	// must move it to the keeper, not leave it dangling on a tombstone.
	uid := bob.VCardUID
	require.NoError(t, db.Model(&models.User{}).Where("id = ?", ds.User.ID).Update("self_contact_vcard_uid", uid).Error)

	var adaNoteText, bobNoteText string
	for _, n := range ds.Notes {
		if n.ContactID == nil {
			continue
		}
		switch *n.ContactID {
		case ada.ID:
			adaNoteText = n.Content
		case bob.ID:
			bobNoteText = n.Content
		}
	}
	require.NotEmpty(t, adaNoteText)
	require.NotEmpty(t, bobNoteText)

	resolutions := map[string]string{
		"firstname":           "Ada",
		"lastname":            "Lovelace",
		"nickname":            "Bobby", // loser's value wins on purpose: the golden must pin loser-picks, not only keeper-picks
		"gender":              "non-binary",
		"birthday":            "1815-12-10",
		"anniversary":         "1835-07-08",
		"organization":        "Analytical Engine Co.",
		"department":          "Research",
		"job_title":           "Mathematician",
		"role":                "Author",
		"how_we_met":          "Through the Analytical Engine project.",
		"work_information":    "First programmer.",
		"contact_information": "Prefers written correspondence.",
		"cadence_policy":      "Every 60 days (call, visit)",
	}

	return &richPairScenario{
		db: db, router: goldenMergeRouter(t, db, ds.User.ID), user: ds.User, ds: ds,
		ada: ada, bob: bob, adaNoteText: adaNoteText, bobNoteText: bobNoteText,
		resolutions: resolutions,
	}
}

// TestContactMerge_Golden_RichFixturePair is the primary golden: keeper Ada,
// loser Bob, both drawn from the TEST-02 fixture. Every association type is
// present on the loser (fixture-declared or scenario-added), both sides carry
// conflicting scalars and multi-valued fields, and the merge is resolved with
// a mix of keeper-picks and a loser-pick (nickname). The whole post-merge
// state is asserted as one snapshot — a dropped repoint, a broken union dedup,
// an inverted edge, or a missed resolution all fail the golden and the diff
// names the table.
func TestContactMerge_Golden_RichFixturePair(t *testing.T) {
	sc := setupRichPairScenario(t)

	// --- Preview: the association counts are part of the golden too. ---
	previewResp := doGoldenMerge(t, sc.router, "/contacts/merge/preview",
		models.ContactMergeRequest{KeepID: sc.ada.ID, MergeID: sc.bob.ID})
	require.Equal(t, http.StatusOK, previewResp.Code, previewResp.Body.String())
	var preview models.ContactMergePreviewResponse
	require.NoError(t, json.Unmarshal(previewResp.Body.Bytes(), &preview))

	// 13 scalar conflicts (every mergeScalarFields entry where both sides
	// disagree) plus the cadence-policy conflict.
	require.Len(t, preview.Resolution.Conflicts, 14, "conflicts: %+v", preview.Resolution.Conflicts)
	conflictByField := make(map[string]models.ContactMergeFieldConflict, len(preview.Resolution.Conflicts))
	for _, c := range preview.Resolution.Conflicts {
		conflictByField[c.Field] = c
	}
	firstnameConflict, ok := conflictByField["firstname"]
	require.True(t, ok, "firstname must be among the conflicts")
	assert.Equal(t, "Ada", firstnameConflict.KeeperValue)
	assert.Equal(t, "Bob", firstnameConflict.LoserValue)
	require.Contains(t, conflictByField, "cadence_policy", "the cadence-policy conflict must be present")
	assert.Equal(t, "Every 30 days", conflictByField["cadence_policy"].KeeperValue)
	assert.Equal(t, "Every 60 days (call, visit)", conflictByField["cadence_policy"].LoserValue)
	require.Equal(t, models.ContactMergeAssociationCounts{
		Notes:                   1,
		Activities:              1,
		Reminders:               1,
		ReminderCompletions:     1,
		RelationshipEdges:       3,
		HouseholdMemberships:    1,
		CircleMemberships:       1,
		Tags:                    1,
		LifeEvents:              1,
		LifeEventReferences:     0,
		ConversationAgendaItems: 1,
		GiftItems:               0,
		FieldValues:             1,
		ContactSyncLinks:        1,
		Attachments:             1,
		Preferences:             1,
		ExternalIdentities:      1,
		ExternalActivities:      1,
		CadencePolicies:         1,
		ReachOutSuggestions:     1,
	}, preview.AssociationCounts)

	// --- Commit. ---
	commitResp := doGoldenMerge(t, sc.router, "/contacts/merge",
		models.ContactMergeRequest{KeepID: sc.ada.ID, MergeID: sc.bob.ID, Resolutions: sc.resolutions})
	require.Equal(t, http.StatusOK, commitResp.Code, commitResp.Body.String())

	got := captureGoldenMergeState(t, sc.db, sc.user.ID, sc.ada, sc.bob, fixtureNames(sc.ds))

	want := goldenMergeState{
		// Scalar outcome: 13 conflicts resolved (nickname to the loser's
		// value, the rest to the keeper's), cadence to the loser's policy.
		Firstname: "Ada", Lastname: "Lovelace", MiddleName: "Augusta",
		Prefix: "Countess", Suffix: "OL",
		Nickname: "Bobby", Gender: "non-binary",
		Birthday: "1815-12-10", Anniversary: "1835-07-08",
		Organization: "Analytical Engine Co.", Department: "Research",
		JobTitle: "Mathematician", Role: "Author",
		HowWeMet:           "Through the Analytical Engine project.",
		WorkInformation:    "First programmer.",
		ContactInformation: "Prefers written correspondence.",

		// Multi-valued unions: no cross-pair duplicates among fixture contacts,
		// so every entry survives; keeper entries first.
		Emails: []string{
			"home|augusta@babbage.example",
			"home|bob@home.example",
			"tax|robert.smith+tax@example.com",
			"work|ada@lovelace.example",
			"work|bob.smith@example.com",
		},
		Phones: []string{
			"cell|+44 20 7946 0958",
			"home|+44 20 7946 0000",
			"home|+49 30 901821",
			"mobile|+1 555 0100 42",
			"work|+49 30 901820",
		},
		Addresses: []string{
			"home|8 High Street, Old Town, Sussex, BN1 1AA, United Kingdom",
			"work|1 St James Square, London, Greater London, SW1Y 4JX, United Kingdom",
			"work|Musterstraße 1, PO Box 42, Apt 4B, 3, Berlin, Berlin, 10115, Germany",
		},
		URLs: []string{},
		IMPPs: []string{
			"matrix|https://matrix.to/#/@ada:example.org",
			"signal|tel:+4930901821",
		},
		Circles: []string{"legacy-work"},

		// Notes: the keeper's own plus Bob's long note (repointed). The audit
		// note is asserted separately below.
		Notes: []string{sc.adaNoteText, sc.bobNoteText},

		Reminders:           []string{"Follow up re tax docs"},
		ReminderCompletions: []string{"done"},
		Activities:          []string{"Coffee catch-up"},

		// Relationship edges: the coworker self-loop dropped; the two
		// exact-duplicate friend_of/parent_of pairs collapsed to the
		// higher-confidence survivor; the inverse child_of dropped. The
		// survivor tuples pin the direction rule — the stored edge is always
		// the source's role relative to the target, never inverted.
		RelationshipEdges: []string{
			"ada|celine|friend_of|confirmed|normal|1",
			"ada|eve|parent_of|confirmed|normal|1",
			"ada|frank|owns|confirmed|normal|1",
			"ada|hugo|mentor_of|confirmed|secret|1",
			"hugo|ida|spouse_of|confirmed|private|1",
		},
		Households:        []string{"Smith Family"},
		CircleMemberships: []string{"book-club", "work"},
		Tags:              []string{"vip"},
		LifeEvents: []string{
			"graduated|BSc Computer Science.|[]",
			"had_child|Eve was born.|[eve]",
			"published_a_paper|Sketch of the Analytical Engine.|[]",
		},
		ConversationAgenda: []string{"Ask about Berlin move"},
		Gifts:              []string{"Hand-knitted scarf|given|birthday|3500|USD"},
		FieldValues: map[string]string{
			"dietary_restrictions": "vegetarian",
			"favorite_coffee":      "pour-over",
			"private_nickname":     "stormy",
			"shoe_size":            "43",
		},
		Attachments: []string{
			"[deleted] f3-welcome.pdf|welcome-letter.pdf|application/pdf|8192",
			"f1-2025-tax.pdf|tax-forms-2025.pdf|application/pdf|245760",
		},
		Preferences: []string{
			"hobby|favorite|chess|private",
			"hobby|favorite|sailing|normal",
		},
		ExternalIdentities: []string{
			"immich|person-ada-lovelace|synced",
			"paperless|doc-2025-tax|idle",
		},
		ExternalActivities:  []string{"immich|asset-bob-golden|photo-appearance"},
		ReachOutSuggestions: []string{"title|pending"},
		CadencePolicy:       &cadenceGolden{Interval: 60, Qualifying: "call,visit"},

		LoserSoftDeleted: true,
		LoserLiveOrphans: zeroOrphanCounts(),
	}
	assert.Equal(t, want, got)

	// users.self_contact_vcard_uid ("Me") followed the loser's tombstone onto
	// the keeper instead of dangling.
	var user models.User
	require.NoError(t, sc.db.First(&user, sc.user.ID).Error)
	require.NotNil(t, user.SelfContactVCardUID)
	assert.Equal(t, sc.ada.VCardUID, *user.SelfContactVCardUID)

	// The audit trail: the merge note on the keeper must name the merge and
	// report every count the repoint actually moved — a repoint that silently
	// stops moving an association type changes this note and fails the test.
	audit := captureAuditNote(t, sc.db, sc.user.ID, sc.ada.ID)
	assert.Contains(t, audit, fmt.Sprintf("Merged contact #%d (Bob Smith) into this record.", sc.bob.ID))
	assert.Contains(t, audit, `First Name: kept "Ada" (merged contact had "Bob")`)
	assert.Contains(t, audit, `Nickname: took "Bobby" from the merged contact (was "The Enchantress of Numbers")`)
	assert.Contains(t, audit, `Stay-in-touch cadence: took "Every 60 days (call, visit)" from the merged contact (was "Every 30 days")`)
	assert.Contains(t, audit, "Re-pointed: 1 notes, 1 activities, 1 reminders, 1 reminder completions, "+
		"3 relationship edges (4 dropped as duplicate/self-loop), 1 household memberships, "+
		"1 circle memberships, 1 tags, 1 life events (0 references), 1 custom field values, "+
		"1 attachments, 1 preferences, 1 external identities, 1 external activities, 1 cadence policies.")
	assert.Contains(t, audit, "1 CardDAV sync link(s) on the merged contact were discarded (not re-pointed).")
}

// fixtureNames maps every fixture contact's VCardUID to its manifest name so
// the golden's relationship/related-entity tuples read "ada|celine" rather
// than a 36-char UUID — the readable form is what makes a dropped or inverted
// edge diff as a named failure instead of an opaque string.
func fixtureNames(ds *canonicalfixture.Dataset) map[string]string {
	names := make(map[string]string, len(ds.Contacts))
	for name, c := range ds.Contacts {
		names[c.VCardUID] = name
	}
	return names
}

// captureAuditNote finds the merge audit note on the keeper.
func captureAuditNote(t *testing.T, db *gorm.DB, userID, keeperID uint) string {
	t.Helper()
	var note models.Note
	require.NoError(t, db.Unscoped().
		Where("contact_id = ? AND user_id = ? AND content LIKE ?", keeperID, userID, "%Merged contact #% into this record.%").
		First(&note).Error)
	return note.Content
}

// TestContactMerge_Golden_DuplicatePairDedupAndNearDedup is the union-family
// golden: the fixture's deliberate duplicate pair (hugo/ida share email,
// phone, name) is merged with bespoke *near*-duplicates added — entries that
// differ only in a dimension the dedup key does NOT include must not collapse,
// while exact duplicates must. Also pins: the spouse edge self-loop, the
// related-entity self-reference removal on the keeper's own life event, and
// the loser-side soft-deleted rows (note + activity) that must survive the
// merge as tombstones.
func TestContactMerge_Golden_DuplicatePairDedupAndNearDedup(t *testing.T) {
	db := dbtest.New(t)
	closeTestDBAtTeardown(t, db)

	m, err := canonicalfixture.Read()
	require.NoError(t, err)
	ds, err := canonicalfixture.Populate(db, m)
	require.NoError(t, err)

	ida := ds.Contacts["ida"]
	hugo := ds.Contacts["hugo"]

	// Near-duplicates that MUST NOT collapse, one per union key dimension:
	//   - email: same value, different type (unionEmails dedups on type|value)
	//   - phone: same number, different type (unionPhones dedups on type|key)
	//   - address: identical tuple except apartment (unionAddresses dedups on
	//     the whole tuple)
	// Plus an exact duplicate URL that MUST collapse (unionURLs: type|value).
	var idaRel, hugoRel models.Contact
	require.NoError(t, db.First(&idaRel, ida.ID).Error)
	require.NoError(t, db.First(&hugoRel, hugo.ID).Error)
	idaRel.Emails = append(idaRel.Emails, models.ContactEmail{Type: "work", Value: "hugo.smith@example.com"})
	idaRel.Phones = append(idaRel.Phones, models.ContactPhone{Type: "work", Value: "+1 555 0100"})
	idaRel.URLs = append(idaRel.URLs, models.ContactURL{Type: "home", Value: "https://example.com"})
	require.NoError(t, db.Save(&idaRel).Error)
	hugoRel.URLs = append(hugoRel.URLs, models.ContactURL{Type: "home", Value: "https://example.com"})
	hugoRel.Addresses = append(hugoRel.Addresses, models.ContactAddress{
		Type: "home", Street: "14 Garden Lane", Apartment: "Apt 2",
		City: "Springfield", Region: "IL", Postal: "62701", Country: "USA",
	})
	require.NoError(t, db.Save(&hugoRel).Error)

	router := goldenMergeRouter(t, db, ds.User.ID)

	// Preview: pin the union functions at the resolution level first.
	previewResp := doGoldenMerge(t, router, "/contacts/merge/preview",
		models.ContactMergeRequest{KeepID: ida.ID, MergeID: hugo.ID})
	require.Equal(t, http.StatusOK, previewResp.Code, previewResp.Body.String())
	var preview models.ContactMergePreviewResponse
	require.NoError(t, json.Unmarshal(previewResp.Body.Bytes(), &preview))
	assert.Equal(t, 2, len(preview.Resolution.Emails), "same-value different-type emails must BOTH survive")
	assert.Equal(t, 2, len(preview.Resolution.Phones), "same-number different-type phones must BOTH survive")
	assert.Equal(t, 2, len(preview.Resolution.Addresses), "addresses differing only in apartment must BOTH survive")
	assert.Equal(t, 1, len(preview.Resolution.URLs), "the exact duplicate URL must collapse to one")

	// Commit: only firstname and gender conflict.
	commitResp := doGoldenMerge(t, router, "/contacts/merge",
		models.ContactMergeRequest{
			KeepID: ida.ID, MergeID: hugo.ID,
			Resolutions: map[string]string{"firstname": "Ida", "gender": "female"},
		})
	require.Equal(t, http.StatusOK, commitResp.Code, commitResp.Body.String())

	got := captureGoldenMergeState(t, db, ds.User.ID, ida, hugo, fixtureNames(ds))

	want := goldenMergeState{
		Firstname: "Ida", Lastname: "Smith",
		Gender:   "female",
		Birthday: "1984-07-04",                  // only Hugo had one: auto-resolved
		HowWeMet: "Met at a friend's barbecue.", // only Hugo had one: auto-resolved
		Circles:  []string{},

		Emails: []string{
			"home|hugo.smith@example.com",
			"work|hugo.smith@example.com",
		},
		Phones: []string{
			"cell|+1 555 0100",
			"work|+1 555 0100",
		},
		Addresses: []string{
			"home|14 Garden Lane, Apt 2, Springfield, IL, 62701, USA",
			"home|14 Garden Lane, Springfield, IL, 62701, USA",
		},
		URLs:  []string{"home|https://example.com"},
		IMPPs: []string{},

		Notes:      []string{}, // Hugo's only note is soft-deleted: it stays with his tombstone, not repointed
		Activities: []string{"Birthday brunch", "[deleted] Disagreement about parking"},

		// The full user graph after the merge: the spouse self-loop is gone,
		// the mentor edge (secret) repointed from Hugo onto Ida, and every
		// edge that never involved the pair is untouched. The tuples pin the
		// direction rule — the stored edge is always the source's role
		// relative to the target, never inverted.
		RelationshipEdges: []string{
			"ada|bob|coworker_of|confirmed|normal|1",
			"ada|celine|friend_of|confirmed|normal|1",
			"ada|eve|parent_of|confirmed|normal|1",
			"ada|frank|owns|confirmed|normal|1",
			"ada|ida|mentor_of|confirmed|secret|1",
			"bob|celine|friend_of|confirmed|normal|1",
			"bob|eve|parent_of|suggested|normal|0.6",
		},
		Households:        []string{"Garden Flat"},
		CircleMemberships: []string{"book-club"},
		Tags:              []string{"volunteer"},
		LifeEvents: []string{
			"bought_a_home|14 Garden Lane, Springfield.|[]",
			"got_engaged|Proposed at Christmas dinner.|[]", // Hugo was the related entity; self-reference removed
		},
		Gifts:               []string{"Watch|purchased|anniversary|25000|EUR"},
		FieldValues:         map[string]string{"dietary_restrictions": "vegan"},
		Reminders:           []string{},
		ReminderCompletions: []string{},
		ConversationAgenda:  []string{},
		Attachments:         []string{},
		Preferences:         []string{},
		ExternalIdentities:  []string{},
		ExternalActivities:  []string{},
		ReachOutSuggestions: []string{},

		LoserSoftDeleted: true,
		LoserLiveOrphans: zeroOrphanCounts(),
	}
	assert.Equal(t, want, got)

	// The loser's soft-deleted note stays with the tombstone (recoverable
	// with it), not silently repointed and not destroyed.
	var loserNotes []models.Note
	require.NoError(t, db.Unscoped().Where("contact_id = ? AND user_id = ?", hugo.ID, ds.User.ID).Find(&loserNotes).Error)
	require.Len(t, loserNotes, 1)
	assert.Equal(t, "Old note: Hugo used to have a +1 555 0199 number. No longer valid.", loserNotes[0].Content)
	assert.True(t, loserNotes[0].DeletedAt.Valid, "the loser's tombstoned note must remain a tombstone")

	// Audit note: Hugo's merged-in activity set is two (Birthday brunch plus
	// the soft-deleted parking disagreement, which the raw join counts even
	// though the activity is tombstoned).
	audit := captureAuditNote(t, db, ds.User.ID, ida.ID)
	assert.Contains(t, audit, fmt.Sprintf("Merged contact #%d (Hugo Smith) into this record.", hugo.ID))
	assert.Contains(t, audit, "Re-pointed: 0 notes, 2 activities, 0 reminders, 0 reminder completions, "+
		"2 relationship edges (1 dropped as duplicate/self-loop), 1 household memberships, "+
		"1 circle memberships, 1 tags, 1 life events (1 references), 1 custom field values, "+
		"0 attachments, 0 preferences, 0 external identities, 0 external activities, 0 cadence policies.")
}

// TestContactMerge_Golden_ReciprocalEdgeDirection pins the direction rule
// (#433 recommended action 5): RelationshipEdge.Type describes the *source's*
// role relative to the target, and only one direction is ever stored. A
// reciprocal pair — ada->eve parent_of (confirmed) and eve->ada child_of
// (suggested) — must collapse to exactly one row, the higher-authority
// confirmed parent_of with source=ada, never its inverse.
func TestContactMerge_Golden_ReciprocalEdgeDirection(t *testing.T) {
	db := dbtest.New(t)
	closeTestDBAtTeardown(t, db)

	m, err := canonicalfixture.Read()
	require.NoError(t, err)
	ds, err := canonicalfixture.Populate(db, m)
	require.NoError(t, err)

	ada := ds.Contacts["ada"]
	eve := ds.Contacts["eve"]
	dmitri := ds.Contacts["dmitri"]

	// The reciprocal edge the fixture deliberately does not store.
	require.NoError(t, db.Create(&models.RelationshipEdge{
		UserID: ds.User.ID, SourceID: eve.VCardUID, TargetID: ada.VCardUID, Type: "child_of",
		Source: models.RelationshipSourceHouseholdInferred, Confidence: 0.5,
		Status: models.RelationshipStatusSuggested, Sensitivity: models.RelationshipSensitivityNormal,
	}).Error)

	router := goldenMergeRouter(t, db, ds.User.ID)
	commitResp := doGoldenMerge(t, router, "/contacts/merge",
		models.ContactMergeRequest{KeepID: ada.ID, MergeID: dmitri.ID, Resolutions: map[string]string{"firstname": "Ada"}})
	require.Equal(t, http.StatusOK, commitResp.Code, commitResp.Body.String())

	// Exactly one edge may span ada<->eve, and it must be the confirmed
	// parent_of FROM ada, never the suggested child_of FROM eve.
	var pair []models.RelationshipEdge
	require.NoError(t, db.Where(
		"user_id = ? AND ((source_id = ? AND target_id = ?) OR (source_id = ? AND target_id = ?))",
		ds.User.ID, ada.VCardUID, eve.VCardUID, eve.VCardUID, ada.VCardUID,
	).Find(&pair).Error)
	require.Len(t, pair, 1, "the inverse pair must collapse to exactly one edge")
	assert.Equal(t, ada.VCardUID, pair[0].SourceID)
	assert.Equal(t, eve.VCardUID, pair[0].TargetID)
	assert.Equal(t, "parent_of", pair[0].Type)
	assert.Equal(t, models.RelationshipStatusConfirmed, pair[0].Status)
	assert.Equal(t, models.RelationshipSourceUserConfirmed, pair[0].Source)
	assert.Equal(t, 1.0, pair[0].Confidence)

	// No self-loops survive.
	var selfLoops int64
	require.NoError(t, db.Model(&models.RelationshipEdge{}).
		Where("user_id = ? AND source_id = ? AND target_id = ?", ds.User.ID, ada.VCardUID, ada.VCardUID).
		Count(&selfLoops).Error)
	assert.Zero(t, selfLoops)

	// The loser (dmitri) is tombstoned, not gone.
	var live, unscoped int64
	require.NoError(t, db.Model(&models.Contact{}).Where("id = ?", dmitri.ID).Count(&live).Error)
	require.NoError(t, db.Unscoped().Model(&models.Contact{}).Where("id = ?", dmitri.ID).Count(&unscoped).Error)
	assert.Zero(t, live)
	assert.EqualValues(t, 1, unscoped)
}

// TestContactMerge_Golden_Idempotency is the issue's "merge twice" check: a
// second commit of the same pair must be rejected (the loser is already
// tombstoned) and must leave the state byte-for-byte unchanged — no
// compounding, no duplicate rows from a re-run.
func TestContactMerge_Golden_Idempotency(t *testing.T) {
	sc := setupRichPairScenario(t)

	commitResp := doGoldenMerge(t, sc.router, "/contacts/merge",
		models.ContactMergeRequest{KeepID: sc.ada.ID, MergeID: sc.bob.ID, Resolutions: sc.resolutions})
	require.Equal(t, http.StatusOK, commitResp.Code, commitResp.Body.String())

	first := captureGoldenMergeState(t, sc.db, sc.user.ID, sc.ada, sc.bob, fixtureNames(sc.ds))
	assert.True(t, first.LoserSoftDeleted, "the loser must be tombstoned after a successful merge")

	again := doGoldenMerge(t, sc.router, "/contacts/merge",
		models.ContactMergeRequest{KeepID: sc.ada.ID, MergeID: sc.bob.ID, Resolutions: sc.resolutions})
	require.Equal(t, http.StatusNotFound, again.Code, again.Body.String())

	second := captureGoldenMergeState(t, sc.db, sc.user.ID, sc.ada, sc.bob, fixtureNames(sc.ds))
	assert.Equal(t, first, second, "a rejected second merge must not compound or duplicate any state")
}
