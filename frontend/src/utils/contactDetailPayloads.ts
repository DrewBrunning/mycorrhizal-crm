import {
  type Card,
  type ContactRecordInput,
  type ContactRecordResponse,
  type CRMEnvelope,
  getOrganizationFields,
  getTitleField,
  type NameComponent,
  nameComponentValue,
  withAnniversary,
  withOrganization,
  withTitles,
} from '../api/contacts';
import type { FieldValueInput } from '../api/fieldDefinitions';
import type { Gift, GiftInput } from '../api/gifts';
import type { LifeEventInputData } from '../api/lifeEvents';
import type { ProfileValues } from '../components/ContactHeader';
import type { LifeEventFormData } from '../components/LifeEventDialog';

// Pure request-shaping helpers for ContactDetailPage: every function here
// turns page state into the exact payload an API call sends, so the rules
// (which half of a paired field is preserved, what "mark given" copies, how
// the profile form maps onto Card.name) are unit-testable without rendering
// the page.

export interface RecordPatch {
  card?: Partial<Card>;
  crm?: Partial<CRMEnvelope>;
  gender?: string;
}

// Fields whose inline editor takes a display-format date (converted to/from
// ISO on the way in and out).
export function isDateField(field: string | null): boolean {
  return field === 'birthday' || field === 'anniversary';
}

// Maps one of ContactInformation's scalar field names to the Card/CRM patch
// it corresponds to. Building an organization/title patch needs the *other*
// half of the pair (department when editing organization, and vice versa),
// which is why it takes the current card.
export function buildRecordPatch(
  card: Card | undefined,
  field: string,
  value: string,
): RecordPatch {
  const c = card || {};
  switch (field) {
    case 'gender':
      return { gender: value };
    case 'birthday':
      return { card: { anniversaries: withAnniversary(c.anniversaries, 'birth', value) } };
    case 'anniversary':
      return { card: { anniversaries: withAnniversary(c.anniversaries, 'wedding', value) } };
    case 'organization': {
      const { department } = getOrganizationFields(c.organizations);
      return { card: { organizations: withOrganization(value, department || '') } };
    }
    case 'department': {
      const { organization } = getOrganizationFields(c.organizations);
      return { card: { organizations: withOrganization(organization || '', value) } };
    }
    case 'job_title': {
      const role = getTitleField(c.titles, 'role');
      return { card: { titles: withTitles(value, role || '') } };
    }
    case 'role': {
      const jobTitle = getTitleField(c.titles, 'title');
      return { card: { titles: withTitles(jobTitle || '', value) } };
    }
    case 'work_information':
      return { crm: { work_information: value } };
    case 'how_we_met':
      return { crm: { how_we_met: value } };
    case 'contact_information':
      return { crm: { contact_information: value } };
    default:
      return {};
  }
}

// The full PUT body for a record with a patch merged over it.
export function applyRecordPatch(
  record: ContactRecordResponse,
  patch: RecordPatch,
): ContactRecordInput {
  return {
    gender: patch.gender ?? record.gender,
    card: { ...record.card, ...patch.card },
    crm: { ...record.crm, ...patch.crm },
  };
}

export const EMPTY_PROFILE_VALUES: ProfileValues = {
  prefix: '',
  firstname: '',
  middle_name: '',
  lastname: '',
  suffix: '',
  nickname: '',
  // CRMEnvelope.Kind (T27): human|animal. Defaults to human so the header's
  // Kind select always has a valid selection.
  kind: 'human',
  // Card.Kind (T29) + Card.Language (T29).
  cardKind: '',
  language: '',
};

export function profileValuesFromRecord(record: ContactRecordResponse): ProfileValues {
  const components = record.card?.name?.components;
  return {
    prefix: nameComponentValue(components, 'title') || '',
    firstname: nameComponentValue(components, 'given') || '',
    middle_name: nameComponentValue(components, 'given2') || '',
    lastname: nameComponentValue(components, 'surname') || '',
    suffix: nameComponentValue(components, 'generation') || '',
    nickname: record.card?.nicknames?.[0]?.name || '',
    kind: record.crm?.kind || 'human',
    cardKind: record.card?.kind || '',
    language: record.card?.language || '',
  };
}

// The PUT body for a header profile save. Preserves the existing name's rich
// metadata (sortAs, phonetic system, separators) and each component's
// phonetic value so an imported contact never loses them on a UI
// edit-and-save (T29). Blank components are dropped.
export function buildProfileUpdate(
  record: ContactRecordResponse,
  values: ProfileValues,
): ContactRecordInput {
  const existingName = record.card?.name;
  const existingComps = existingName?.components || [];
  const phoneticFor = (kind: string) => existingComps.find((c) => c.kind === kind)?.phonetic;

  const nameComponents: NameComponent[] = [];
  const push = (kind: NameComponent['kind'], value: string) => {
    if (value.trim())
      nameComponents.push({ kind, value: value.trim(), phonetic: phoneticFor(kind) });
  };
  push('title', values.prefix);
  push('given', values.firstname);
  push('given2', values.middle_name);
  push('surname', values.lastname);
  push('generation', values.suffix);

  return {
    gender: record.gender,
    card: {
      ...record.card,
      name: {
        ...(existingName || {}),
        components: nameComponents,
      },
      nicknames: values.nickname.trim() ? [{ name: values.nickname.trim() }] : undefined,
      kind: values.cardKind || undefined,
      language: values.language || undefined,
    },
    crm: {
      ...record.crm,
      kind: values.kind,
    },
  };
}

// The full replacement set of custom-field values after one definition's
// value changes (null/undefined clears it). The endpoint is a bulk replace,
// so every other definition's current value is carried along.
export function fieldValueInputsWith(
  current: Map<string, unknown>,
  definitionId: string,
  value: unknown,
): FieldValueInput[] {
  const next = new Map(current);
  if (value === null || value === undefined) {
    next.delete(definitionId);
  } else {
    next.set(definitionId, value);
  }
  const inputs: FieldValueInput[] = [];
  for (const [defId, v] of next) {
    if (v !== null && v !== undefined) inputs.push({ field_definition_id: defId, value: v });
  }
  return inputs;
}

// One-click "mark it given" (T20b): status flips to given, the date defaults
// to now when the idea had none. All other fields are preserved.
export function markGivenGiftInput(gift: Gift, entityId: string, nowIso: string): GiftInput {
  return {
    entity_id: entityId,
    status: 'given',
    description: gift.description,
    url: gift.url,
    notes: gift.notes,
    occasion: gift.occasion,
    date: gift.date ?? nowIso,
    value_cents: gift.value_cents,
    currency: gift.currency,
    life_event_id: gift.life_event_id,
    activity_id: gift.activity_id ?? null,
  };
}

// Explicit field-by-field mapping, not a blind {...data} spread:
// LifeEventFormData.relatedEntityIds is camelCase (the dialog's own shape)
// but the API wants related_entity_ids -- a spread would silently carry the
// wrong key through (TS excess-property checks don't fire on spreads) and the
// picked related contacts would never actually save.
export function lifeEventPayloadFromForm(
  entityId: string,
  data: LifeEventFormData,
): LifeEventInputData {
  return {
    entity_id: entityId,
    type: data.type,
    category: data.category,
    date: data.date,
    end_date: data.endDate,
    description: data.description,
    related_entity_ids: data.relatedEntityIds,
    remind: data.remind,
  };
}
