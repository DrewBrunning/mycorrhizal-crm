// i18n key maps for the closed entity vocabularies (issue #2301). Written as
// literal keys, not template strings, so the unused-key gate
// (i18n/keyUsage.test.ts) sees every one of them.
import type { EntityKind, EntitySensitivity, EntitySuggestionReason } from '../api/entities';

// Stryker disable all: module-level constant tables are "static" mutants, which
// the vitest runner cannot re-load per mutant (they always survive); their
// values are pinned by entityLabels.test.ts and the keyUsage gate instead.
export const ENTITY_KIND_KEYS: Record<EntityKind, string> = {
  organisation: 'entities.kind.organisation',
  place: 'entities.kind.place',
  pet: 'entities.kind.pet',
};

export const ENTITY_KIND_PLURAL_KEYS: Record<EntityKind, string> = {
  organisation: 'entities.kindPlural.organisation',
  place: 'entities.kindPlural.place',
  pet: 'entities.kindPlural.pet',
};

export const ENTITY_NEW_KEYS: Record<EntityKind, string> = {
  organisation: 'entities.new.organisation',
  place: 'entities.new.place',
  pet: 'entities.new.pet',
};

export const ENTITY_EMPTY_KEYS: Record<EntityKind, string> = {
  organisation: 'entities.empty.organisation',
  place: 'entities.empty.place',
  pet: 'entities.empty.pet',
};

export const ENTITY_SENSITIVITY_KEYS: Record<EntitySensitivity, string> = {
  normal: 'entities.sensitivity.normal',
  private: 'entities.sensitivity.private',
  secret: 'entities.sensitivity.secret',
};

export const ENTITY_REASON_KEYS: Record<EntitySuggestionReason, string> = {
  exact_name: 'entities.suggestions.reason.exact_name',
  normalized_name: 'entities.suggestions.reason.normalized_name',
  address_components: 'entities.suggestions.reason.address_components',
};

export const ENTITY_ROLE_KEYS: Record<EntityKind, Record<string, string>> = {
  organisation: {
    employee: 'entities.roles.organisation.employee',
    owner: 'entities.roles.organisation.owner',
    contractor: 'entities.roles.organisation.contractor',
    volunteer: 'entities.roles.organisation.volunteer',
    member: 'entities.roles.organisation.member',
    other: 'entities.roles.organisation.other',
  },
  place: {
    resident: 'entities.roles.place.resident',
    owner: 'entities.roles.place.owner',
    visitor: 'entities.roles.place.visitor',
    other: 'entities.roles.place.other',
  },
  pet: {
    owner: 'entities.roles.pet.owner',
    carer: 'entities.roles.pet.carer',
    other: 'entities.roles.pet.other',
  },
};

// Stryker restore all
/** The translation key for a role token, or undefined for one this client does not know. */
export function roleKey(kind: EntityKind, role: string): string | undefined {
  return ENTITY_ROLE_KEYS[kind]?.[role];
}
