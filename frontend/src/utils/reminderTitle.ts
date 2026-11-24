import type { TFunction } from 'i18next';
import type { Reminder } from '../api/reminders';

// Issue #2915: a reminder materialized from an occasion obligation is
// machine-synthesized, and used to carry an English "Occasion — " prefix in
// its stored message. New rows store "label: name" and the views add the
// localized title, so strip a legacy prefix to avoid rendering it twice.
const LEGACY_OCCASION_PREFIX = 'Occasion — ';

// Every reminder view renders its title through here. Only reminders linked
// to an occasion get the occasion title; a user's own reminder is shown
// exactly as stored, even if it happens to start with the same words.
export function reminderTitle(t: TFunction, reminder: Reminder): string {
  if (!reminder.occasion_obligation_id) return reminder.message;
  const stored = reminder.message.startsWith(LEGACY_OCCASION_PREFIX)
    ? reminder.message.slice(LEGACY_OCCASION_PREFIX.length)
    : reminder.message;
  return t('reminders.occasionTitle', { title: stored });
}
