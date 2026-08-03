// Single source of truth for the toggleable extended contact fields.
// Used by both the settings UI (ContactFieldSettings) and the contact form/detail.
// firstname/lastname are intentionally NOT listed here — they are always shown.
//
// Keys name the backend's nested Card/CRM concept they gate (see
// backend/openapi.yaml and api/contacts.ts's Card/CRMEnvelope types), not the
// legacy flat Contact shape's field names. Two renames and two consolidations
// fell out of that:
//   - 'urls' -> 'links' (Card.links), 'impps' -> 'imppAddresses'
//     (Card.imppAddresses): same concept, matching the wire name exactly.
//   - 'organization' + 'department' -> a single 'organizations' key: the
//     nested model has one Card.organizations array (department lives as a
//     unit *within* an organization entry), not two independent fields.
//   - 'job_title' + 'role' -> a single 'titles' key: both are entries in
//     Card.titles, differentiated by `kind: 'title' | 'role'`, not separate
//     fields.
// The UI these gate still renders one input per legacy scalar today (the
// adapter shim hasn't been retired yet) — `multiValue` below records which
// concepts are genuinely array-valued in the nested model so the components
// that migrate to real Card arrays (contactFields step 2+) know which
// sections need an add/remove list instead of a single input.

export type ContactFieldKey =
  | 'emails'
  | 'phones'
  | 'addresses'
  | 'links'
  | 'imppAddresses'
  | 'socialProfiles'
  | 'otherOnlineServices'
  | 'nickname'
  | 'gender'
  | 'birthday'
  | 'anniversary'
  | 'anniversaries'
  | 'prefix'
  | 'middle_name'
  | 'suffix'
  | 'organizations'
  | 'titles'
  | 'how_we_met'
  | 'work_information'
  | 'contact_information'
  | 'speakToAs'
  | 'personalInfo'
  | 'keywords'
  | 'cardNotes'
  | 'preferredLanguages'
  | 'cardKind'
  | 'language';

export interface ContactFieldDef {
  key: ContactFieldKey;
  /** i18n key for the field label */
  labelKey: string;
  /** group identifier used to render section subheaders in settings */
  group: 'communication' | 'name' | 'work' | 'personal' | 'mycorrhizal';
  /** true if this concept is an array in the nested Card model (vs. a scalar) */
  multiValue: boolean;
}

export const CONTACT_FIELDS: ContactFieldDef[] = [
  { key: 'emails', labelKey: 'contacts.email', group: 'communication', multiValue: true },
  { key: 'phones', labelKey: 'contacts.phone', group: 'communication', multiValue: true },
  { key: 'addresses', labelKey: 'contacts.address', group: 'communication', multiValue: true },
  { key: 'links', labelKey: 'contacts.urls', group: 'communication', multiValue: true },
  { key: 'imppAddresses', labelKey: 'contacts.impps', group: 'communication', multiValue: true },
  { key: 'socialProfiles', labelKey: 'contacts.socialProfiles', group: 'communication', multiValue: true },
  { key: 'otherOnlineServices', labelKey: 'contacts.otherOnlineServices', group: 'communication', multiValue: true },

  { key: 'prefix', labelKey: 'contacts.prefix', group: 'name', multiValue: false },
  { key: 'middle_name', labelKey: 'contacts.middleName', group: 'name', multiValue: false },
  { key: 'suffix', labelKey: 'contacts.suffix', group: 'name', multiValue: false },
  { key: 'nickname', labelKey: 'contacts.nickname', group: 'name', multiValue: false },

  { key: 'organizations', labelKey: 'contacts.organization', group: 'work', multiValue: true },
  { key: 'titles', labelKey: 'contacts.titles', group: 'work', multiValue: true },
  { key: 'work_information', labelKey: 'contacts.workInformation', group: 'work', multiValue: false },

  { key: 'gender', labelKey: 'contacts.gender', group: 'personal', multiValue: false },
  { key: 'birthday', labelKey: 'contacts.birthday', group: 'personal', multiValue: false },
  { key: 'anniversary', labelKey: 'contacts.anniversary', group: 'personal', multiValue: false },
  { key: 'anniversaries', labelKey: 'contacts.anniversaries', group: 'personal', multiValue: true },
  { key: 'speakToAs', labelKey: 'contacts.speakToAsLabel', group: 'personal', multiValue: true },
  { key: 'personalInfo', labelKey: 'contacts.personalInfoLabel', group: 'personal', multiValue: true },
  { key: 'keywords', labelKey: 'contacts.keywordsLabel', group: 'personal', multiValue: true },
  { key: 'cardNotes', labelKey: 'contacts.cardNotesLabel', group: 'personal', multiValue: true },
  { key: 'preferredLanguages', labelKey: 'contacts.preferredLanguagesLabel', group: 'personal', multiValue: true },
  { key: 'cardKind', labelKey: 'contacts.cardKindLabel', group: 'personal', multiValue: false },
  { key: 'language', labelKey: 'contacts.language', group: 'personal', multiValue: false },

  { key: 'how_we_met', labelKey: 'contacts.howWeMet', group: 'mycorrhizal', multiValue: false },
  { key: 'contact_information', labelKey: 'contacts.contactInformation', group: 'mycorrhizal', multiValue: false },
];

