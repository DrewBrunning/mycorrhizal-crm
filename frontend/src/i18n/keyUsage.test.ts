import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';
import { describe, expect, test } from 'vitest';
import en from './locales/en.json';

// Issue #1907: the locale parity test keeps the five locale files consistent
// with each other, but not with the code. A key removed from the UI lingers in
// all five files forever. This test collects every leaf key in en.json and
// fails when nothing under src/ references it.
//
// Keys referenced through a template literal (t(`contactScore.reasons.${code}`))
// cannot be found by a literal scan, so each dynamic prefix is listed below
// with the file that builds it. The test verifies that file still contains the
// prefix: a stale allowlist entry fails rather than silently widening the
// exemption.
//
// The source scan deliberately excludes tests, generated code and the locale
// files themselves -- a key only a test mentions is not used by the UI.

type Translations = Record<string, unknown>;

interface DynamicKeySource {
  /** Static prefix before the first `${...}` (or the whole concatenated key). */
  prefix: string;
  /** File that builds keys under this prefix. */
  file: string;
  /** When the interpolated part is a closed set, the exact values it takes. */
  values?: readonly string[];
  /** Why this cannot be a plain literal key. */
  reason: string;
}

const DYNAMIC_KEY_SOURCES: readonly DynamicKeySource[] = [
  {
    prefix: 'audit.entityTypes.',
    file: 'AuditPage.tsx',
    reason: 'entity type comes from the event payload',
  },
  {
    prefix: 'audit.operations.',
    file: 'AuditPage.tsx',
    reason: 'operation comes from the event payload',
  },
  {
    prefix: 'backgroundJobs.jobs.',
    file: 'components/BackgroundJobsPanel.tsx',
    reason: 'job name comes from the server',
  },
  {
    prefix: 'backgroundJobs.result.',
    file: 'components/BackgroundJobsPanel.tsx',
    reason: 'run result is an enum from the server',
  },
  {
    prefix: 'backgroundJobs.status.',
    file: 'components/BackgroundJobsPanel.tsx',
    reason: 'job status is an enum from the server',
  },
  {
    prefix: 'backgroundJobs.trigger.',
    file: 'components/BackgroundJobsPanel.tsx',
    reason: 'trigger is an enum from the server',
  },
  {
    prefix: 'cadence.types.',
    file: 'components/CadenceDialog.tsx',
    reason: 'interaction type tokens are a backend enum',
  },
  {
    prefix: 'contactDetail.',
    file: 'components/ContactHeader.tsx',
    values: ['animal'],
    reason: 'the only interpolated contact-kind key (kind === "animal")',
  },
  {
    prefix: 'contactScore.badge.',
    file: 'components/ContactHeader.tsx',
    reason: 'health band is an enum from the server',
  },
  {
    prefix: 'contactScore.facets.',
    file: 'components/ContactHeader.tsx',
    reason: 'facet key comes from the score breakdown',
  },
  {
    prefix: 'contactScore.reasons.',
    file: 'utils/scoreReason.ts',
    reason: 'reason code is built by key(code) from the score payload',
  },
  {
    prefix: 'contactShares.status.',
    file: 'ContactSharesPage.tsx',
    reason: 'share status is an enum from the server',
  },
  {
    prefix: 'contacts.',
    file: 'components/ContactInformation.tsx',
    values: ['male', 'female', 'non_binary', 'genderfluid', 'other', 'prefer_not_to_say'],
    reason: 'gender suggestions (GENDER_OPTIONS) are stored as free text',
  },
  {
    prefix: 'contacts.anniversaryFields.kindOptions.',
    file: 'components/AnniversaryList.tsx',
    reason: 'anniversary kind is a backend enum',
  },
  {
    prefix: 'contacts.cardKind.',
    file: 'components/ContactHeader.tsx',
    reason: 'vCard kind token is a backend enum',
  },
  {
    prefix: 'contacts.contexts.',
    file: 'components/OnlineServiceEditor.tsx',
    reason: 'vCard context tokens are a backend enum',
  },
  {
    prefix: 'contacts.import.preview.kind.',
    file: 'components/ImportContactsDialog.tsx',
    reason: 'preview row kind is derived from the imported value',
  },
  {
    prefix: 'contacts.import.steps.',
    file: 'components/ImportContactsDialog.tsx',
    reason: 'wizard step id comes from the step list',
  },
  {
    prefix: 'contacts.personalInfo.kindOptions.',
    file: 'components/PersonalInfoDialog.tsx',
    reason: 'personal-info kind is a backend enum',
  },
  {
    prefix: 'contacts.personalInfo.levelOptions.',
    file: 'components/PersonalInfoDialog.tsx',
    reason: 'personal-info level is a backend enum',
  },
  {
    prefix: 'contacts.speakToAs.gramGender.',
    file: 'components/SpeakToAsEditor.tsx',
    reason: 'grammatical gender is a backend enum',
  },
  {
    prefix: 'contacts.types.',
    file: 'components/AddressFields.tsx',
    reason: 'vCard TYPE tokens are a backend enum',
  },
  {
    prefix: 'customFields.sensitivity',
    file: 'components/CustomFieldsSettings.tsx',
    values: ['Normal', 'Private', 'Secret'],
    reason: 'sensitivity suffix is capitalised from the stored value',
  },
  {
    prefix: 'customFields.types.',
    file: 'components/CustomFieldsSettings.tsx',
    reason: 'custom field type is a backend enum',
  },
  {
    prefix: 'diagnostics.checks.',
    file: 'components/DiagnosticsPanel.tsx',
    reason: 'check name comes from the diagnostics payload',
  },
  {
    prefix: 'diagnostics.status.',
    file: 'components/DiagnosticsPanel.tsx',
    reason: 'check status is an enum from the server',
  },
  {
    prefix: 'feeds.detail.',
    file: 'components/FeedCreateDialog.tsx',
    reason: 'feed detail level is a backend enum',
  },
  {
    prefix: 'feeds.detailHelp.',
    file: 'components/FeedCreateDialog.tsx',
    reason: 'feed detail level is a backend enum',
  },
  {
    prefix: 'feeds.kind.',
    file: 'components/FeedCreateDialog.tsx',
    reason: 'feed kind is a backend enum',
  },
  {
    prefix: 'gifts.status.',
    file: 'components/GiftDialog.tsx',
    reason: 'gift status is a backend enum',
  },
  {
    prefix: 'household.roles.',
    file: 'components/HouseholdList.tsx',
    reason: 'household role is free text with suggested tokens',
  },
  {
    prefix: 'household.types.',
    file: 'components/HouseholdDialog.tsx',
    reason: 'household type is a backend enum',
  },
  {
    prefix: 'lifeEvent.categories.',
    file: 'components/LifeEventDialog.tsx',
    reason: 'life-event category is a backend enum',
  },
  {
    prefix: 'lifeEvent.types.',
    file: 'components/LifeEventDialog.tsx',
    reason: 'life-event type is a backend enum',
  },
  {
    prefix: 'notificationHealth.channels.',
    file: 'components/NotificationHealthPanel.tsx',
    reason: 'notification channel is an enum from the server',
  },
  {
    prefix: 'notificationHealth.status.',
    file: 'components/NotificationHealthPanel.tsx',
    reason: 'notification health status is an enum from the server',
  },
  {
    prefix: 'notifications.settings.channels.',
    file: 'components/NotificationSettings.tsx',
    reason: 'notification channel is an enum from the server',
  },
  {
    prefix: 'occasions.events.rsvp.',
    file: 'components/OccasionEventAttendeesDialog.tsx',
    reason: 'RSVP state is a backend enum',
  },
  {
    prefix: 'occasions.giftShoppingList.status.',
    file: 'OccasionsPage.tsx',
    reason: 'gift shopping status is a backend enum',
  },
  {
    prefix: 'occasions.kinds.',
    file: 'components/OccasionObligationList.tsx',
    reason: 'occasion kind is a backend enum',
  },
  {
    prefix: 'occasions.months.',
    file: 'components/OccasionObligationDialog.tsx',
    reason: 'month number selects one of twelve keys',
  },
  {
    prefix: 'occasions.obligation.sensitivities.',
    file: 'components/OccasionEventList.tsx',
    reason: 'sensitivity is a backend enum',
  },
  {
    prefix: 'occasions.sources.',
    file: 'components/UpcomingOccasionsWidget.tsx',
    reason: 'occasion source is a backend enum',
  },
  {
    prefix: 'preference.categories.',
    file: 'components/PreferenceDialog.tsx',
    reason: 'preference category is a backend enum',
  },
  {
    prefix: 'preference.keys.',
    file: 'components/PreferenceDialog.tsx',
    reason: 'preference key is free text with suggested tokens',
  },
  {
    prefix: 'preference.levels.',
    file: 'components/PreferenceDialog.tsx',
    reason: 'preference level is a backend enum',
  },
  {
    prefix: 'preference.sections.',
    file: 'components/PreferenceList.tsx',
    reason: 'preference section is derived from the category',
  },
  {
    prefix: 'preference.sensitivities.',
    file: 'components/PreferenceList.tsx',
    reason: 'sensitivity is a backend enum',
  },
  {
    prefix: 'prep.upcomingDates.',
    file: 'PrepViewPage.tsx',
    reason: 'upcoming-date label comes from the briefing payload',
  },
  {
    prefix: 'reachOut.kind.',
    file: 'components/ReachOutSuggestionsList.tsx',
    reason: 'reach-out suggestion kind is a backend enum',
  },
  {
    prefix: 'relationships.sensitivity',
    file: 'components/RelationshipEdgeList.tsx',
    values: ['Normal', 'Private', 'Secret'],
    reason: 'sensitivity suffix is capitalised from the stored value',
  },
  {
    prefix: 'relationships.types.',
    file: 'api/relationshipEdges.ts',
    reason: 'relationship type tokens are a backend enum',
  },
  {
    prefix: 'reminders.recurrence.',
    file: 'components/ReminderList.tsx',
    reason: 'recurrence rule is a backend enum',
  },
  {
    prefix: 'settings.contactFields.groups.',
    file: 'components/ContactFieldSettings.tsx',
    reason: 'field group is a backend enum',
  },
  {
    prefix: 'settings.data.import.history.format.',
    file: 'DataSettingsPage.tsx',
    reason: 'import format comes from the history payload',
  },
  {
    prefix: 'settings.exportFieldPicker.sections.',
    file: 'components/FieldSectionPicker.tsx',
    reason: 'export section token comes from the field list',
  },
  {
    prefix: 'settings.linkFieldTypes.categoryOptions.',
    file: 'components/LinkFieldTypesSettings.tsx',
    reason: 'link-field category is a backend enum',
  },
  {
    prefix: 'settings.sourceImport.action.',
    file: 'components/sourceImport/SourceImportReview.tsx',
    reason: 'source-import action is a backend enum',
  },
  {
    prefix: 'settings.sourceImport.lossCategory.',
    file: 'components/sourceImport/SourceImportLossReport.tsx',
    reason: 'loss category comes from the report payload',
  },
  {
    prefix: 'settings.sourceImport.phase.',
    file: 'components/sourceImport/SourceImportProgress.tsx',
    reason: 'source-import phase is a backend enum',
  },
  {
    prefix: 'settings.sourceImport.steps.',
    file: 'components/sourceImport/SourceImportWizard.tsx',
    reason: 'wizard step id comes from the step list',
  },
  {
    prefix: 'settings.timeFormat.options.',
    file: 'SettingsPage.tsx',
    reason: 'time format option is a closed set of tokens',
  },
  {
    prefix: 'settings.webhooks.eventLabels.',
    file: 'components/WebhooksSettings.tsx',
    reason: 'webhook event name comes from the server',
  },
  {
    prefix: 'subsystemHealth.status.',
    file: 'components/SubsystemHealthPanel.tsx',
    reason: 'subsystem status is an enum from the server',
  },
  {
    prefix: 'subsystemHealth.subsystems.',
    file: 'components/SubsystemHealthPanel.tsx',
    reason: 'subsystem name comes from the health payload',
  },
  {
    prefix: 'syncConflicts.field.',
    file: 'components/SyncConflictList.tsx',
    reason: 'conflicting field name comes from the sync payload',
  },
  {
    prefix: 'systemEvents.eventTypes.',
    file: 'SystemEventsPage.tsx',
    reason: 'event type comes from the event payload',
  },
  {
    prefix: 'systemEvents.results.',
    file: 'SystemEventsPage.tsx',
    reason: 'event result is a backend enum',
  },
  {
    prefix: 'systemEvents.severities.',
    file: 'SystemEventsPage.tsx',
    reason: 'event severity is a backend enum',
  },
  {
    prefix: 'systemStatus.features.',
    file: 'SystemStatusPage.tsx',
    reason: 'feature name comes from the status payload',
  },
  {
    prefix: 'systemStatus.health.facets.',
    file: 'SystemStatusPage.tsx',
    reason: 'health facet name comes from the status payload',
  },
  {
    prefix: 'systemStatus.status.',
    file: 'SystemStatusPage.tsx',
    reason: 'component status is an enum from the server',
  },
  {
    prefix: 'targetCloseness.status.',
    file: 'components/ContactHeader.tsx',
    reason: 'target-closeness status is a backend enum',
  },
  {
    prefix: 'targetCloseness.tier.',
    file: 'components/ContactHeader.tsx',
    reason: 'target-closeness tier is a backend enum',
  },
  {
    prefix: 'timeline.buckets.',
    file: 'components/TimelineExplorerDialog.tsx',
    reason: 'timeline bucket id comes from the grouping logic',
  },
  {
    prefix: 'timeline.gift.',
    file: 'components/ContactTimeline.tsx',
    reason: 'gift status is a backend enum',
  },
  {
    prefix: 'timeline.types.',
    file: 'components/TimelineExplorerDialog.tsx',
    reason: 'timeline item type comes from the entry',
  },
];

