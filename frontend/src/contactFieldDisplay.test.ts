import { describe, expect, test } from 'vitest';
import type { Card, CRMEnvelope } from './api/contacts';
import {
  type FieldDisplayInput,
  HIDE_EMPTY_FIELD_KEYS,
  hiddenEmptyFields,
  isFieldRendered,
  renderedFieldSet,
} from './contactFieldDisplay';
import { type ContactFieldKey, DEFAULT_ENABLED_CONTACT_FIELDS } from './contactFields';

const card: Card = {
  emails: [{ address: 'a@example.com' }],
  organizations: [{ name: 'Acme' }],
  anniversaries: [{ kind: 'birth', date: { partial: { year: 1990 } } }],
};
const crm: CRMEnvelope = { kind: 'human' };

function input(over: Partial<FieldDisplayInput> = {}): FieldDisplayInput {
  return {
    card,
    crm,
    gender: '',
    enabled: new Set<ContactFieldKey>(DEFAULT_ENABLED_CONTACT_FIELDS),
    hideEmpty: false,
    revealed: new Set<ContactFieldKey>(),
    ...over,
  };
}

describe('isFieldRendered', () => {
  test('a disabled field is never rendered, with or without a value', () => {
    expect(isFieldRendered('links', input())).toBe(false);
    expect(
      isFieldRendered('links', input({ card: { links: [{ uri: 'https://x.example' }] } })),
    ).toBe(false);
  });

  test('without hide-empty every enabled field renders, empty or not', () => {
    expect(isFieldRendered('emails', input())).toBe(true);
    expect(isFieldRendered('phones', input())).toBe(true);
  });

  test('hide-empty suppresses an enabled field with no value but keeps one with a value', () => {
    expect(isFieldRendered('phones', input({ hideEmpty: true }))).toBe(false);
    expect(isFieldRendered('emails', input({ hideEmpty: true }))).toBe(true);
  });

  test('a revealed field stays rendered while empty (the user is filling it)', () => {
    const revealed = new Set<ContactFieldKey>(['phones']);
    expect(isFieldRendered('phones', input({ hideEmpty: true, revealed }))).toBe(true);
  });

  test('a birth anniversary is not an anniversaries-section value', () => {
    const enabled = new Set<ContactFieldKey>(['anniversaries']);
    expect(isFieldRendered('anniversaries', input({ enabled, hideEmpty: true }))).toBe(false);
    const withWedding: Card = {
      anniversaries: [{ kind: 'wedding', date: { partial: { year: 2015 } } }],
    };
    expect(
      isFieldRendered('anniversaries', input({ enabled, hideEmpty: true, card: withWedding })),
    ).toBe(true);
  });

  test('the legacy anniversary key enables the same section', () => {
    const enabled = new Set<ContactFieldKey>(['anniversary']);
    expect(isFieldRendered('anniversaries', input({ enabled }))).toBe(true);
  });
});

describe('hiddenEmptyFields', () => {
  test('is empty when hide-empty is off', () => {
    expect(hiddenEmptyFields(input())).toEqual([]);
  });

  test('lists exactly the enabled fields hide-empty is suppressing, in display order', () => {
    const hidden = hiddenEmptyFields(input({ hideEmpty: true }));
    expect(hidden).toContain('phones');
    expect(hidden).toContain('birthday');
    expect(hidden).not.toContain('emails'); // has a value
    expect(hidden).not.toContain('links'); // not enabled
    // Display order follows HIDE_EMPTY_FIELD_KEYS.
    const idx = hidden.map((k) => HIDE_EMPTY_FIELD_KEYS.indexOf(k));
    expect(idx).toEqual([...idx].sort((a, b) => a - b));
  });

  test('a revealed field leaves the menu', () => {
    const revealed = new Set<ContactFieldKey>(['phones']);
    expect(hiddenEmptyFields(input({ hideEmpty: true, revealed }))).not.toContain('phones');
  });
});

describe('renderedFieldSet', () => {
  test('removes only the suppressed fields and leaves header-only keys alone', () => {
    const enabled = new Set<ContactFieldKey>([
      'emails',
      'phones',
      'prefix',
      'cardKind',
      'nickname',
    ]);
    const out = renderedFieldSet(input({ enabled, hideEmpty: true }), enabled);
    expect(out.has('emails')).toBe(true);
    expect(out.has('phones')).toBe(false);
    // Profile-edit-form keys are never suppressed by hide-empty.
    expect(out.has('prefix')).toBe(true);
    expect(out.has('cardKind')).toBe(true);
    expect(out.has('nickname')).toBe(true);
  });

  test('is the enabled set unchanged when hide-empty is off', () => {
    const enabled = new Set<ContactFieldKey>(DEFAULT_ENABLED_CONTACT_FIELDS);
    expect(renderedFieldSet(input({ enabled }), enabled)).toEqual(enabled);
  });

  test('the legacy anniversary key follows its section', () => {
    const enabled = new Set<ContactFieldKey>(['anniversary', 'anniversaries']);
    expect(renderedFieldSet(input({ enabled, hideEmpty: true }), enabled).size).toBe(0);
    const withWedding: Card = {
      anniversaries: [{ kind: 'wedding', date: { partial: { year: 2015 } } }],
    };
    expect(
      renderedFieldSet(input({ enabled, hideEmpty: true, card: withWedding }), enabled),
    ).toEqual(enabled);
  });
});
