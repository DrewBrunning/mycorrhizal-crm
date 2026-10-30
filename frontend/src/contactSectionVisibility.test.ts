import { describe, expect, test } from 'vitest';
import type { Card, CRMEnvelope } from './api/contacts';
import { type ContactFieldKey, DEFAULT_ENABLED_CONTACT_FIELDS } from './contactFields';
import { fieldHasValue, hasVisibleFields } from './contactSectionVisibility';

const emptyCard: Card = {};
const emptyCrm: CRMEnvelope = { kind: 'human' };

function hasValue(
  key: ContactFieldKey,
  card: Card = emptyCard,
  crm: CRMEnvelope = emptyCrm,
): boolean {
  return fieldHasValue(card, crm, undefined, key);
}

describe('fieldHasValue', () => {
  test('an absent value is empty for every cardinality', () => {
    for (const key of DEFAULT_ENABLED_CONTACT_FIELDS) {
      expect(hasValue(key)).toBe(false);
    }
  });

  test('gender is read from the top-level argument, not the card', () => {
    expect(fieldHasValue(emptyCard, emptyCrm, 'non-binary', 'gender')).toBe(true);
    expect(fieldHasValue(emptyCard, emptyCrm, undefined, 'gender')).toBe(false);
  });

  test('birthday is the birth anniversary only', () => {
    const card: Card = {
      anniversaries: [{ kind: 'birth', date: { partial: { year: 1990, month: 4, day: 12 } } }],
    };
    expect(hasValue('birthday', card)).toBe(true);
    expect(hasValue('birthday', { anniversaries: [{ kind: 'wedding', date: {} }] })).toBe(false);
  });

  test('list-valued fields are non-empty only with at least one entry', () => {
    const cases: Array<[ContactFieldKey, Card]> = [
      ['keywords', { keywords: ['friend'] }],
      ['preferredLanguages', { preferredLanguages: [{ language: 'en' }] }],
      ['phones', { phones: [{ number: '+15551234567' }] }],
      ['addresses', { addresses: [{ full: '1 Main St' }] }],
      ['emails', { emails: [{ address: 'a@example.com' }] }],
      ['socialProfiles', { socialProfiles: [{ uri: 'https://social.example/a' }] }],
      ['otherOnlineServices', { otherOnlineServices: [{ uri: 'https://chat.example/a' }] }],
      ['imppAddresses', { imppAddresses: [{ uri: 'xmpp:a@example.com' }] }],
      ['links', { links: [{ uri: 'https://example.com' }] }],
      ['cardNotes', { notes: [{ note: 'met at a conference' }] }],
      ['personalInfo', { personalInfo: [{ kind: 'hobby', value: 'climbing' }] }],
      ['organizations', { organizations: [{ name: 'Acme' }] }],
      ['titles', { titles: [{ name: 'Engineer' }] }],
    ];
    for (const [key, card] of cases) {
      expect(hasValue(key, card)).toBe(true);
    }
    for (const [key] of cases) {
      expect(hasValue(key, {})).toBe(false);
    }
  });

  test('CRM-envelope fields come from the CRM side', () => {
    expect(hasValue('how_we_met', emptyCard, { how_we_met: 'conference' })).toBe(true);
    expect(hasValue('contact_information', emptyCard, { contact_information: 'call first' })).toBe(
      true,
    );
    expect(hasValue('work_information', emptyCard, { work_information: 'Acme' })).toBe(true);
    expect(hasValue('how_we_met', emptyCard, {})).toBe(false);
    expect(hasValue('contact_information', emptyCard, {})).toBe(false);
    expect(hasValue('work_information', emptyCard, {})).toBe(false);
  });

  test('speakToAs needs a pronoun or grammatical gender', () => {
    expect(
      hasValue('speakToAs', {
        speakToAs: { pronouns: [{ pronouns: 'they/them' }] },
      }),
    ).toBe(true);
    expect(
      hasValue('speakToAs', {
        speakToAs: { grammaticalGenders: [{ value: 'neuter' }] },
      }),
    ).toBe(true);
    expect(hasValue('speakToAs', { speakToAs: {} })).toBe(false);
  });

  test('anniversaries excludes the birth entry and follows the legacy key', () => {
    const birthOnly: Card = {
      anniversaries: [{ kind: 'birth', date: { partial: { year: 1990 } } }],
    };
    expect(hasValue('anniversaries', birthOnly)).toBe(false);
    expect(hasValue('anniversary', birthOnly)).toBe(false);

    const withWedding: Card = {
      anniversaries: [
        { kind: 'birth', date: { partial: { year: 1990 } } },
        { kind: 'wedding', date: { partial: { year: 2015 } } },
      ],
    };
    expect(hasValue('anniversaries', withWedding)).toBe(true);
    expect(hasValue('anniversary', withWedding)).toBe(true);
  });

  test('a key with no section mapping has no value', () => {
    expect(hasValue('prefix', {})).toBe(false);
  });
});

describe('hasVisibleFields', () => {
  test('a section is visible only when an enabled field carries a value', () => {
    const card: Card = {
      emails: [{ address: 'a@example.com' }],
      keywords: ['friend'],
    };
    const enabled = new Set<ContactFieldKey>([...DEFAULT_ENABLED_CONTACT_FIELDS, 'keywords']);

    expect(hasVisibleFields('contact', { card, crm: emptyCrm, enabled })).toBe(true);
    expect(hasVisibleFields('about', { card, crm: emptyCrm, enabled })).toBe(true);
    expect(hasVisibleFields('notes', { card, crm: emptyCrm, enabled })).toBe(false);
    expect(hasVisibleFields('genderAndPronouns', { card, crm: emptyCrm, enabled })).toBe(false);
  });

  test('a field that is disabled does not make its section visible', () => {
    const card: Card = { emails: [{ address: 'a@example.com' }] };
    const enabled = new Set<ContactFieldKey>(DEFAULT_ENABLED_CONTACT_FIELDS);
    enabled.delete('emails');

    expect(hasVisibleFields('contact', { card, crm: emptyCrm, enabled })).toBe(false);
  });

  test('notes are visible from either the Card notes or the CRM envelope', () => {
    const enabled = new Set<ContactFieldKey>([...DEFAULT_ENABLED_CONTACT_FIELDS, 'cardNotes']);
    expect(
      hasVisibleFields('notes', {
        card: { notes: [{ note: 'remember the dog' }] },
        crm: emptyCrm,
        enabled,
      }),
    ).toBe(true);
    expect(
      hasVisibleFields('notes', {
        card: emptyCard,
        crm: { how_we_met: 'conference' },
        enabled,
      }),
    ).toBe(true);
  });
});