// Vitest runs from the frontend root (same assumption as craMigrationGuard).
const SRC_DIR = join(process.cwd(), 'src');

// Test-only trees under src/ are not UI usage.
const EXCLUDED_DIRS = new Set(['generated', 'test', 'testSupport']);

function collectSourceFiles(dir: string, acc: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) {
      if (!EXCLUDED_DIRS.has(entry)) collectSourceFiles(full, acc);
    } else if (
      /\.(ts|tsx)$/.test(entry) &&
      !/\.(test|spec)\./.test(entry) &&
      entry !== 'setupTests.ts'
    ) {
      acc.push(full);
    }
  }
  return acc;
}

// `src/`-relative POSIX paths, matching DYNAMIC_KEY_SOURCES.file.
const SOURCE_BY_PATH = new Map(
  collectSourceFiles(SRC_DIR).map((file) => [
    relative(SRC_DIR, file).split('\\').join('/'),
    readFileSync(file, 'utf8'),
  ]),
);
const SOURCES = Object.fromEntries(SOURCE_BY_PATH);

// Separate patterns on purpose: a template literal can contain a quoted key
// (`${t('notes.assignedTo')}`), and a combined alternation would swallow the
// inner literal as part of the template.
const STRING_LITERAL_PATTERNS = [/'([^'\\\n]*)'/g, /"([^"\\\n]*)"/g, /`([^`\\]*)`/g];

