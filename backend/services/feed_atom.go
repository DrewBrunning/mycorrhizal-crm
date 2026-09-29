package services

import (
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/i18n"
	"mycorrhizal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// feedEntryNamespace is the fixed UUIDv5 namespace entry ids are derived from
// (ADR 0030 decision 2). Changing it changes every entry id, which readers
// treat as a new entry, so it is a constant and never a per-instance value.
var feedEntryNamespace = uuid.MustParse("f3a1c2d4-5b6e-4f70-8a91-0c1d2e3f4a5b")

// feedMaxEntries bounds a feed's window (ADR 0030 decision 3): the 50 most
// recent entries by timeline date, descending, with no paging.
const feedMaxEntries = 50

// AtomRenderInput is everything RenderAtomFeed needs. Contact is nil for an
// aggregate feed. Now is the request time; it is used only to exclude
// future-dated entries and resolve yearless life-event dates — it never
// appears in the output, so rendering is deterministic for stable data.
type AtomRenderInput struct {
	Feed    *models.Feed
	User    *models.User
	Contact *models.Contact
	Now     time.Time
}

// Atom 1.0 structs (RFC 4287). Plain encoding/xml marshalling, no new
// dependency; every text construct is type="text" so a reader is never told
// to render markup (ADR 0030 decision 2).
type atomFeed struct {
	XMLName   xml.Name    `xml:"http://www.w3.org/2005/Atom feed"`
	ID        string      `xml:"id"`
	Title     atomText    `xml:"title"`
	Updated   string      `xml:"updated"`
	Generator string      `xml:"generator"`
	Entries   []atomEntry `xml:"entry"`
}

type atomText struct {
	Type string `xml:"type,attr"`
	Body string `xml:",chardata"`
}

type atomLink struct {
	Rel  string `xml:"rel,attr"`
	Href string `xml:"href,attr"`
}

type atomEntry struct {
	ID        string    `xml:"id"`
	Title     atomText  `xml:"title"`
	Published string    `xml:"published"`
	Updated   string    `xml:"updated"`
	Link      *atomLink `xml:"link,omitempty"`
	Content   *atomText `xml:"content,omitempty"`
}

// RenderAtomFeed composes the feed's timeline window and renders it as Atom
// 1.0. The output is deterministic: the same stored data renders the same
// bytes (ADR 0030 decision 2), which is what makes the endpoint's strong ETag
// correct by construction.
func RenderAtomFeed(db *gorm.DB, cfg *config.Config, in AtomRenderInput) ([]byte, error) {
	if in.Feed == nil || in.User == nil {
		return nil, fmt.Errorf("feed and user are required")
	}
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}

	q := TimelineQuery{
		Limit:             feedMaxEntries,
		Desc:              true,
		NotAfter:          &now,
		Now:               now,
		FilterSensitivity: true,
	}
	if in.Contact != nil {
		id := in.Contact.ID
		q.ContactID = &id
	}

	items, _, err := ComposeTimeline(db, in.Feed.UserID, q)
	if err != nil {
		return nil, err // # pragma: no cover — DB failure only in the feed render path
	}

	resolver, err := newFeedContactResolver(db, in, items)
	if err != nil {
		return nil, err // # pragma: no cover — DB failure only in the feed render path
	}

	lang := in.User.Language
	feedID := "urn:uuid:" + in.Feed.ID
	title := feedTitle(lang, in.Feed, in.Contact)

	entries := make([]atomEntry, 0, len(items))
	var maxUpdated time.Time
	for _, item := range items {
		entry, updated, err := buildAtomEntry(cfg, lang, in.Feed, resolver, item)
		if err != nil {
			return nil, err // # pragma: no cover — DB failure only in the feed render path
		}
		if updated.After(maxUpdated) {
			maxUpdated = updated
		}
		entries = append(entries, entry)
	}

	feedUpdated := in.Feed.CreatedAt
	if !maxUpdated.IsZero() {
		feedUpdated = maxUpdated
	}

	doc := atomFeed{
		ID:        feedID,
		Title:     atomText{Type: "text", Body: title},
		Updated:   atomTimestamp(feedUpdated),
		Generator: "Mycorrhizal CRM",
		Entries:   entries,
	}

	body, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err // # pragma: no cover — DB failure only in the feed render path
	}
	return append([]byte(xml.Header), body...), nil
}

