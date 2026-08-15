import { describe, test, expect, vi, beforeEach, afterEach } from 'vitest';
import {
  cardEmailsToValues,
  valuesToCardEmails,
  cardPhonesToValues,
  valuesToCardPhones,
  cardAddressesToValues,
  valuesToCardAddresses,
  CardAddress,
  getAnniversaryField,
  withAnniversary,
  getOrganizationFields,
  withOrganization,
  getTitleField,
  withTitles,
  formatAnniversaryDate,
  parseAnniversaryDate,
  toContactRecordInput,
  getContactsByUid,
  getContacts,
  getAllContacts,
  onlineServicesToRows,
  rowsToOnlineServices,
} from './contacts';

describe('email conversion', () => {
  test('round-trips multiple emails with contexts', () => {
    const values = cardEmailsToValues([
      { address: 'work@example.com', contexts: ['work'] },
      { address: 'home@example.com', contexts: ['home'] },
    ]);
    expect(values).toEqual([
      { type: 'work', value: 'work@example.com', contexts: ['work'] },
      { type: 'home', value: 'home@example.com', contexts: ['home'] },
    ]);
    expect(valuesToCardEmails(values)).toEqual([
      { address: 'work@example.com', contexts: ['work'] },
      { address: 'home@example.com', contexts: ['home'] },
    ]);
  });

  test('drops rows with an empty value when converting back', () => {
    expect(valuesToCardEmails([{ type: 'home', value: '  ' }, { type: '', value: 'a@b.com' }])).toEqual([
      { address: 'a@b.com', contexts: undefined },
    ]);
  });

  test('handles an empty/undefined array', () => {
    expect(cardEmailsToValues(undefined)).toEqual([]);
  });

  test('preserves pref and label through the round trip (WP11)', () => {
    const card = [{ address: 'work@example.com', contexts: ['work', 'private'], pref: 1, label: 'Main' }];
    const values = cardEmailsToValues(card);
    expect(values[0].pref).toBe(1);
    expect(values[0].label).toBe('Main');
    expect(values[0].contexts).toEqual(['work', 'private']);
    expect(valuesToCardEmails(values)).toEqual(card);
  });
});

describe('phone conversion', () => {
  test('display prefers features over contexts (vCard feature tokens like cell/fax)', () => {
    expect(cardPhonesToValues([{ number: '555-1234', features: ['cell'], contexts: ['work'] }])).toEqual([
      { type: 'cell', value: '555-1234', features: ['cell'], contexts: ['work'] },
    ]);
  });

  test('falls back to contexts when no features are set', () => {
    expect(cardPhonesToValues([{ number: '555-1234', contexts: ['work'] }])).toEqual([
      { type: 'work', value: '555-1234', contexts: ['work'] },
    ]);
  });

  test('valuesToCardPhones writes the type into contexts', () => {
    expect(valuesToCardPhones([{ type: 'cell', value: '555-1234' }])).toEqual([
      { number: '555-1234', contexts: ['cell'] },
    ]);
  });

  test('preserves features and pref through the round trip (WP11)', () => {
    const card = [{ number: '555-1234', features: ['cell', 'text'], contexts: ['work'], pref: 2, label: 'Work cell' }];
    const values = cardPhonesToValues(card);
    expect(values[0].features).toEqual(['cell', 'text']);
    expect(values[0].pref).toBe(2);
    expect(values[0].label).toBe('Work cell');
    expect(valuesToCardPhones(values)).toEqual(card);
  });
});