function stringLiterals(source: string): string[] {
  const literals: string[] = [];
  for (const pattern of STRING_LITERAL_PATTERNS) {
    for (const match of source.matchAll(pattern)) literals.push(match[1]);
  }
  return literals;
}

function flatten(value: Translations, prefix = ''): Map<string, string> {
  const out = new Map<string, string>();
  for (const [key, child] of Object.entries(value)) {
    const path = prefix ? `${prefix}.${key}` : key;
    if (child !== null && typeof child === 'object' && !Array.isArray(child)) {
      for (const [k, v] of flatten(child as Translations, path)) out.set(k, v);
    } else {
      out.set(path, String(child));
    }
  }
  return out;
}

// i18next plural forms: t('duplicates.count') reads duplicates.count_one /
// _other / _many, none of which appear literally in the source.
const PLURAL_SUFFIX = /_(zero|one|two|few|many|other)$/;
const baseKey = (key: string) => key.replace(PLURAL_SUFFIX, '');

function isDynamicallyReferenced(key: string): boolean {
  const base = baseKey(key);
  for (const entry of DYNAMIC_KEY_SOURCES) {
    if (entry.values) {
      const candidates = entry.values.map((value) => `${entry.prefix}${value}`);
      if (candidates.includes(key) || candidates.includes(base)) return true;
    } else if (key.startsWith(entry.prefix) || base.startsWith(entry.prefix)) {
      return true;
    }
  }
  return false;
}

