package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/scoring"
	"mycorrhizal/logger"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// target_behind reach-out suggestions (issue #1796, ADR 0023 amendment
// 2026-10-16). A state-driven second pass of the reach_out_detection job: a
// contact whose target-closeness status is `behind` gets one nudge per gap.
// ---------------------------------------------------------------------------

// targetBehindKey identifies one gap: the target tier plus the last qualifying
// interaction's timestamp (RFC3339, UTC). A new interaction or a tier change
// produces a new key and therefore a new gap.
type targetBehindKey struct {
	tier string
	last string
}

// DismissPendingTargetBehindSuggestions dismisses every pending target_behind
// suggestion for one contact. Used when the target is cleared so a stale nudge
// never outlives the goal. Does not touch the companion Reminder.
func DismissPendingTargetBehindSuggestions(db *gorm.DB, userID uint, contactVCardUID string) error {
	err := db.Model(&models.ReachOutSuggestion{}).
		Where("user_id = ? AND contact_vcard_uid = ? AND kind = ? AND status = ?",
			userID, contactVCardUID, models.ReachOutKindTargetBehind, models.ReachOutStatusPending).
		Update("status", models.ReachOutStatusDismissed).Error
	if err != nil {
		return fmt.Errorf("dismissing target_behind suggestions: %w", err)
	}
	return nil
}

// detectTargetBehindSuggestions is the job's second pass. It evaluates every
// user who has a targeted contact (or a still-pending target_behind row to
// reconcile), creates suggestions per the ADR's rules 1-6, and auto-dismisses
// pending rows whose gap no longer holds. All queries are bulk per user.
// Returns the number of suggestions created.
func detectTargetBehindSuggestions(ctx context.Context, db *gorm.DB, cfg config.Config, now time.Time) (int, error) {
	var withTargets, withPending []uint
	if err := db.Model(&models.Contact{}).
		Where("target_closeness IS NOT NULL").
		Distinct().Pluck("user_id", &withTargets).Error; err != nil {
		return 0, fmt.Errorf("loading users with target closeness: %w", err)
	}
	if err := db.Model(&models.ReachOutSuggestion{}).
		Where("kind = ? AND status = ?", models.ReachOutKindTargetBehind, models.ReachOutStatusPending).
		Distinct().Pluck("user_id", &withPending).Error; err != nil {
		return 0, fmt.Errorf("loading users with pending target suggestions: %w", err)
	}
	seen := make(map[uint]bool, len(withTargets)+len(withPending))
	var userIDs []uint
	for _, id := range append(withTargets, withPending...) {
		if !seen[id] {
			seen[id] = true
			userIDs = append(userIDs, id)
		}
	}

	created := 0
	var errs []error
	for _, userID := range userIDs {
		n, err := detectTargetBehindForUser(ctx, db, cfg, userID, now)
		created += n
		if err != nil {
			logger.Error().Err(err).Uint("user_id", userID).Msg("reach-out: target pass failed for user")
			errs = append(errs, err)
		}
	}
	return created, errors.Join(errs...)
}

func detectTargetBehindForUser(ctx context.Context, db *gorm.DB, cfg config.Config, userID uint, now time.Time) (int, error) {
	var contacts []models.Contact
	if err := ExcludeSelfContact(
		db.Where("user_id = ? AND archived = ? AND target_closeness IS NOT NULL", userID, false),
		userID,
	).Find(&contacts).Error; err != nil {
		return 0, fmt.Errorf("loading targeted contacts: %w", err)
	}

	type candidate struct {
		contact models.Contact
		key     targetBehindKey
		last    time.Time
	}
	current := make(map[string]targetBehindKey)
	var candidates []candidate
	if len(contacts) > 0 {
		in, err := gatherScoreInputs(db, userID, now, contacts)
		if err != nil {
			return 0, err
		}
		for _, c := range contacts {
			raw := in.raw[c.ID]
			last := in.lastInteraction[c.VCardUID]
			switch {
			case c.Card.IsDeceased(), in.hasPolicy[c.VCardUID], raw.Ambient, last == nil:
				continue
			}
			target := scoring.Compute(raw, in.cfg).Target
			if target == nil || target.Status != scoring.TargetStatusBehind {
				continue
			}
			key := targetBehindKey{tier: target.Tier, last: last.UTC().Format(time.RFC3339)}
			current[c.VCardUID] = key
			candidates = append(candidates, candidate{contact: c, key: key, last: *last})
		}
	}

	var existing []models.ReachOutSuggestion
	if err := db.Where("user_id = ? AND kind = ?", userID, models.ReachOutKindTargetBehind).
		Find(&existing).Error; err != nil {
		return 0, fmt.Errorf("loading target_behind suggestions: %w", err)
	}
	have := make(map[string]map[targetBehindKey]bool)
	var stale []string
	for _, s := range existing {
		k := targetBehindKey{tier: s.OldValue, last: s.NewValue}
		if have[s.ContactVCardUID] == nil {
			have[s.ContactVCardUID] = make(map[targetBehindKey]bool)
		}
		have[s.ContactVCardUID][k] = true
		if s.Status != models.ReachOutStatusPending {
			continue
		}
		if cur, ok := current[s.ContactVCardUID]; !ok || cur != k {
			stale = append(stale, s.ID)
		}
	}

	created := 0
	for _, cand := range candidates {
		if have[cand.contact.VCardUID][cand.key] {
			continue
		}
		ch := detectedChange{
			Kind:     models.ReachOutKindTargetBehind,
			OldValue: cand.key.tier,
			NewValue: cand.key.last,
			// The last-interaction date is a calendar day (ADR 0015 category
			// 2): use its UTC date, never the reminder-zone conversion that
			// pulls a west-of-UTC activity a day early (issue #1823).
			ReminderMessage: fmt.Sprintf("Reach out — behind your %s target: %s (last interaction %s)",
				cand.key.tier, displayContactName(cand.contact), interactionCalendarDay(cand.last).Format("2006-01-02")),
		}
		if err := createReachOutSuggestion(ctx, db, cfg, userID, cand.contact, 0, ch); err != nil {
			return created, err
		}
		created++
	}

	if len(stale) > 0 {
		if err := db.Model(&models.ReachOutSuggestion{}).
			Where("user_id = ? AND id IN ?", userID, stale).
			Update("status", models.ReachOutStatusDismissed).Error; err != nil {
			return created, fmt.Errorf("auto-dismissing target_behind suggestions: %w", err)
		}
	}
	return created, nil
}
