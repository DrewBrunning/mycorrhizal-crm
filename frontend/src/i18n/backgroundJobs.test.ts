import { expect, test } from 'vitest';
import de from './locales/de.json';
import en from './locales/en.json';
import es from './locales/es.json';
import fr from './locales/fr.json';
import it from './locales/it.json';

// Keep in sync with backend models/job_run.go's KnownJobNames (the API returns
// one health row per entry, so every one can reach the Background jobs panel).
const KNOWN_JOB_KEYS = [
  'daily_reminders',
  'webhook_retries',
  'calendar_sync',
  'purge_deleted',
  'audit_purge',
  'system_event_purge',
  'webhook_delivery_purge',
  'idempotency_key_purge',
  'session_purge',
  'cadence_overdue',
  'reach_out_detection',
  'immich_sync',
  'db_integrity_check',
  'restore_drill',
  'alert_eval',
  'job_run_purge',
  'storage_sample',
  'search_index_rebuild',
  'derived_columns_rebuild',
] as const;

const LOCALES = { en, de, es, fr, it };

test('the English jobs map covers exactly the known job keys', () => {
  expect(Object.keys(en.backgroundJobs.jobs).sort()).toEqual([...KNOWN_JOB_KEYS].sort());
});

for (const [locale, messages] of Object.entries(LOCALES)) {
  test(`${locale} has a real translated label for every known job key`, () => {
    for (const key of KNOWN_JOB_KEYS) {
      const label = (messages.backgroundJobs.jobs as Record<string, string>)[key];
      expect(label, `${locale} is missing backgroundJobs.jobs.${key}`).toBeTruthy();
      expect(label, `${locale} still shows the raw key for ${key}`).not.toBe(key);
      // The UI must never expose snake_case job keys as labels.
      expect(label).not.toMatch(/_/);
    }
  });
}
