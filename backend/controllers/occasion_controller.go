package controllers

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"mycorrhizal/contactmodel"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/internal/clock"
	"mycorrhizal/logger"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetUpcomingOccasions is the "upcoming occasions" dashboard-widget aggregate
// (ADR 0024, issue #387, ticket #1224): every birthday/anniversary/life-event/
// active-obligation occurrence within the next `days` (30 or 90, default 30),
// sorted ascending by days-until.
//
// "Today" is decided via viewerNow(c): the viewer's zone when ?tz is sent,
// otherwise the single reminder-zone clock (ADR 0015 Rule 4) — never the
// server's own local zone.
func GetUpcomingOccasions(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	days := 30
	if raw := c.Query("days"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || (parsed != 30 && parsed != 90) {
			apperrors.AbortWithError(c, apperrors.ErrValidation("days must be 30 or 90").WithReason(apperrors.ReasonOccasionDaysInvalid, nil))
			return
		}
		days = parsed
	}
	includeSensitive := c.Query("include_sensitive") == "true"

	now, ok := viewerNow(c)
	if !ok {
		return
	}
	occasions, err := services.GetUpcomingOccasions(db, userID, now, days, includeSensitive)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve upcoming occasions").WithError(err))
		return
	}
	// CLAUDE.md frontend trap #8: never let an empty result serialize as an
	// absent key — a nil slice with `omitempty` disappears from the JSON
	// entirely, which a required TS array type cannot guard against.
	if occasions == nil {
		occasions = []models.UpcomingOccasion{}
	}

	c.JSON(http.StatusOK, gin.H{"occasions": occasions, "days": days})
}

