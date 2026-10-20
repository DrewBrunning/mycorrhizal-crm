package services

import (
	"regexp"
	"strings"
	"time"

	"mycorrhizal/internal/scoring"

	"gorm.io/gorm"
)

// Ambient contacts (issue #1733, ADR 0023 amendment): a contact is ambient --
// counts as in regular contact without logged interactions -- when either
//
//   - they are co-resident with the user: the user's self-contact and the
//     contact are both CURRENT members of one household (scoring.
//     AmbientReasonHousehold), derived here at read time, nothing stored; or
//   - the user marked them with the per-contact ambient flag (scoring.
//     AmbientReasonFlag), stored in contacts.ambient.
//
// Co-residence wins the reported reason when both hold. A user with no
// self-contact set has no co-residents (there is nobody to live with), but the
// flag still applies.

// ambientTenureCurrent reports whether a household membership with the given
// since/until partial dates (YYYY, YYYY-MM or YYYY-MM-DD; "" = open) is current
// on today (a YYYY-MM-DD wall-clock date, ADR 0015). Partial dates compare by
// the prefix they specify, so until "2026-10" is still current on 2026-10-20
// (through the end of that month) and since "2026-11" has not started yet.
//
// household_members.since/until are not format-validated on write, so a bound
// that is not a leading YYYY[-MM[-DD]] (free text, a year-less "--MM-DD") cannot
// be placed on the calendar: the membership is then NOT current. Failing closed
// is the safe side here, since ambient suppresses overdue nudges -- a wrongly
// non-ambient contact just scores from logged activity, as before #1733. A
// trailing time ("2026-10-20T00:00:00Z") is tolerated and ignored.
func ambientTenureCurrent(since, until, today string) bool {
	if since != "" {
		s, ok := tenureDatePrefix(since)
		if !ok || !partialDateNotAfter(s, today) {
			return false
		}
	}
	if until != "" {
		u, ok := tenureDatePrefix(until)
		if !ok || !partialDateNotAfter(today, u) {
			return false
		}
	}
	return true
}

// tenureDateRE matches the calendar part of a tenure bound: a zero-padded ISO
// partial date (YYYY, YYYY-MM or YYYY-MM-DD) at the start of the value.
var tenureDateRE = regexp.MustCompile(`^\d{4}(-\d{2}(-\d{2})?)?`)

// tenureDatePrefix returns the leading ISO partial date of a tenure bound, or
// ok=false when the value does not start with one.
func tenureDatePrefix(v string) (string, bool) {
	m := tenureDateRE.FindString(strings.TrimSpace(v))
	return m, m != ""
}

// partialDateNotAfter reports whether a <= b, comparing only the precision
// the shorter operand specifies (both are zero-padded ISO partial dates, so a
// prefix string compare is a calendar compare).
func partialDateNotAfter(a, b string) bool {
	n := min(len(a), len(b))
	return a[:n] <= b[:n]
}

// ambientByEntityID returns, for the given contact vCard UIDs, the ambient
// reason (scoring.AmbientReasonHousehold or scoring.AmbientReasonFlag) of each
// ambient one; non-ambient contacts are absent from the map. It is ONE bulk
// query regardless of len(uids): contacts left-joined to the household rows
// they share with the user's self-contact (read from users in a subquery, so
// no separate self-contact lookup, and a user with none simply matches
// nothing), keeping only flagged contacts or ones with a shared household. The
// "current" test on the partial-date tenures is applied in Go.
func ambientByEntityID(db *gorm.DB, userID uint, uids []string, now time.Time) (map[string]string, error) {
	result := make(map[string]string)
	if len(uids) == 0 {
		return result, nil
	}

	type ambientRow struct {
		VCardUID  string `gorm:"column:vcard_uid"`
		Flag      bool   `gorm:"column:flag"`
		SharedRow *uint  `gorm:"column:shared_row"` // hs.id; nil unless the self-contact shares this household
		Since     string `gorm:"column:since"`      // COALESCEd: the columns are nullable TEXT
		Until     string `gorm:"column:until"`
		SelfSince string `gorm:"column:self_since"`
		SelfUntil string `gorm:"column:self_until"`
	}
	const selfUID = "(SELECT self_contact_vcard_uid FROM users WHERE id = ?)"
	var rows []ambientRow
	if err := db.Table("contacts c").
		Select("c.vcard_uid AS vcard_uid, c.ambient AS flag, hs.id AS shared_row, "+
			"COALESCE(hm.since, '') AS since, COALESCE(hm.until, '') AS until, "+
			"COALESCE(hs.since, '') AS self_since, COALESCE(hs.until, '') AS self_until").
		Joins("LEFT JOIN household_members hm ON hm.member_vcard_uid = c.vcard_uid AND hm.user_id = ?", userID).
		Joins("LEFT JOIN household_members hs ON hs.household_id = hm.household_id AND hs.id <> hm.id "+
			"AND hs.user_id = ? AND hs.member_vcard_uid = "+selfUID, userID, userID).
		Where("c.user_id = ? AND c.vcard_uid IN ? AND c.deleted_at IS NULL AND c.vcard_uid <> COALESCE("+selfUID+", '') "+
			"AND (c.ambient = ? OR hs.id IS NOT NULL)", userID, uids, userID, true).
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	today := now.Format("2006-01-02")
	for _, r := range rows {
		if r.SharedRow != nil &&
			ambientTenureCurrent(r.Since, r.Until, today) &&
			ambientTenureCurrent(r.SelfSince, r.SelfUntil, today) {
			result[r.VCardUID] = scoring.AmbientReasonHousehold // co-residence wins the reason
			continue
		}
		if r.Flag {
			if _, done := result[r.VCardUID]; !done {
				result[r.VCardUID] = scoring.AmbientReasonFlag
			}
		}
	}
	return result, nil
}

// ambientForUIDs is ambientByEntityID under the name the cadence callers use.
func ambientForUIDs(db *gorm.DB, userID uint, uids []string, now time.Time) (map[string]string, error) {
	return ambientByEntityID(db, userID, uids, now)
}