describe('address conversion', () => {
  test('round-trips a full address', () => {
    const card = [
      {
        contexts: ['home'],
        components: [
          { kind: 'name', value: '123 Main St' },
          { kind: 'locality', value: 'Springfield' },
          { kind: 'region', value: 'IL' },
          { kind: 'postcode', value: '62704' },
          { kind: 'country', value: 'USA' },
        ],
      },
    ];
    const values = cardAddressesToValues(card);
    expect(values).toEqual([
      { type: 'home', street: '123 Main St', city: 'Springfield', region: 'IL', postal: '62704', country: 'USA' },
    ]);
    expect(valuesToCardAddresses(values)).toEqual(card);
  });

  test('translates neutral address contexts to the flat type vocabulary (T91)', () => {
    // The importer stores a vCard ADR;TYPE=home as contexts:["private"] --
    // correct for RFC 9553, but "private" has no contacts.types.* i18n key, so
    // it rendered as the raw token on the contact detail page.
    const mk = (context: string): CardAddress[] => [
      { components: [{ kind: 'name', value: '123 Fake St' }], contexts: [context] },
    ];
    expect(cardAddressesToValues(mk('private'))[0].type).toBe('home');
    expect(cardAddressesToValues(mk('work'))[0].type).toBe('work');
    expect(cardAddressesToValues(mk('billing'))[0].type).toBe('billing');
    // Already-flat tokens pass through unchanged...
    expect(cardAddressesToValues(mk('home'))[0].type).toBe('home');
    // ...and so does arbitrary free text, which the write side genuinely
    // allows into contexts, so it must be shown rather than blanked.
    expect(cardAddressesToValues(mk('cabin'))[0].type).toBe('cabin');
    // No contexts at all stays empty, not undefined-mapped.
    expect(cardAddressesToValues([{ components: [{ kind: 'name', value: 'x' }] }])[0].type).toBe('');
  });

  test('preserves unknown address component kinds through round-trip (T25)', () => {
    const card: CardAddress[] = [
      {
        components: [
          { kind: 'name', value: '123 Main St' },
          { kind: 'room', value: 'Loft' },
          { kind: 'building', value: 'North Tower' },
          { kind: 'locality', value: 'Springfield' },
          { kind: 'region', value: 'IL' },
          { kind: 'postcode', value: '62704' },
          { kind: 'country', value: 'USA' },
        ],
        contexts: ['home'],
      },
    ];
    const values = cardAddressesToValues(card);
    expect(values[0].street).toBe('123 Main St');
    expect(values[0].passthrough).toEqual([
      { kind: 'room', value: 'Loft' },
      { kind: 'building', value: 'North Tower' },
    ]);
    // Full round-trip preserves non-standard components (order may differ).
    const result = valuesToCardAddresses(values);
    const resultComps = result[0].components || [];
    expect(resultComps).toEqual(expect.arrayContaining([
      { kind: 'name', value: '123 Main St' },
      { kind: 'room', value: 'Loft' },
      { kind: 'building', value: 'North Tower' },
      { kind: 'locality', value: 'Springfield' },
      { kind: 'region', value: 'IL' },
      { kind: 'postcode', value: '62704' },
      { kind: 'country', value: 'USA' },
    ]));
    expect(result[0].contexts).toEqual(['home']);
  });

  test('round-trips PO box / apartment / floor as first-class flat fields (T79)', () => {
    const card: CardAddress[] = [
      {
        components: [
          { kind: 'name', value: '123 Main St' },
          { kind: 'postOfficeBox', value: 'PO Box 42' },
          { kind: 'apartment', value: '3B' },
          { kind: 'floor', value: '4' },
          { kind: 'locality', value: 'Springfield' },
          { kind: 'region', value: 'IL' },
          { kind: 'postcode', value: '62704' },
          { kind: 'country', value: 'USA' },
        ],
        contexts: ['home'],
      },
    ];
    const values = cardAddressesToValues(card);
    expect(values[0].street).toBe('123 Main St');
    expect(values[0].pobox).toBe('PO Box 42');
    expect(values[0].apartment).toBe('3B');
    expect(values[0].floor).toBe('4');
    // The three kinds no longer ride passthrough — they have flat slots now.
    expect(values[0].passthrough).toBeUndefined();
    const result = valuesToCardAddresses(values);
    const resultComps = result[0].components || [];
    expect(resultComps).toEqual(expect.arrayContaining([
      { kind: 'name', value: '123 Main St' },
      { kind: 'postOfficeBox', value: 'PO Box 42' },
      { kind: 'apartment', value: '3B' },
      { kind: 'floor', value: '4' },
      { kind: 'locality', value: 'Springfield' },
      { kind: 'region', value: 'IL' },
      { kind: 'postcode', value: '62704' },
      { kind: 'country', value: 'USA' },
    ]));
    expect(result[0].contexts).toEqual(['home']);
  });

  test('keeps an address whose only non-blank part is a sub-street field (T79)', () => {
    expect(
      valuesToCardAddresses([{ type: 'home', street: '', city: '', region: '', postal: '', country: '', pobox: 'PO Box 42' }])
    ).toEqual([
      { components: [{ kind: 'postOfficeBox', value: 'PO Box 42' }], contexts: ['home'] },
    ]);
  });

  test('drops an address with every field blank', () => {
    expect(valuesToCardAddresses([{ type: 'home', street: '', city: '', region: '', postal: '', country: '' }])).toEqual([]);
  });

  test('preserves coordinates, timeZone, pref and full through the round trip (WP11)', () => {
    const card = [
      {
        components: [
          { kind: 'name', value: '123 Main St' },
          { kind: 'locality', value: 'Springfield' },
        ],
        contexts: ['home'],
        coordinates: 'geo:37.2,-93.3',
        timeZone: 'America/Chicago',
        pref: 1,
        full: '123 Main St\nSpringfield',
      },
    ];
    const values = cardAddressesToValues(card);
    expect(values[0].coordinates).toBe('geo:37.2,-93.3');
    expect(values[0].timeZone).toBe('America/Chicago');
    expect(values[0].pref).toBe(1);
    expect(valuesToCardAddresses(values)).toEqual(card);
  });
});