// feedTitle localizes the feed title. A contact feed names the contact; an
// aggregate feed uses the static key.
func feedTitle(lang string, feed *models.Feed, contact *models.Contact) string {
	if feed.Kind == models.FeedKindContact && contact != nil {
		return i18n.T(lang, "feeds.title.contact", map[string]string{"name": contactDisplayName(contact)})
	}
	return i18n.T(lang, "feeds.title.aggregate")
}

// buildAtomEntry renders one timeline item, returning the entry and the
// entity's updated instant (for the feed-level max).
func buildAtomEntry(cfg *config.Config, lang string, feed *models.Feed, resolver *feedContactResolver, item models.TimelineItem) (atomEntry, time.Time, error) {
	updated := feedItemUpdated(item)

	label := i18n.T(lang, "feeds.type."+item.Type)
	if ea, ok := item.Data.(models.ExternalActivity); ok {
		// external_activity's type label is qualified by its source system
		// and its payload is never read (ADR 0030 decision 4).
		if ea.SourceSystem != "" {
			label += " (" + ea.SourceSystem + ")"
		}
	}

	names := resolver.displayNames(item)
	title := label
	if len(names) > 0 {
		title = label + ": " + strings.Join(names, ", ")
	}

	entry := atomEntry{
		ID:        "urn:uuid:" + feedEntryID(item),
		Title:     atomText{Type: "text", Body: title},
		Published: atomTimestamp(item.Date),
		Updated:   atomTimestamp(updated),
	}

	if link := resolver.linkHref(cfg, item); link != "" {
		entry.Link = &atomLink{Rel: "alternate", Href: link}
	}

	if text, ok := feedEntryContent(feed.Detail, item); ok {
		entry.Content = &atomText{Type: "text", Body: text}
	}

	return entry, updated, nil
}

// feedEntryID is the stable UUIDv5 of "<type>:<id>" under feedEntryNamespace.
func feedEntryID(item models.TimelineItem) string {
	return uuid.NewSHA1(feedEntryNamespace, []byte(item.Type+":"+item.ID)).String()
}

