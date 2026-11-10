import { describe, expect, test } from 'vitest';
import type { Entity } from '../api/entities';
import {
  coreSummary,
  coreToForm,
  fillableMergeFields,
  formatPartialInput,
  formToCore,
  parsePartialInput,
} from './entityCore';

describe('parsePartialInput / formatPartialInput', () => {
  test('parses year, year-month and full dates, and blank to undefined', () => {
    expect(parsePartialInput('')).toBeUndefined();
    expect(parsePartialInput('  ')).toBeUndefined();
    expect(parsePartialInput('2019')).toEqual({ year: 2019 });
    expect(parsePartialInput('2019-04')).toEqual({ year: 2019, month: 4 });
    expect(parsePartialInput('2019-04-30')).toEqual({ year: 2019, month: 4, day: 30 });
  });

  test('rejects malformed or impossible dates', () => {
    for (const bad of ['19', '2019-13', '2019-02-30', 'abc', '2019-4', '2019-04-00']) {
      expect(parsePartialInput(bad)).toBe('invalid');
    }
  });

  test('round-trips through formatPartialInput', () => {
    expect(formatPartialInput(undefined)).toBe('');
    expect(formatPartialInput({ year: 2019 })).toBe('2019');
    expect(formatPartialInput({ year: 2019, month: 4 })).toBe('2019-04');
    expect(formatPartialInput({ year: 2019, month: 4, day: 3 })).toBe('2019-04-03');
  });
});

describe('organisation core', () => {
  test('form -> core mirrors the entity name into organization.name and keeps unknown keys', () => {
    const core = formToCore(
      'organisation',
      'Acme',
      {
        sortAs: 'Acme Corp',
        unitsText: 'Sales\n  Support \n',
        url: 'https://acme.test',
        notes: '',
      },
      { extra: 1, organization: { id: 'x', units: [{ name: 'Sales', id: 'u1' }] } },
    );
    expect(core).toEqual({
      extra: 1,
      organization: {
        id: 'x',
        name: 'Acme',
        sortAs: 'Acme Corp',
        units: [{ name: 'Sales', id: 'u1' }, { name: 'Support' }],
      },
      url: 'https://acme.test',
    });
  });

  test('core -> form flattens units to lines', () => {
    expect(
      coreToForm('organisation', {
        organization: { name: 'Acme', units: [{ name: 'A' }, { name: 'B' }] },
        url: 'u',
      }),
    ).toMatchObject({ unitsText: 'A\nB', url: 'u', notes: '', sortAs: '' });
  });
});

describe('place core', () => {
  test('address fields round-trip through the neutral components', () => {
    const core = formToCore('place', 'Cabin', {
      street: '1 Lake Rd',
      city: 'Springfield',
      region: '',
      postal: '12345',
      country: 'US',
      notes: 'Key under mat',
    });
    expect(core).toEqual({
      address: {
        components: [
          { kind: 'name', value: '1 Lake Rd' },
          { kind: 'locality', value: 'Springfield' },
          { kind: 'postcode', value: '12345' },
          { kind: 'country', value: 'US' },
        ],
      },
      notes: 'Key under mat',
    });
    expect(coreToForm('place', core)).toMatchObject({
      street: '1 Lake Rd',
      city: 'Springfield',
      postal: '12345',
      country: 'US',
      notes: 'Key under mat',
    });
  });

  test('an empty address yields an empty address object, never sensitivity', () => {
    const core = formToCore('place', 'Cabin', {
      street: '',
      city: '',
      region: '',
      postal: '',
      country: '',
      notes: '',
    });
    expect(core).toEqual({ address: {} });
    expect(JSON.stringify(core)).not.toContain('sensitivity');
  });
});

describe('pet core', () => {
  test('species/breed/dates/notes round-trip', () => {
    const core = formToCore('pet', 'Rex', {
      species: 'dog',
      breed: 'lab',
      birth: '2019-04',
      death: '',
      notes: '',
    });
    expect(core).toEqual({ species: 'dog', breed: 'lab', birth: { year: 2019, month: 4 } });
    expect(coreToForm('pet', core)).toMatchObject({ birth: '2019-04', death: '' });
  });

  test('an invalid date throws a typed error naming the field', () => {
    expect(() =>
      formToCore('pet', 'Rex', { species: '', breed: '', birth: 'nope', death: '', notes: '' }),
    ).toThrowError(/birth/);
  });
});

describe('coreSummary', () => {
  const base = { id: '1', name: 'X', sensitivity: 'normal', revision: 1 } as Entity;
  test('summarises each kind with one short line', () => {
    expect(coreSummary({ ...base, kind: 'organisation', core: { url: 'https://a.test' } })).toBe(
      'https://a.test',
    );
    expect(
      coreSummary({
        ...base,
        kind: 'place',
        core: { address: { components: [{ kind: 'locality', value: 'Bern' }] } },
      }),
    ).toBe('Bern');
    expect(coreSummary({ ...base, kind: 'pet', core: { species: 'cat', breed: 'tabby' } })).toBe(
      'cat, tabby',
    );
    expect(coreSummary({ ...base, kind: 'pet', core: {} })).toBe('');
  });
});

describe('fillableMergeFields', () => {
  test('lists only fields the survivor leaves empty and the loser has', () => {
    const survivor = {
      kind: 'organisation',
      core: { url: 'https://keep.test' },
    } as unknown as Entity;
    const loser = {
      kind: 'organisation',
      core: {
        url: 'https://other.test',
        notes: 'n',
        organization: { name: 'A', sortAs: 's', units: [{ name: 'U' }] },
      },
    } as unknown as Entity;
    expect(fillableMergeFields(survivor, loser)).toEqual([
      'organization.sortAs',
      'organization.units',
      'notes',
    ]);
  });

  test('place and pet fields', () => {
    const place = (core: object) => ({ kind: 'place', core }) as unknown as Entity;
    expect(
      fillableMergeFields(
        place({}),
        place({ address: { components: [{ kind: 'name', value: 'x' }] } }),
      ),
    ).toEqual(['address']);
    expect(
      fillableMergeFields(place({ address: { full: 'F' } }), place({ address: { full: 'G' } })),
    ).toEqual([]);
    const pet = (core: object) => ({ kind: 'pet', core }) as unknown as Entity;
    expect(
      fillableMergeFields(
        pet({ species: 'dog' }),
        pet({ species: 'cat', breed: 'b', birth: { year: 2000 } }),
      ),
    ).toEqual(['breed', 'birth']);
  });
});