describe('online service conversion (WP3)', () => {
  test('round-trips social profile rows with service/uri/user', () => {
    const rows = onlineServicesToRows([
      { service: 'Mastodon', uri: 'https://mastodon.social/@ada', user: '@ada', contexts: ['work'], pref: 1, label: 'Work' },
    ]);
    expect(rows).toEqual([
      { service: 'Mastodon', uri: 'https://mastodon.social/@ada', user: '@ada', contexts: ['work'], pref: 1, label: 'Work' },
    ]);
    expect(rowsToOnlineServices(rows)).toEqual([
      { service: 'Mastodon', uri: 'https://mastodon.social/@ada', user: '@ada', contexts: ['work'], pref: 1, label: 'Work' },
    ]);
  });

  test('drops empty rows', () => {
    expect(rowsToOnlineServices([{ service: '', uri: '', user: '', label: '', contexts: [] }])).toEqual([]);
  });

  test('omits blank fields', () => {
    expect(rowsToOnlineServices([{ service: 'GitHub', uri: '', user: '', label: '', contexts: [] }])).toEqual([
      { service: 'GitHub' },
    ]);
  });
});

describe('anniversary date formatting', () => {
  test('formats a full date', () => {
    expect(formatAnniversaryDate({ partial: { year: 1990, month: 3, day: 15 } })).toBe('1990-03-15');
  });

  test('formats a year-less date', () => {
    expect(formatAnniversaryDate({ partial: { month: 3, day: 15 } })).toBe('--03-15');
  });

  test('parses both formats back losslessly', () => {
    expect(parseAnniversaryDate('1990-03-15')).toEqual({ partial: { year: 1990, month: 3, day: 15 } });
    expect(parseAnniversaryDate('--03-15')).toEqual({ partial: { month: 3, day: 15 } });
  });
});

describe('getAnniversaryField / withAnniversary', () => {
  test('reads the entry matching the requested kind only', () => {
    const anniversaries = [
      { kind: 'birth' as const, date: { partial: { year: 1990, month: 3, day: 15 } } },
      { kind: 'wedding' as const, date: { partial: { year: 2015, month: 6, day: 1 } } },
    ];
    expect(getAnniversaryField(anniversaries, 'birth')).toBe('1990-03-15');
    expect(getAnniversaryField(anniversaries, 'wedding')).toBe('2015-06-01');
  });

  test('withAnniversary replaces only the given kind, leaving the other untouched', () => {
    const anniversaries = [
      { kind: 'birth' as const, date: { partial: { year: 1990, month: 3, day: 15 } } },
      { kind: 'wedding' as const, date: { partial: { year: 2015, month: 6, day: 1 } } },
    ];
    const updated = withAnniversary(anniversaries, 'birth', '1991-04-16');
    expect(getAnniversaryField(updated, 'birth')).toBe('1991-04-16');
    expect(getAnniversaryField(updated, 'wedding')).toBe('2015-06-01');
  });

  test('withAnniversary drops the entry when given an empty value', () => {
    const anniversaries = [{ kind: 'birth' as const, date: { partial: { year: 1990, month: 3, day: 15 } } }];
    expect(withAnniversary(anniversaries, 'birth', '')).toEqual([]);
  });
});

