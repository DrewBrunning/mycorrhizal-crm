import { describe, expect, test } from 'vitest';
import type { Card, ContactRecordResponse } from '../api/contacts';
import type { Gift } from '../api/gifts';
import {
  applyRecordPatch,
  buildProfileUpdate,
  buildRecordPatch,
  EMPTY_PROFILE_VALUES,
  fieldValueInputsWith,
  isDateField,
  lifeEventPayloadFromForm,
  markGivenGiftInput,
  profileValuesFromRecord,
} from './contactDetailPayloads';

const record = (overrides: Partial<ContactRecordResponse> = {}): ContactRecordResponse => ({
  id: 1,
  uid: 'alice-uid',
  etag: '',
  revision: 1,
  gender: 'female',
  card: {
    name: {
      components: [
        { kind: 'given', value: 'Alice' },
        { kind: 'surname', value: 'Wonder' },
      ],
    },
  },
  crm: { kind: 'human', how_we_met: 'School' },
  ...overrides,
});

test('isDateField is true only for birthday and anniversary', () => {
  expect(isDateField('birthday')).toBe(true);
  expect(isDateField('anniversary')).toBe(true);
  expect(isDateField('gender')).toBe(false);
  expect(isDateField(null)).toBe(false);
});

describe('buildRecordPatch', () => {
  const card: Card = {
    organizations: [{ name: 'Acme', units: [{ name: 'Sales' }] }],
    titles: [
      { name: 'Engineer', kind: 'title' },
      { name: 'Lead', kind: 'role' },
    ],
    anniversaries: [{ kind: 'wedding', date: { partial: { year: 2010, month: 6, day: 1 } } }],
  };

  test('gender is a top-level patch', () => {
    expect(buildRecordPatch(card, 'gender', 'male')).toEqual({ gender: 'male' });
  });

  test('birthday replaces only the birth anniversary, keeping the wedding one', () => {
    const patch = buildRecordPatch(card, 'birthday', '1990-04-30');
    expect(patch.card?.anniversaries).toEqual([
      { kind: 'wedding', date: { partial: { year: 2010, month: 6, day: 1 } } },
      { kind: 'birth', date: { partial: { year: 1990, month: 4, day: 30 } } },
    ]);
  });

  test('a blank anniversary clears the wedding date', () => {
    expect(buildRecordPatch(card, 'anniversary', '').card?.anniversaries).toEqual([]);
  });

  test('organization keeps the existing department and vice versa', () => {
    expect(buildRecordPatch(card, 'organization', 'NewCo').card?.organizations).toEqual([
      { name: 'NewCo', units: [{ name: 'Sales' }] },
    ]);
    expect(buildRecordPatch(card, 'department', 'Ops').card?.organizations).toEqual([
      { name: 'Acme', units: [{ name: 'Ops' }] },
    ]);
  });

  test('job title keeps the existing role and vice versa', () => {
    expect(buildRecordPatch(card, 'job_title', 'CTO').card?.titles).toEqual([
      { name: 'CTO', kind: 'title' },
      { name: 'Lead', kind: 'role' },
    ]);
    expect(buildRecordPatch(card, 'role', 'Manager').card?.titles).toEqual([
      { name: 'Engineer', kind: 'title' },
      { name: 'Manager', kind: 'role' },
    ]);
  });

  test('paired fields tolerate a card with nothing set yet', () => {
    expect(buildRecordPatch(undefined, 'department', 'Ops').card?.organizations).toEqual([]);
    expect(buildRecordPatch(undefined, 'organization', 'Acme').card?.organizations).toEqual([
      { name: 'Acme', units: undefined },
    ]);
    expect(buildRecordPatch(undefined, 'role', 'Lead').card?.titles).toEqual([
      { name: 'Lead', kind: 'role' },
    ]);
    expect(buildRecordPatch(undefined, 'job_title', 'CTO').card?.titles).toEqual([
      { name: 'CTO', kind: 'title' },
    ]);
  });

  test.each([['work_information'], ['how_we_met'], ['contact_information']])(
    '%s is a plain CRM string field',
    (field) => {
      expect(buildRecordPatch(card, field, 'x')).toEqual({ crm: { [field]: 'x' } });
    },
  );

  test('an unknown field produces an empty patch', () => {
    expect(buildRecordPatch(card, 'nope', 'x')).toEqual({});
  });
});

describe('applyRecordPatch', () => {
  test('merges card/crm patches over the record and keeps gender when unpatched', () => {
    const r = record();
    expect(applyRecordPatch(r, { crm: { how_we_met: 'Work' } })).toEqual({
      gender: 'female',
      card: r.card,
      crm: { kind: 'human', how_we_met: 'Work' },
    });
  });

  test('a gender patch wins over the record', () => {
    expect(applyRecordPatch(record(), { gender: 'male' }).gender).toBe('male');
  });
});

describe('profile values', () => {
  test('are read from every name component, nickname, kinds and language', () => {
    const r = record({
      card: {
        name: {
          components: [
            { kind: 'title', value: 'Dr.' },
            { kind: 'given', value: 'Alice' },
            { kind: 'given2', value: 'May' },
            { kind: 'surname', value: 'Wonder' },
            { kind: 'generation', value: 'Jr.' },
          ],
        },
        nicknames: [{ name: 'Ali' }],
        kind: 'individual',
        language: 'en',
      },
      crm: { kind: 'animal' },
    });
    expect(profileValuesFromRecord(r)).toEqual({
      prefix: 'Dr.',
      firstname: 'Alice',
      middle_name: 'May',
      lastname: 'Wonder',
      suffix: 'Jr.',
      nickname: 'Ali',
      kind: 'animal',
      cardKind: 'individual',
      language: 'en',
    });
  });

  test('default to empty strings and a human kind', () => {
    expect(profileValuesFromRecord(record({ card: {}, crm: {} }))).toEqual(EMPTY_PROFILE_VALUES);
  });
});