const enKeys = flatten(en as Translations);

describe('i18n key usage (issue #1907)', () => {
  const literals = new Set(Object.values(SOURCES).flatMap(stringLiterals));

  test('every leaf key in en.json is referenced from src/', () => {
    const unused = [...enKeys.keys()].filter((key) => {
      if (literals.has(key) || literals.has(baseKey(key))) return false;
      return !isDynamicallyReferenced(key);
    });

    expect(
      unused,
      'these en.json keys are not referenced anywhere under src/ (delete them from all five locales)',
    ).toEqual([]);
  });

  test('every dynamic-prefix entry names a real builder file', () => {
    const stale: string[] = [];

    for (const entry of DYNAMIC_KEY_SOURCES) {
      const source = SOURCE_BY_PATH.get(entry.file);
      if (source === undefined) {
        stale.push(`${entry.prefix}: ${entry.file} does not exist`);
      } else if (!source.includes(entry.prefix)) {
        stale.push(`${entry.prefix}: ${entry.file} no longer contains that prefix`);
      }
    }

    expect(
      stale,
      'the dynamic-key allowlist has entries whose builder file or prefix changed; update DYNAMIC_KEY_SOURCES',
    ).toEqual([]);
  });

  test('every dynamic-prefix entry is justified', () => {
    const unjustified = DYNAMIC_KEY_SOURCES.filter((entry) => entry.reason.trim() === '').map(
      (entry) => entry.prefix,
    );
    expect(unjustified).toEqual([]);
  });
});