describe('organization fields', () => {
  test('getOrganizationFields reads name + first unit as department', () => {
    expect(getOrganizationFields([{ name: 'Acme', units: [{ name: 'R&D' }] }])).toEqual({
      organization: 'Acme',
      department: 'R&D',
    });
  });

  test('withOrganization preserves department when only organization changes', () => {
    // Simulates ContactDetailPage's buildRecordPatch: read the current
    // department, then patch organization while passing it back through.
    const current = getOrganizationFields([{ name: 'Acme', units: [{ name: 'R&D' }] }]);
    const updated = withOrganization('Globex', current.department || '');
    expect(getOrganizationFields(updated)).toEqual({ organization: 'Globex', department: 'R&D' });
  });

  test('withOrganization returns an empty array when organization is blank', () => {
    expect(withOrganization('', 'R&D')).toEqual([]);
  });
});

describe('title fields', () => {
  test('getTitleField distinguishes title from role by kind', () => {
    const titles = [{ name: 'Engineer', kind: 'title' as const }, { name: 'Lead', kind: 'role' as const }];
    expect(getTitleField(titles, 'title')).toBe('Engineer');
    expect(getTitleField(titles, 'role')).toBe('Lead');
  });

  test('withTitles preserves role when only job title changes', () => {
    const current = { role: getTitleField([{ name: 'Lead', kind: 'role' }], 'role') };
    const updated = withTitles('Senior Engineer', current.role || '');
    expect(getTitleField(updated, 'title')).toBe('Senior Engineer');
    expect(getTitleField(updated, 'role')).toBe('Lead');
  });
});

describe('toContactRecordInput', () => {
  // toLegacyContact/getContact/createContact/updateContact were retired once
  // every contact-editing component migrated onto getContactRecord/
  // updateContactRecord/createContactRecord (, Tier 0 items
  // 3-7) -- toContactRecordInput itself survives only for e2e test fixtures
  // (e2e/fixtures.ts, e2e/global-setup.ts), which still find it convenient
  // to build nested payloads from simple flat test data.
  test('builds an equivalent nested shape from a flat Contact-like input', () => {
    const input = toContactRecordInput({
      firstname: 'Marie',
      lastname: 'Curie',
      prefix: 'Dr.',
      middle_name: 'Salomea',
      nickname: 'Manya',
      gender: 'other',
      emails: [{ type: 'work', value: 'marie@sorbonne.fr' }],
      phones: [{ type: 'cell', value: '555-0100' }],
      organization: 'Sorbonne University',
      department: 'Physics',
      job_title: 'Professor',
      role: 'Nobel Laureate',
      birthday: '1867-11-07',
    });

    expect(input.gender).toBe('other');
    expect(input.card.name?.components).toEqual([
      { kind: 'title', value: 'Dr.' },
      { kind: 'given', value: 'Marie' },
      { kind: 'given2', value: 'Salomea' },
      { kind: 'surname', value: 'Curie' },
    ]);
    expect(input.card.nicknames).toEqual([{ name: 'Manya' }]);
    expect(input.card.emails).toEqual([{ address: 'marie@sorbonne.fr', contexts: ['work'] }]);
    expect(input.card.phones).toEqual([{ number: '555-0100', contexts: ['cell'] }]);
    expect(input.card.organizations).toEqual([{ name: 'Sorbonne University', units: [{ name: 'Physics' }] }]);
    expect(input.card.titles).toEqual([{ name: 'Professor', kind: 'title' }, { name: 'Nobel Laureate', kind: 'role' }]);
    expect(input.card.anniversaries).toEqual([{ kind: 'birth', date: { partial: { year: 1867, month: 11, day: 7 } } }]);
  });

  test('maps kind (T27) into crm.kind when set', () => {
    const input = toContactRecordInput({ firstname: 'Fluffy', kind: 'pet' });
    expect(input.crm.kind).toBe('pet');
  });

  test('omits kind from crm when not set', () => {
    const input = toContactRecordInput({ firstname: 'Marie' });
    expect(input.crm.kind).toBeUndefined();
  });
});

