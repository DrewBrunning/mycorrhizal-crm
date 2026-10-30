// Display rules for the contact detail view (issue #1997).
//
// A field is *enabled* by the resolved visibility (per-contact override >
// account default > built-in default; the server resolves it, see
// api/contactFieldVisibility.ts). Hide-empty mode then suppresses an enabled
// field that carries no value -- in the DETAIL view only. The page is
// inline-editing, so "the edit view" is the same surface: the add-this-field
// affordance must stay reachable, which is what `hiddenEmptyFields` feeds (the
// "Add a field" menu), and a field the user revealed stays rendered while they
// fill it. Create (AddContactDialog) never calls any of this: every enabled
// field is a blank input to fill.
//
// Cosmetic only. Nothing here is an access boundary.

import type { Card, CRMEnvelope } from './api/contacts';
import type { ContactFieldKey } from './contactFields';
import { fieldHasValue } from './contactSectionVisibility';

/**
 * The fields the detail view renders as a row or section and can therefore
 * hide when empty. Header-only keys (name parts, kind, language) live in the
 * profile edit form, which hide-empty never touches.
 */
export const HIDE_EMPTY_FIELD_KEYS: readonly ContactFieldKey[] = [
  'birthday',
  'preferredLanguages',
  'keywords',
  'phones',
  'addresses',
  'emails',
  'socialProfiles',
  'otherOnlineServices',
  'imppAddresses',
  'links',
  'gender',
  'speakToAs',
  'cardNotes',
  'how_we_met',
  'contact_information',
  'personalInfo',
  'organizations',
  'titles',
  'work_information',
  'anniversaries',
];

export interface FieldDisplayInput {
  card: Card;
  crm: CRMEnvelope;
  gender?: string;
  /** The resolved set of enabled fields for this contact. */
  enabled: ReadonlySet<ContactFieldKey>;
  hideEmpty: boolean;
  /** Fields the user asked to see (via the Add a field menu) although empty. */
  revealed: ReadonlySet<ContactFieldKey>;
}

/** The two legacy anniversary keys share one section, keyed 'anniversaries'. */
function isEnabled(key: ContactFieldKey, enabled: ReadonlySet<ContactFieldKey>): boolean {
  if (key === 'anniversaries') return enabled.has('anniversaries') || enabled.has('anniversary');
  return enabled.has(key);
}

/** Whether the detail view should render `key` right now. */
export function isFieldRendered(key: ContactFieldKey, input: FieldDisplayInput): boolean {
  if (!isEnabled(key, input.enabled)) return false;
  if (!input.hideEmpty) return true;
  if (input.revealed.has(key)) return true;
  return fieldHasValue(input.card, input.crm, input.gender, key);
}

/**
 * The enabled fields hide-empty is currently suppressing, in display order:
 * what the "Add a field" menu offers. Empty when hide-empty is off.
 */
export function hiddenEmptyFields(input: FieldDisplayInput): ContactFieldKey[] {
  if (!input.hideEmpty) return [];
  return HIDE_EMPTY_FIELD_KEYS.filter(
    (key) => isEnabled(key, input.enabled) && !isFieldRendered(key, input),
  );
}

/** The set of enabled keys the detail view renders (the input to the rows). */
export function renderedFieldSet(
  input: FieldDisplayInput,
  allEnabledKeys: Iterable<ContactFieldKey>,
): Set<ContactFieldKey> {
  const out = new Set<ContactFieldKey>();
  for (const key of allEnabledKeys) {
    // The legacy 'anniversary' key renders the same section as 'anniversaries'.
    const sectionKey = key === 'anniversary' ? 'anniversaries' : key;
    // Keys outside the hide-empty scope (header-only) are never suppressed.
    if (!HIDE_EMPTY_FIELD_KEYS.includes(sectionKey) || isFieldRendered(sectionKey, input)) {
      out.add(key);
    }
  }
  return out;
}