describe('buildProfileUpdate', () => {
  test('preserves name metadata and phonetics, trims, and drops blank components', () => {
    const r = record({
      card: {
        name: {
          sortAs: { surname: 'Wonder' },
          components: [
            { kind: 'given', value: 'Alice', phonetic: 'a-lis' },
            { kind: 'surname', value: 'Wonder' },
          ],
        },
        emails: [{ address: 'a@example.com' }],
      },
    });
    const update = buildProfileUpdate(r, {
      ...EMPTY_PROFILE_VALUES,
      firstname: '  Alicia ',
      lastname: 'Wonderland',
      middle_name: '   ',
      nickname: ' Ali ',
      kind: 'animal',
      cardKind: 'individual',
      language: 'de',
    });
    expect(update).toEqual({
      gender: 'female',
      card: {
        name: {
          sortAs: { surname: 'Wonder' },
          components: [
            { kind: 'given', value: 'Alicia', phonetic: 'a-lis' },
            { kind: 'surname', value: 'Wonderland', phonetic: undefined },
          ],
        },
        emails: [{ address: 'a@example.com' }],
        nicknames: [{ name: 'Ali' }],
        kind: 'individual',
        language: 'de',
      },
      crm: { kind: 'animal', how_we_met: 'School' },
    });
  });

  test('blank nickname/cardKind/language are cleared, and a record with no name still works', () => {
    const update = buildProfileUpdate(record({ card: {} }), {
      ...EMPTY_PROFILE_VALUES,
      prefix: 'Dr.',
      firstname: 'Bob',
      suffix: 'III',
    });
    expect(update.card).toEqual({
      name: {
        components: [
          { kind: 'title', value: 'Dr.', phonetic: undefined },
          { kind: 'given', value: 'Bob', phonetic: undefined },
          { kind: 'generation', value: 'III', phonetic: undefined },
        ],
      },
      nicknames: undefined,
      kind: undefined,
      language: undefined,
    });
  });
});

describe('fieldValueInputsWith', () => {
  const current = new Map<string, unknown>([
    ['def-a', 'one'],
    ['def-b', 2],
  ]);

  test('sets a new value and carries every other one along', () => {
    expect(fieldValueInputsWith(current, 'def-c', true)).toEqual([
      { field_definition_id: 'def-a', value: 'one' },
      { field_definition_id: 'def-b', value: 2 },
      { field_definition_id: 'def-c', value: true },
    ]);
  });

  test('replaces an existing value in place', () => {
    expect(fieldValueInputsWith(current, 'def-a', 'uno')[0]).toEqual({
      field_definition_id: 'def-a',
      value: 'uno',
    });
  });

  test.each([[null], [undefined]])('%s clears the definition', (value) => {
    expect(fieldValueInputsWith(current, 'def-a', value)).toEqual([
      { field_definition_id: 'def-b', value: 2 },
    ]);
  });

  test('drops a nullish value that was already in the map, and never mutates the input', () => {
    const withNull = new Map<string, unknown>([['def-x', null]]);
    expect(fieldValueInputsWith(withNull, 'def-y', 'v')).toEqual([
      { field_definition_id: 'def-y', value: 'v' },
    ]);
    expect([...withNull.keys()]).toEqual(['def-x']);
  });
});

describe('markGivenGiftInput', () => {
  const idea: Gift = {
    id: 'g-1',
    created_at: '',
    updated_at: '',
    entity_id: 'alice-uid',
    status: 'idea',
    description: 'Book',
    url: 'https://example.com',
    notes: 'hardcover',
    occasion: 'birthday',
    value_cents: 1500,
    currency: 'EUR',
    life_event_id: 'le-1',
  };

  test('flips to given, defaults the date to now, preserves the rest', () => {
    expect(markGivenGiftInput(idea, 'alice-uid', '2026-01-01T00:00:00.000Z')).toEqual({
      entity_id: 'alice-uid',
      status: 'given',
      description: 'Book',
      url: 'https://example.com',
      notes: 'hardcover',
      occasion: 'birthday',
      date: '2026-01-01T00:00:00.000Z',
      value_cents: 1500,
      currency: 'EUR',
      life_event_id: 'le-1',
      activity_id: null,
    });
  });

  test('keeps an existing date and activity link', () => {
    const input = markGivenGiftInput(
      { ...idea, date: '2025-12-24T00:00:00Z', activity_id: 9 },
      'alice-uid',
      '2026-01-01T00:00:00.000Z',
    );
    expect(input.date).toBe('2025-12-24T00:00:00Z');
    expect(input.activity_id).toBe(9);
  });
});

test('lifeEventPayloadFromForm maps camelCase form fields onto the API shape', () => {
  expect(
    lifeEventPayloadFromForm('alice-uid', {
      type: 'married',
      category: 'relationships',
      date: { year: 2020 },
      endDate: { year: 2021 },
      description: 'wedding',
      relatedEntityIds: ['bob-uid'],
      remind: true,
    }),
  ).toEqual({
    entity_id: 'alice-uid',
    type: 'married',
    category: 'relationships',
    date: { year: 2020 },
    end_date: { year: 2021 },
    description: 'wedding',
    related_entity_ids: ['bob-uid'],
    remind: true,
  });
});