// atomTimestamp renders an instant as RFC 3339 UTC.
func atomTimestamp(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// feedItemUpdated extracts the entity's updated instant from a timeline item.
func feedItemUpdated(item models.TimelineItem) time.Time {
	switch d := item.Data.(type) {
	case models.Note:
		return d.UpdatedAt
	case models.Activity:
		return d.UpdatedAt
	case models.ReminderCompletion:
		return d.UpdatedAt
	case models.LifeEvent:
		return d.UpdatedAt
	case models.ExternalActivity:
		return d.UpdatedAt
	case models.Gift:
		return d.UpdatedAt
	default:
		return item.Date
	}
}

// feedEntryContent returns the type="text" body for a full-detail feed, and
// ok=false for headlines or for external_activity (which never carries
// content at either level). Parts are joined with a blank line and empty
// parts are skipped.
func feedEntryContent(detail string, item models.TimelineItem) (string, bool) {
	if detail != models.FeedDetailFull {
		return "", false
	}
	var parts []string
	switch d := item.Data.(type) {
	case models.Note:
		parts = []string{d.Content}
	case models.Activity:
		parts = []string{d.Title, d.Description, d.Location}
	case models.ReminderCompletion:
		parts = []string{d.Message}
	case models.LifeEvent:
		parts = []string{d.Description}
	case models.Gift:
		parts = []string{d.Description}
	case models.ExternalActivity:
		return "", false
	default:
		return "", false
	}
	nonEmpty := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	if len(nonEmpty) == 0 {
		return "", false
	}
	return strings.Join(nonEmpty, "\n\n"), true
}

// feedContactResolver resolves the contacts named by a feed's entries. For a
// contact feed it always returns the one contact; for an aggregate feed it
// loads only the contacts referenced by the rendered window, so the feed
// never gains a second unbounded scan of the address book.
type feedContactResolver struct {
	single *models.Contact
	byID   map[uint]*models.Contact
	byUID  map[string]*models.Contact
}

func newFeedContactResolver(db *gorm.DB, in AtomRenderInput, items []models.TimelineItem) (*feedContactResolver, error) {
	if in.Contact != nil {
		return &feedContactResolver{single: in.Contact}, nil
	}

	// Collect only the contacts the rendered window references, so the
	// resolver is bounded by the feed's 50 entries rather than the whole
	// address book.
	idSet := map[uint]bool{}
	uidSet := map[string]bool{}
	for _, item := range items {
		switch d := item.Data.(type) {
		case models.Note:
			if d.ContactID != nil {
				idSet[*d.ContactID] = true
			}
		case models.ReminderCompletion:
			idSet[d.ContactID] = true
		case models.LifeEvent:
			uidSet[d.EntityID] = true
		case models.ExternalActivity:
			uidSet[d.EntityID] = true
		case models.Gift:
			uidSet[d.EntityID] = true
		}
	}
	ids := make([]uint, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	uids := make([]string, 0, len(uidSet))
	for uid := range uidSet {
		uids = append(uids, uid)
	}

	r := &feedContactResolver{
		byID:  make(map[uint]*models.Contact, len(ids)),
		byUID: make(map[string]*models.Contact, len(uids)),
	}
	var contacts []models.Contact
	if len(ids) > 0 {
		if err := db.Where("user_id = ? AND id IN ?", in.User.ID, ids).Find(&contacts).Error; err != nil {
			return nil, err // # pragma: no cover — DB failure only in the contact resolver
		}
	}
	if len(uids) > 0 {
		var byUID []models.Contact
		if err := db.Where("user_id = ? AND vcard_uid IN ?", in.User.ID, uids).Find(&byUID).Error; err != nil {
			return nil, err // # pragma: no cover — DB failure only in the contact resolver
		}
		contacts = append(contacts, byUID...)
	}
	for i := range contacts {
		c := &contacts[i]
		r.byID[c.ID] = c
		r.byUID[c.VCardUID] = c
	}
	return r, nil
}

// displayNames returns the contact names an entry should carry, deduplicated
// and in a stable order.
func (r *feedContactResolver) displayNames(item models.TimelineItem) []string {
	if r.single != nil {
		return []string{contactDisplayName(r.single)}
	}
	var contacts []*models.Contact
	switch d := item.Data.(type) {
	case models.Note:
		contacts = append(contacts, r.lookupID(d.ContactID))
	case models.ReminderCompletion:
		id := d.ContactID
		contacts = append(contacts, r.lookupID(&id))
	case models.Activity:
		for i := range d.Contacts {
			c := d.Contacts[i]
			contacts = append(contacts, &c)
		}
	case models.LifeEvent:
		contacts = append(contacts, r.byUID[d.EntityID])
	case models.ExternalActivity:
		contacts = append(contacts, r.byUID[d.EntityID])
	case models.Gift:
		contacts = append(contacts, r.byUID[d.EntityID])
	}

	seen := make(map[uint]bool, len(contacts))
	names := make([]string, 0, len(contacts))
	for _, c := range contacts {
		if c == nil || seen[c.ID] {
			continue
		}
		seen[c.ID] = true
		names = append(names, contactDisplayName(c))
	}
	return names
}

// linkHref returns the entry's alternate link (the first involved contact's
// page), or "" when FrontendURL is the dev sentinel and no absolute URL
// exists.
func (r *feedContactResolver) linkHref(cfg *config.Config, item models.TimelineItem) string {
	if cfg.FrontendURL == "*" || cfg.FrontendURL == "" {
		return ""
	}
	var contact *models.Contact
	if r.single != nil {
		contact = r.single
	} else {
		switch d := item.Data.(type) {
		case models.Note:
			contact = r.lookupID(d.ContactID)
		case models.ReminderCompletion:
			id := d.ContactID
			contact = r.lookupID(&id)
		case models.Activity:
			if len(d.Contacts) > 0 {
				c := d.Contacts[0]
				contact = &c
			}
		case models.LifeEvent:
			contact = r.byUID[d.EntityID]
		case models.ExternalActivity:
			contact = r.byUID[d.EntityID]
		case models.Gift:
			contact = r.byUID[d.EntityID]
		}
	}
	if contact == nil {
		return ""
	}
	return fmt.Sprintf("%s/contacts/%d", strings.TrimRight(cfg.FrontendURL, "/"), contact.ID)
}

func (r *feedContactResolver) lookupID(id *uint) *models.Contact {
	if id == nil {
		return nil
	}
	return r.byID[*id]
}
