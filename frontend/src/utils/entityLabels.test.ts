import { describe, expect, it } from 'vitest';
import {
  ENTITY_EMPTY_KEYS,
  ENTITY_KIND_KEYS,
  ENTITY_KIND_PLURAL_KEYS,
  ENTITY_NEW_KEYS,
  ENTITY_REASON_KEYS,
  ENTITY_ROLE_KEYS,
  ENTITY_SENSITIVITY_KEYS,
  roleKey,
} from './entityLabels';

describe('entityLabels', () => {
  it('maps every kind to its exact i18n key in each kind table', () => {
    expect(ENTITY_KIND_KEYS).toEqual({
      organisation: 'entities.kind.organisation',
      place: 'entities.kind.place',
      pet: 'entities.kind.pet',
    });
    expect(ENTITY_KIND_PLURAL_KEYS).toEqual({
      organisation: 'entities.kindPlural.organisation',
      place: 'entities.kindPlural.place',
      pet: 'entities.kindPlural.pet',
    });
    expect(ENTITY_NEW_KEYS).toEqual({
      organisation: 'entities.new.organisation',
      place: 'entities.new.place',
      pet: 'entities.new.pet',
    });
    expect(ENTITY_EMPTY_KEYS).toEqual({
      organisation: 'entities.empty.organisation',
      place: 'entities.empty.place',
      pet: 'entities.empty.pet',
    });
  });

  it('maps sensitivities and suggestion reasons to their exact keys', () => {
    expect(ENTITY_SENSITIVITY_KEYS).toEqual({
      normal: 'entities.sensitivity.normal',
      private: 'entities.sensitivity.private',
      secret: 'entities.sensitivity.secret',
    });
    expect(ENTITY_REASON_KEYS).toEqual({
      exact_name: 'entities.suggestions.reason.exact_name',
      normalized_name: 'entities.suggestions.reason.normalized_name',
      address_components: 'entities.suggestions.reason.address_components',
    });
  });

  it('maps every role token to its exact per-kind key', () => {
    expect(ENTITY_ROLE_KEYS).toEqual({
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
    });
  });

  it('roleKey resolves a known role and returns undefined otherwise', () => {
    expect(roleKey('pet', 'carer')).toBe('entities.roles.pet.carer');
    expect(roleKey('place', 'carer')).toBeUndefined();
    expect(roleKey('pet', 'nope')).toBeUndefined();
    // An unknown kind (e.g. a newer server) must not throw.
    expect(roleKey('robot' as never, 'owner')).toBeUndefined();
  });
});