export const CONTACT_FIELD_GROUPS: ContactFieldDef['group'][] = [
  'communication',
  'name',
  'work',
  'personal',
  'mycorrhizal',
];

// Default-enabled set = the fields shown today, so existing users see no change.
// New vCard fields (prefix/middle/suffix, org/dept/title/role, urls, impps,
// anniversary, keywords, card notes, preferred languages) are opt-in via
// Settings. Pronouns/grammatical gender and personal info are enabled by
// default because they are the highest-impact gaps the field-gap audit found
// (T29) — pronouns and interests are table-stakes in a personal CRM, while the
// rest are niche vCard extensions.
export const DEFAULT_ENABLED_CONTACT_FIELDS: ContactFieldKey[] = [
  'emails',
  'phones',
  'addresses',
  'nickname',
  'gender',
  'birthday',
  'speakToAs',
  'personalInfo',
  'how_we_met',
  'work_information',
  'contact_information',
];

// vCard TYPE options for typed multi-value fields (emails/phones/addresses/urls).
export const CONTACT_TYPE_OPTIONS = ['home', 'work', 'cell', 'fax', 'other'] as const;

// vCard CONTEXT options (RFC 9554 §4.1.2): the standard usage contexts shared
// by the neutral model's contexts arrays on emails/phones/addresses/online
// services/nicknames/languages.
export const CONTEXT_OPTIONS = ['private', 'work', 'school', 'billing', 'delivery'] as const;

// Grammatical-gender values (RFC 9554 §3.2 GRAMGENDER).
export const GRAMMATICAL_GENDER_OPTIONS = [
  'animate',
  'common',
  'feminine',
  'inanimate',
  'masculine',
  'neuter',
] as const;

// PersonalInfo kinds (JSContact personalInfo / vCard EXPERTISE/HOBBY/INTEREST).
export const PERSONAL_INFO_KIND_OPTIONS = ['expertise', 'hobby', 'interest'] as const;

// PersonalInfo levels (RFC 9555 §2.8 personalInfo.level).
export const PERSONAL_INFO_LEVEL_OPTIONS = ['high', 'medium', 'low'] as const;

// Card.Kind values (RFC 9553 §2.1.4 kind — distinct from CRMEnvelope.Kind).
export const CARD_KIND_OPTIONS = [
  'individual',
  'group',
  'org',
  'location',
  'application',
  'device',
] as const;

// Phone feature tokens (RFC 9554 §6.4.1 TEL TYPE).
export const PHONE_FEATURE_OPTIONS = [
  'voice',
  'fax',
  'cell',
  'video',
  'pager',
  'text',
  'textphone',
  'main-number',
] as const;

// Resolves the stored setting (null/undefined => defaults) into a concrete enabled set.
export function resolveEnabledFields(stored: string[] | null | undefined): Set<ContactFieldKey> {
  const keys = stored == null ? DEFAULT_ENABLED_CONTACT_FIELDS : (stored as ContactFieldKey[]);
  return new Set(keys);
}