// GetOccasionCardListCSV is the sensitivity-filtered "Christmas cards"
// address-list export (ADR 0024 part 4, issue #387, ticket #1225): every
// contact with an active OccasionObligation of the given Kind (default
// "card"), with their display name and first postal address.
//
// This is deliberately NOT routed through GET /api/v1/export (the
// full-fidelity personal-backup exception, issue #861) — that endpoint
// exists precisely because it never leaves the instance in the sense that
// matters; a mailing-label export is exactly the kind of copy meant to leave
// the instance (to a printer, a card-fulfillment service) that the general
// sensitivity rule governs. OccasionObligation rows above `normal`
// sensitivity are excluded by default, the opposite of /export's default,
// re-includable only via ?include_sensitive=true.
//
// A contact with no postal address on file is skipped, not emitted with a
// blank address column.
func GetOccasionCardListCSV(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	log := logger.FromContext(c)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	kind, includeSensitive, ok := parseCardListQuery(c)
	if !ok {
		return
	}

	entries, err := resolveCardList(db, userID, kind, includeSensitive)
	if err != nil {
		abortExportError(c, log, "export:occasion-card-list", "database", "Failed to fetch occasion obligations for card list", err)
		return
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write(csvSafeRecord([]string{"Contact Name", "Address", "Obligation"})); err != nil {
		abortExportError(c, log, "export:occasion-card-list", "serialization", "Failed to write CSV header", err)
		return
	}

	for _, entry := range entries {
		if entry.skip != cardListIncluded {
			continue
		}
		if err := writer.Write(csvSafeRecord([]string{entry.name, entry.address, entry.label})); err != nil {
			abortExportError(c, log, "export:occasion-card-list", "serialization", "Failed to write CSV row", err)
			return
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		abortExportError(c, log, "export:occasion-card-list", "serialization", "CSV writer error", err)
		return
	}

	filename := fmt.Sprintf("mycorrhizal-%s-list-%s.csv", safeDownloadStem(kind), clock.FromContext(c).Now().Format("2006-01-02"))
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Length", fmt.Sprintf("%d", buf.Len()))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// cardListSkip classifies one card-list obligation: either it yields a CSV
// row (cardListIncluded) or it is skipped for exactly one reason.
type cardListSkip int

const (
	cardListIncluded cardListSkip = iota
	// cardListNoAddress: the contact is missing/not owned, has no postal
	// address, or its first exportable address is blank.
	cardListNoAddress
	// cardListPrivateAddressOnly: the contact has addresses, but every one is
	// above normal sensitivity and include_sensitive was not set.
	cardListPrivateAddressOnly
)

// cardListEntry is one active obligation resolved against its contact.
type cardListEntry struct {
	label   string
	name    string
	address string
	skip    cardListSkip
}

// parseCardListQuery reads and validates the query shared by the card-list CSV
// and its summary, so the two can never disagree about what was asked for.
// It aborts with 400 (and returns ok=false) on a malformed `kind`.
func parseCardListQuery(c *gin.Context) (kind string, includeSensitive, ok bool) {
	kind = c.DefaultQuery("kind", models.OccasionObligationKindCard)
	// `kind` is an open classifier (ADR 0024; the web dialog offers card/
	// gift/invite as free-text suggestions, and the write path validates it
	// only as max=100), so an unknown *value* is legitimate and must keep
	// working. It is still untrusted input: reject control characters
	// outright (no legitimate kind contains one), then output-encode the
	// filename via safeDownloadStem regardless. Interpolating a raw control
	// byte into the Content-Disposition filename made the response an invalid
	// HTTP message that nginx answered with 502 (issue #1250); rejecting the
	// malformed class is input validation, sanitizing the filename is the
	// output-encoding guarantee that also covers non-control unsafe bytes
	// (quotes, slashes, non-ASCII).
	if !validKindQuery(kind) {
		apperrors.AbortWithError(c, apperrors.ErrValidation("kind must not contain control characters").WithReason(apperrors.ReasonOccasionKindInvalid, nil))
		return "", false, false
	}
	return kind, c.Query("include_sensitive") == "true", true
}

// cardListContactBatch bounds one `vcard_uid IN (...)` lookup well under
// SQLite's bound-variable limit.
const cardListContactBatch = 500

// resolveCardList is the single selection logic behind both the card-list CSV
// and its summary (issue #2786): the caller's active obligations of `kind`
// (normal sensitivity only unless includeSensitive), each resolved against its
// owner-scoped contact with a batched lookup (no per-obligation query), and
// classified as included or skipped. Entries are in label order, the CSV's row
// order. Above-normal addresses are withheld unless includeSensitive (issue
// #2052), exactly as the CSV leaving the instance requires.
func resolveCardList(db *gorm.DB, userID uint, kind string, includeSensitive bool) ([]cardListEntry, error) {
	query := db.Where("user_id = ? AND active = ? AND kind = ?", userID, true, kind)
	if !includeSensitive {
		query = query.Where("sensitivity = ?", models.RelationshipSensitivityNormal)
	}
	var obligations []models.OccasionObligation
	if err := query.Order("label ASC").Find(&obligations).Error; err != nil {
		return nil, err
	}

	uids := make([]string, 0, len(obligations))
	seen := make(map[string]struct{}, len(obligations))
	for _, o := range obligations {
		if _, dup := seen[o.EntityID]; !dup {
			seen[o.EntityID] = struct{}{}
			uids = append(uids, o.EntityID)
		}
	}
	contacts := make(map[string]models.Contact, len(uids))
	for start := 0; start < len(uids); start += cardListContactBatch {
		end := start + cardListContactBatch
		if end > len(uids) {
			end = len(uids)
		}
		var batch []models.Contact
		if err := db.Where("user_id = ? AND vcard_uid IN ?", userID, uids[start:end]).Find(&batch).Error; err != nil {
			return nil, err
		}
		for _, contact := range batch {
			contacts[contact.VCardUID] = contact
		}
	}

	entries := make([]cardListEntry, 0, len(obligations))
	for _, obligation := range obligations {
		entry := cardListEntry{label: obligation.Label}
		contact, found := contacts[obligation.EntityID]
		if !found {
			// The referenced contact no longer exists or isn't owned; skip
			// rather than fail the whole export.
			entry.skip = cardListNoAddress
			entries = append(entries, entry)
			continue
		}
		// Filter the stored Addresses through the same neutral Card shape
		// ApplyFieldSelection uses, then take the first survivor.
		addresses := make([]contactmodel.Address, len(contact.Addresses))
		for i, a := range contact.Addresses {
			addresses[i] = models.AddressFromContactAddress(a)
		}
		total := len(addresses)
		if !includeSensitive {
			addresses = models.FilterSensitiveAddresses(addresses)
		}
		switch {
		case len(addresses) == 0 && total > 0:
			entry.skip = cardListPrivateAddressOnly
		case len(addresses) == 0 || addresses[0].Full == "":
			entry.skip = cardListNoAddress
		default:
			entry.address = addresses[0].Full
			entry.name = contactDisplayNameFlat(&contact)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// GetOccasionCardListSummary is the counts-only companion to the card-list CSV
// (issue #2786): how many recipients the CSV for the same query would hold and
// how many it skips, and why, so a client can say "12 recipients, 3 skipped".
// It shares resolveCardList with the CSV, so the two cannot disagree. `total`
// counts active obligations (one CSV row each), not distinct contacts. No
// names or addresses are returned.
func GetOccasionCardListSummary(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	kind, includeSensitive, ok := parseCardListQuery(c)
	if !ok {
		return
	}
	entries, err := resolveCardList(db, userID, kind, includeSensitive)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to summarize occasion card list").WithError(err))
		return
	}
	var included, noAddress, privateOnly int
	for _, entry := range entries {
		switch entry.skip {
		case cardListIncluded:
			included++
		case cardListNoAddress:
			noAddress++
		case cardListPrivateAddressOnly:
			privateOnly++
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"total":    len(entries),
		"included": included,
		"skipped":  noAddress + privateOnly,
		"skipped_reasons": gin.H{
			"no_address":           noAddress,
			"private_address_only": privateOnly,
		},
	})
}

// validKindQuery reports whether a `kind` query value is free of control
// characters. It deliberately does NOT restrict the value to the named
// card/gift/invite set: `kind` is an open classifier (models/
// occasion_obligation.go, docs/adrs/0024-occasions.md), the write path accepts
// any string up to 100 chars, and the web dialog is a free-text Autocomplete —
// so rejecting unknown values would break a supported flow. Only the malformed
// class is rejected; output encoding for the filename is safeDownloadStem's job.
func validKindQuery(kind string) bool {
	for _, r := range kind {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// safeDownloadStem reduces the user-supplied `kind` filter to characters that
// are safe inside a Content-Disposition filename.
//
// This is the output-encoding half of the fix for the 502 a Schemathesis fuzz
// run found (issue #1250): the handler interpolated `kind` raw into the
// download filename, so a value carrying a NUL/newline made Go emit an invalid
// response header, which the all-in-one image's nginx rejected with a 502
// (curl refuses the same response: "Nul byte in header"). Keep only
// [A-Za-z0-9._-]; if nothing survives, fall back to a fixed stem so the
// filename is never empty. The raw value still drives the DB filter, so custom
// kinds keep working — only the on-disk name is constrained.
func safeDownloadStem(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '_', r == '-':
			b.WriteRune(r)
		}
		if b.Len() >= 100 {
			break
		}
	}
	if b.Len() == 0 {
		return "occasions"
	}
	return b.String()
}

// contactDisplayNameFlat mirrors services.contactDisplayName exactly, kept
// local to controllers since services cannot depend on controllers and this
// handler needs it directly against a models.Contact it already loaded.
func contactDisplayNameFlat(contact *models.Contact) string {
	name := contact.Firstname
	if contact.Nickname != "" {
		name = contact.Nickname
	}
	if contact.Lastname != "" {
		name += " " + contact.Lastname
	}
	return name
}

// GetGiftShoppingList is "who still needs a gift" (ADR 0024 part 5, issue
// #387, ticket #1226): every active Kind="gift" obligation due within
// `days`, joined against Gift to show whether this cycle's gift already has
// a linked idea/purchase.
func GetGiftShoppingList(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	days := 30
	if raw := c.Query("days"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || (parsed != 30 && parsed != 90) {
			apperrors.AbortWithError(c, apperrors.ErrValidation("days must be 30 or 90").WithReason(apperrors.ReasonOccasionDaysInvalid, nil))
			return
		}
		days = parsed
	}
	includeSensitive := c.Query("include_sensitive") == "true"

	now, ok := viewerNow(c)
	if !ok {
		return
	}
	items, err := services.GetGiftShoppingList(db, userID, now, days, includeSensitive)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve gift shopping list").WithError(err))
		return
	}
	if items == nil {
		items = []models.GiftShoppingItem{}
	}

	c.JSON(http.StatusOK, gin.H{"gift_shopping_list": items, "days": days})
}