describe('getContactsByUid', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  test('short-circuits on empty input without calling fetch', async () => {
    const result = await getContactsByUid([]);
    expect(result.size).toBe(0);
    expect(fetch).not.toHaveBeenCalled();
  });

  test('resolves a batch of uids via the ?vcard_uid= filter in one request', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        contacts: [
          { id: 1, uid: 'alice-uid', firstname: 'Alice', lastname: 'Anderson', nickname: '', fn: '', primary_email: '', primary_phone: '', birthday: '', org: '', photo: '', photo_thumbnail: '', circles: [], archived: false },
          { id: 2, uid: 'bob-uid', firstname: 'Bob', lastname: 'Brown', nickname: '', fn: '', primary_email: '', primary_phone: '', birthday: '', org: '', photo: '', photo_thumbnail: '', circles: [], archived: false },
        ],
        total: 2,
        page: 1,
        limit: 2,
      }),
    });

    const result = await getContactsByUid(['alice-uid', 'bob-uid']);

    expect(fetch).toHaveBeenCalledTimes(1);
    const calledUrl = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(calledUrl).toContain('vcard_uid=alice-uid');
    expect(calledUrl).toContain('vcard_uid=bob-uid');
    expect(calledUrl).toContain('include_archived=true');

    expect(result.size).toBe(2);
    expect(result.get('alice-uid')?.firstname).toBe('Alice');
    expect(result.get('bob-uid')?.firstname).toBe('Bob');
  });

  test('filters out falsy uids before requesting', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ contacts: [], total: 0, page: 1, limit: 0 }),
    });

    await getContactsByUid(['alice-uid', '', undefined as unknown as string]);

    const calledUrl = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    // Only the one real uid should appear as a query param.
    expect((calledUrl.match(/vcard_uid=/g) || []).length).toBe(1);
  });
});

describe('getContacts cursor pagination (T17)', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  const summary = (id: number, firstname: string) => ({
    id, uid: `uid-${id}`, firstname, lastname: '', nickname: '', fn: firstname,
    primary_email: '', primary_phone: '', birthday: '', org: '',
    photo: '', photo_thumbnail: '', circles: [], archived: false,
  });

  test('sends limit/cursor/order params and reads next_cursor (no page/total)', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        contacts: [summary(1, 'Alice')],
        next_cursor: 'CURSOR-1',
        limit: 10,
      }),
    });

    const result = await getContacts({ cursor: 'PREV', limit: 10, order: 'asc', search: 'ali', includeArchived: true });

    const calledUrl = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(calledUrl).toContain('limit=10');
    expect(calledUrl).toContain('cursor=PREV');
    expect(calledUrl).toContain('order=asc');
    expect(calledUrl).toContain('search=ali');
    expect(calledUrl).toContain('include_archived=true');
    expect(calledUrl).not.toContain('page=');

    expect(result.contacts[0].firstname).toBe('Alice');
    expect(result.next_cursor).toBe('CURSOR-1');
    expect(result.limit).toBe(10);
  });

  test('getAllContacts follows next_cursor until it is empty', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ contacts: [summary(1, 'Alice')], next_cursor: 'CURSOR-2', limit: 25 }),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ contacts: [summary(2, 'Bob')], next_cursor: '', limit: 25 }),
      });

    const all = await getAllContacts({ limit: 25 });

    expect(all.map((c) => c.firstname)).toEqual(['Alice', 'Bob']);
    expect((fetch as unknown as ReturnType<typeof vi.fn>)).toHaveBeenCalledTimes(2);
    const secondUrl = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[1][0] as string;
    expect(secondUrl).toContain('cursor=CURSOR-2');
  });
});
