import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import {
  addressesToCardAndPeriods,
  archiveContact,
  type CardAddress,
  type CardEntryPeriod,
  cardAddressesToValues,
  cardEmailsToValues,
  cardImppToValues,
  cardLinksToValues,
  cardPhonesToValues,
  createContactRecord,
  deleteContact,
  favoriteContact,
  formatAnniversaryDate,
  getAllContacts,
  getAnniversaryField,
  getCircles,
  getContactDisplayName,
  getContactProfilePicture,
  getContactRecord,
  getContacts,
  getContactsByLegacyCircle,
  getContactsByUid,
  getLegacyCircles,
  getOrganizationFields,
  getRandomContacts,
  getTitleField,
  getUpcomingBirthdays,
  nameComponentValue,
  onlineServicesToRows,
  organizationEntry,
  parseAnniversaryDate,
  rowsToOnlineServices,
  summaryToLegacyContact,
  titleEntry,
  toContactRecordInput,
  unarchiveContact,
  unfavoriteContact,
  updateContactRecord,
  uploadProfilePicture,
  upsertEntryPeriod,
  valuesToCardAddresses,
  valuesToCardEmails,
  valuesToCardImpp,
  valuesToCardLinks,
  valuesToCardPhones,
  withAnniversary,
  withOrganization,
  withOrganizationEntry,
  withTitleEntry,
  withTitles,
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
    expect(
      valuesToCardEmails([
        { type: 'home', value: '  ' },
        { type: '', value: 'a@b.com' },
      ]),
    ).toEqual([{ address: 'a@b.com', contexts: undefined }]);
  });

  test('handles an empty/undefined array', () => {
    expect(cardEmailsToValues(undefined)).toEqual([]);
  });

  test('preserves pref and label through the round trip', () => {
    const card = [
      { address: 'work@example.com', contexts: ['work', 'private'], pref: 1, label: 'Main' },
    ];
    const values = cardEmailsToValues(card);
    expect(values[0].pref).toBe(1);
    expect(values[0].label).toBe('Main');
    expect(values[0].contexts).toEqual(['work', 'private']);
    expect(valuesToCardEmails(values)).toEqual(card);
  });
});

describe('phone conversion', () => {
  test('display prefers features over contexts (vCard feature tokens like cell/fax)', () => {
    expect(
      cardPhonesToValues([{ number: '555-1234', features: ['cell'], contexts: ['work'] }]),
    ).toEqual([{ type: 'cell', value: '555-1234', features: ['cell'], contexts: ['work'] }]);
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

  test('preserves features and pref through the round trip', () => {
    const card = [
      {
        number: '555-1234',
        features: ['cell', 'text'],
        contexts: ['work'],
        pref: 2,
        label: 'Work cell',
      },
    ];
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
      {
        type: 'home',
        street: '123 Main St',
        city: 'Springfield',
        region: 'IL',
        postal: '62704',
        country: 'USA',
      },
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
    expect(cardAddressesToValues([{ components: [{ kind: 'name', value: 'x' }] }])[0].type).toBe(
      '',
    );
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
    expect(resultComps).toEqual(
      expect.arrayContaining([
        { kind: 'name', value: '123 Main St' },
        { kind: 'room', value: 'Loft' },
        { kind: 'building', value: 'North Tower' },
        { kind: 'locality', value: 'Springfield' },
        { kind: 'region', value: 'IL' },
        { kind: 'postcode', value: '62704' },
        { kind: 'country', value: 'USA' },
      ]),
    );
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
    expect(resultComps).toEqual(
      expect.arrayContaining([
        { kind: 'name', value: '123 Main St' },
        { kind: 'postOfficeBox', value: 'PO Box 42' },
        { kind: 'apartment', value: '3B' },
        { kind: 'floor', value: '4' },
        { kind: 'locality', value: 'Springfield' },
        { kind: 'region', value: 'IL' },
        { kind: 'postcode', value: '62704' },
        { kind: 'country', value: 'USA' },
      ]),
    );
    expect(result[0].contexts).toEqual(['home']);
  });

  test('keeps an address whose only non-blank part is a sub-street field (T79)', () => {
    expect(
      valuesToCardAddresses([
        {
          type: 'home',
          street: '',
          city: '',
          region: '',
          postal: '',
          country: '',
          pobox: 'PO Box 42',
        },
      ]),
    ).toEqual([
      { components: [{ kind: 'postOfficeBox', value: 'PO Box 42' }], contexts: ['home'] },
    ]);
  });

  test('drops an address with every field blank', () => {
    expect(
      valuesToCardAddresses([
        { type: 'home', street: '', city: '', region: '', postal: '', country: '' },
      ]),
    ).toEqual([]);
  });

  test('preserves coordinates, timeZone, pref and full through the round trip', () => {
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

  test('ADR 0025: surfaces a period attached to an address by its element ID', () => {
    const card: CardAddress[] = [
      { id: 'addr-1', components: [{ kind: 'name', value: '1 Main St' }] },
    ];
    const periods: CardEntryPeriod[] = [
      {
        kind: 'address',
        entry_id: 'addr-1',
        range: { start: { year: 2019 }, end: { year: 2024 } },
      },
    ];
    const values = cardAddressesToValues(card, periods);
    expect(values[0].id).toBe('addr-1');
    expect(values[0].periodStartYear).toBe('2019');
    expect(values[0].periodEndYear).toBe('2024');
    // A period for a different entry must not leak onto this address.
    const other = cardAddressesToValues(card, [
      { kind: 'address', entry_id: 'other', range: { start: { year: 2000 } } },
    ]);
    expect(other[0].periodStartYear).toBeUndefined();
  });

  test('ADR 0025: addressesToCardAndPeriods round-trips an address period', () => {
    const { addresses, periods } = addressesToCardAndPeriods([
      {
        id: 'addr-1',
        type: 'home',
        street: '1 Main St',
        city: '',
        region: '',
        postal: '',
        country: '',
        periodStartYear: '2019',
        periodEndYear: '2024',
      },
    ]);
    expect(addresses[0].id).toBe('addr-1');
    expect(periods).toEqual([
      {
        kind: 'address',
        entry_id: 'addr-1',
        range: { start: { year: 2019 }, end: { year: 2024 } },
      },
    ]);
  });

  test('ADR 0025: assigns a stable id to a new address that carries a period', () => {
    const { addresses, periods } = addressesToCardAndPeriods([
      {
        type: 'home',
        street: '9 New Rd',
        city: '',
        region: '',
        postal: '',
        country: '',
        periodStartYear: '2020',
      },
    ]);
    expect(addresses[0].id).toBeTruthy();
    expect(periods[0].entry_id).toBe(addresses[0].id);
    expect(periods[0].range).toEqual({ start: { year: 2020 } });
  });

  test('ADR 0025: no period fields means no periods, and an existing id survives', () => {
    const { addresses, periods } = addressesToCardAndPeriods([
      {
        id: 'addr-9',
        type: 'home',
        street: '2 Elm St',
        city: '',
        region: '',
        postal: '',
        country: '',
      },
    ]);
    expect(addresses[0].id).toBe('addr-9');
    expect(periods).toEqual([]);
  });
});

describe('online service conversion', () => {
  test('round-trips social profile rows with service/uri/user', () => {
    const rows = onlineServicesToRows([
      {
        service: 'Mastodon',
        uri: 'https://mastodon.social/@ada',
        user: '@ada',
        contexts: ['work'],
        pref: 1,
        label: 'Work',
      },
    ]);
    expect(rows).toEqual([
      {
        service: 'Mastodon',
        uri: 'https://mastodon.social/@ada',
        user: '@ada',
        contexts: ['work'],
        pref: 1,
        label: 'Work',
      },
    ]);
    expect(rowsToOnlineServices(rows)).toEqual([
      {
        service: 'Mastodon',
        uri: 'https://mastodon.social/@ada',
        user: '@ada',
        contexts: ['work'],
        pref: 1,
        label: 'Work',
      },
    ]);
  });

  test('drops empty rows', () => {
    expect(
      rowsToOnlineServices([{ service: '', uri: '', user: '', label: '', contexts: [] }]),
    ).toEqual([]);
  });

  test('omits blank fields', () => {
    expect(
      rowsToOnlineServices([{ service: 'GitHub', uri: '', user: '', label: '', contexts: [] }]),
    ).toEqual([{ service: 'GitHub' }]);
  });
});

describe('anniversary date formatting', () => {
  test('formats a full date', () => {
    expect(formatAnniversaryDate({ partial: { year: 1990, month: 3, day: 15 } })).toBe(
      '1990-03-15',
    );
  });

  test('formats a year-less date', () => {
    expect(formatAnniversaryDate({ partial: { month: 3, day: 15 } })).toBe('--03-15');
  });

  test('parses both formats back losslessly', () => {
    expect(parseAnniversaryDate('1990-03-15')).toEqual({
      partial: { year: 1990, month: 3, day: 15 },
    });
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
    const anniversaries = [
      { kind: 'birth' as const, date: { partial: { year: 1990, month: 3, day: 15 } } },
    ];
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

  test('organizationEntry returns the first organization', () => {
    expect(organizationEntry(undefined)).toBeUndefined();
    expect(organizationEntry([{ name: 'Acme' }, { name: 'Globex' }])).toEqual({ name: 'Acme' });
  });

  // ADR 0025 (#1233): the edit path must not drop the entry ID a period
  // references.
  test('withOrganizationEntry preserves the entry ID and extra fields', () => {
    const updated = withOrganizationEntry(
      [{ id: 'org-1', name: 'Acme', units: [{ name: 'R&D' }], sortAs: 'Acme Inc' }],
      'Globex',
      'Research',
    );
    expect(updated).toEqual([
      { id: 'org-1', name: 'Globex', units: [{ name: 'Research' }], sortAs: 'Acme Inc' },
    ]);
  });

  test('withOrganizationEntry keeps organizations beyond the first untouched', () => {
    const updated = withOrganizationEntry(
      [
        { id: 'org-1', name: 'Acme' },
        { id: 'org-2', name: 'Side Co' },
      ],
      'Globex',
      '',
    );
    expect(updated[1]).toEqual({ id: 'org-2', name: 'Side Co' });
  });

  test('withOrganizationEntry drops the first entry on a blank name', () => {
    expect(withOrganizationEntry([{ id: 'org-1', name: 'Acme' }], '', '')).toEqual([]);
  });
});

describe('title fields', () => {
  test('getTitleField distinguishes title from role by kind', () => {
    const titles = [
      { name: 'Engineer', kind: 'title' as const },
      { name: 'Lead', kind: 'role' as const },
    ];
    expect(getTitleField(titles, 'title')).toBe('Engineer');
    expect(getTitleField(titles, 'role')).toBe('Lead');
  });

  test('withTitles preserves role when only job title changes', () => {
    const current = { role: getTitleField([{ name: 'Lead', kind: 'role' }], 'role') };
    const updated = withTitles('Senior Engineer', current.role || '');
    expect(getTitleField(updated, 'title')).toBe('Senior Engineer');
    expect(getTitleField(updated, 'role')).toBe('Lead');
  });

  test('titleEntry finds the job title even when kind is omitted', () => {
    expect(titleEntry([{ id: 't1', name: 'Engineer' }], 'title')).toEqual({
      id: 't1',
      name: 'Engineer',
    });
    expect(titleEntry([{ id: 'r1', name: 'Lead', kind: 'role' }], 'title')).toBeUndefined();
  });

  // ADR 0025 (#1233): the edit path must preserve the title entry's ID and
  // organizationId so a period attached to it survives the edit.
  test('withTitleEntry preserves the ID and leaves the other kind alone', () => {
    const titles = [
      { id: 't1', name: 'Engineer', kind: 'title' as const, organizationId: 'org-1' },
      { id: 'r1', name: 'Lead', kind: 'role' as const },
    ];
    const updated = withTitleEntry(titles, 'Senior Engineer', 'title');
    expect(updated).toEqual([
      { id: 't1', name: 'Senior Engineer', kind: 'title', organizationId: 'org-1' },
      { id: 'r1', name: 'Lead', kind: 'role' },
    ]);
  });

  test('withTitleEntry appends a missing entry and drops one left blank', () => {
    expect(withTitleEntry([], 'Engineer', 'title')).toEqual([{ name: 'Engineer', kind: 'title' }]);
    expect(withTitleEntry([{ id: 't1', name: 'Engineer', kind: 'title' }], '', 'title')).toEqual(
      [],
    );
  });
});

describe('entry periods', () => {
  const range = { start: { year: 2019 }, end: { year: 2024 } };

  test('upsertEntryPeriod adds a period keyed by kind + entry', () => {
    expect(upsertEntryPeriod(undefined, 'organization', 'org-1', range)).toEqual([
      { kind: 'organization', entry_id: 'org-1', range },
    ]);
  });

  test('upsertEntryPeriod replaces the matching period and preserves others', () => {
    const existing: CardEntryPeriod[] = [
      { kind: 'address', entry_id: 'addr-1', range: { start: { year: 2000 } } },
      { kind: 'organization', entry_id: 'org-1', range: { start: { year: 2010 } } },
    ];
    const updated = upsertEntryPeriod(existing, 'organization', 'org-1', range);
    expect(updated).toEqual([
      { kind: 'address', entry_id: 'addr-1', range: { start: { year: 2000 } } },
      { kind: 'organization', entry_id: 'org-1', range },
    ]);
  });

  test('upsertEntryPeriod clears the matching period for an empty range', () => {
    const existing: CardEntryPeriod[] = [{ kind: 'organization', entry_id: 'org-1', range }];
    expect(upsertEntryPeriod(existing, 'organization', 'org-1', undefined)).toEqual([]);
    expect(upsertEntryPeriod(existing, 'organization', 'org-1', {})).toEqual([]);
  });
});

describe('toContactRecordInput', () => {
  // toLegacyContact/getContact/createContact/updateContact were retired once
  // every contact-editing component migrated onto getContactRecord/
  // updateContactRecord/createContactRecord -- toContactRecordInput itself
  // survives only for e2e test fixtures
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
    expect(input.card.organizations).toEqual([
      { name: 'Sorbonne University', units: [{ name: 'Physics' }] },
    ]);
    expect(input.card.titles).toEqual([
      { name: 'Professor', kind: 'title' },
      { name: 'Nobel Laureate', kind: 'role' },
    ]);
    expect(input.card.anniversaries).toEqual([
      { kind: 'birth', date: { partial: { year: 1867, month: 11, day: 7 } } },
    ]);
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
          {
            id: 1,
            uid: 'alice-uid',
            firstname: 'Alice',
            lastname: 'Anderson',
            nickname: '',
            fn: '',
            primary_email: '',
            primary_phone: '',
            birthday: '',
            org: '',
            photo: '',
            photo_thumbnail: '',
            circles: [],
            archived: false,
          },
          {
            id: 2,
            uid: 'bob-uid',
            firstname: 'Bob',
            lastname: 'Brown',
            nickname: '',
            fn: '',
            primary_email: '',
            primary_phone: '',
            birthday: '',
            org: '',
            photo: '',
            photo_thumbnail: '',
            circles: [],
            archived: false,
          },
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
    id,
    uid: `uid-${id}`,
    firstname,
    lastname: '',
    nickname: '',
    fn: firstname,
    primary_email: '',
    primary_phone: '',
    birthday: '',
    org: '',
    photo: '',
    photo_thumbnail: '',
    circles: [],
    archived: false,
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

    const result = await getContacts({
      cursor: 'PREV',
      limit: 10,
      order: 'asc',
      search: 'ali',
      includeArchived: true,
    });

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
    expect(fetch as unknown as ReturnType<typeof vi.fn>).toHaveBeenCalledTimes(2);
    const secondUrl = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[1][0] as string;
    expect(secondUrl).toContain('cursor=CURSOR-2');
  });
});

// --- Issue #173 favorites ----------------------------------------------------

describe('favorites', () => {
  test('favorites=true is appended to the list URL', async () => {
    vi.stubGlobal('fetch', vi.fn());
    try {
      (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
        ok: true,
        json: async () => ({ contacts: [], next_cursor: '', limit: 25 }),
      });

      await getContacts({ favorites: true });

      const calledUrl = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
      expect(calledUrl).toContain('favorites=true');
    } finally {
      vi.unstubAllGlobals();
    }
  });

  test('is_favorite maps through summaryToLegacyContact', () => {
    const contact = summaryToLegacyContact({
      id: 1,
      uid: 'u',
      firstname: 'Alice',
      lastname: '',
      nickname: '',
      fn: 'Alice',
      primary_email: '',
      primary_phone: '',
      birthday: '',
      org: '',
      photo: '',
      photo_thumbnail: '',
      archived: false,
      is_favorite: true,
      revision: 1,
    });
    expect(contact.is_favorite).toBe(true);

    const plain = summaryToLegacyContact({
      id: 2,
      uid: 'v',
      firstname: 'Bob',
      lastname: '',
      nickname: '',
      fn: 'Bob',
      primary_email: '',
      primary_phone: '',
      birthday: '',
      org: '',
      photo: '',
      photo_thumbnail: '',
      archived: false,
      is_favorite: false,
      revision: 1,
    });
    expect(plain.is_favorite).toBe(false);
  });

  test('favoriteContact POSTs to the favorite endpoint', async () => {
    vi.stubGlobal('fetch', vi.fn());
    try {
      (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
        ok: true,
        json: async () => ({ ID: 7, is_favorite: true }),
      });

      const result = await favoriteContact(7);

      expect(fetch).toHaveBeenCalledTimes(1);
      const [url, init] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
      expect(url).toContain('/contacts/7/favorite');
      expect(init.method).toBe('POST');
      expect(result.is_favorite).toBe(true);
    } finally {
      vi.unstubAllGlobals();
    }
  });

  test('unfavoriteContact POSTs to the unfavorite endpoint', async () => {
    vi.stubGlobal('fetch', vi.fn());
    try {
      (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
        ok: true,
        json: async () => ({ ID: 7, is_favorite: false }),
      });

      const result = await unfavoriteContact(7);

      const [url, init] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
      expect(url).toContain('/contacts/7/unfavorite');
      expect(init.method).toBe('POST');
      expect(result.is_favorite).toBe(false);
    } finally {
      vi.unstubAllGlobals();
    }
  });
});

// --- link / IMPP conversion --------------------------------------------------

describe('link conversion', () => {
  test('round-trips links with contexts', () => {
    const links = [{ uri: 'https://example.com', contexts: ['work'], pref: 1, label: 'Site' }];
    const values = cardLinksToValues(links);
    expect(values).toEqual([
      { type: 'work', value: 'https://example.com', pref: 1, label: 'Site', contexts: ['work'] },
    ]);
    expect(valuesToCardLinks(values)).toEqual([
      { uri: 'https://example.com', contexts: ['work'], pref: 1, label: 'Site' },
    ]);
  });

  test('drops rows with an empty value when converting back', () => {
    expect(valuesToCardLinks([{ type: '', value: '   ', contexts: [] }])).toEqual([]);
  });

  test('handles an empty/undefined array', () => {
    expect(cardLinksToValues(undefined)).toEqual([]);
    expect(cardLinksToValues([])).toEqual([]);
  });

  test('falls back to type when contexts is empty on write', () => {
    expect(valuesToCardLinks([{ type: 'home', value: 'https://x.example', contexts: [] }])).toEqual(
      [{ uri: 'https://x.example', contexts: ['home'], pref: undefined, label: undefined }],
    );
  });
});

describe('IMPP conversion', () => {
  test('round-trips impp addresses with contexts', () => {
    const impps = [
      { uri: 'xmpp:alice@example.com', contexts: ['work'], pref: 2, label: 'Work IM' },
    ];
    const values = cardImppToValues(impps);
    expect(values).toEqual([
      {
        type: 'work',
        value: 'xmpp:alice@example.com',
        pref: 2,
        label: 'Work IM',
        contexts: ['work'],
      },
    ]);
    expect(valuesToCardImpp(values)).toEqual([
      { uri: 'xmpp:alice@example.com', contexts: ['work'], pref: 2, label: 'Work IM' },
    ]);
  });

  test('defaults an absent uri to an empty string on read', () => {
    expect(cardImppToValues([{ contexts: [] }])).toEqual([
      { type: '', value: '', pref: undefined, label: undefined, contexts: [] },
    ]);
  });

  test('drops rows with an empty value when converting back', () => {
    expect(valuesToCardImpp([{ type: '', value: '', contexts: [] }])).toEqual([]);
  });

  test('handles an empty/undefined array', () => {
    expect(cardImppToValues(undefined)).toEqual([]);
  });
});

// --- name assembly ------------------------------------------------------------

describe('nameComponentValue', () => {
  test('finds the component matching the requested kind', () => {
    const components = [
      { kind: 'given' as const, value: 'Marie' },
      { kind: 'surname' as const, value: 'Curie' },
    ];
    expect(nameComponentValue(components, 'given')).toBe('Marie');
    expect(nameComponentValue(components, 'surname')).toBe('Curie');
  });

  test('returns undefined when the kind is absent', () => {
    expect(
      nameComponentValue([{ kind: 'given' as const, value: 'Marie' }], 'surname'),
    ).toBeUndefined();
  });

  test('returns undefined for an undefined components array', () => {
    expect(nameComponentValue(undefined, 'given')).toBeUndefined();
  });
});

describe('getContactDisplayName', () => {
  test('assembles prefix, given, quoted nickname, middle, surname, suffix in order', () => {
    const record = {
      card: {
        name: {
          components: [
            { kind: 'title' as const, value: 'Dr.' },
            { kind: 'given' as const, value: 'Marie' },
            { kind: 'given2' as const, value: 'Salomea' },
            { kind: 'surname' as const, value: 'Curie' },
            { kind: 'generation' as const, value: 'PhD' },
          ],
        },
        nicknames: [{ name: 'Manya' }],
      },
    };
    expect(getContactDisplayName(record)).toBe('Dr. Marie "Manya" Salomea Curie PhD');
  });

  test('skips missing parts without leaving extra whitespace', () => {
    const record = { card: { name: { components: [{ kind: 'given' as const, value: 'Bob' }] } } };
    expect(getContactDisplayName(record)).toBe('Bob');
  });

  test('returns an empty string when there is no name data at all', () => {
    expect(getContactDisplayName({ card: {} })).toBe('');
  });

  test('handles a missing card entirely', () => {
    expect(getContactDisplayName({ card: undefined as unknown as never })).toBe('');
  });
});

// --- anniversary date formatting edge cases ------------------------------------

describe('formatAnniversaryDate edge cases', () => {
  test('returns undefined for an undefined date', () => {
    expect(formatAnniversaryDate(undefined)).toBeUndefined();
  });

  test('falls back to the raw timestamp slice when there is no usable partial', () => {
    expect(formatAnniversaryDate({ timestamp: '2019-06-01T00:00:00Z' })).toBe('2019-06-01');
  });

  test('returns undefined when neither partial nor timestamp is usable', () => {
    expect(formatAnniversaryDate({ partial: { year: 1990 } })).toBeUndefined();
  });

  test('returns undefined for a completely empty date object', () => {
    expect(formatAnniversaryDate({})).toBeUndefined();
  });
});

describe('parseAnniversaryDate edge cases', () => {
  test('passes an unparseable value through as a raw timestamp', () => {
    expect(parseAnniversaryDate('not-a-date')).toEqual({ timestamp: 'not-a-date' });
  });

  test('parses a full YYYY-MM-DD date', () => {
    expect(parseAnniversaryDate('2024-03-01')).toEqual({
      partial: { year: 2024, month: 3, day: 1 },
    });
  });
});

// --- organization / title field edge cases -------------------------------------

describe('organization fields edge cases', () => {
  test('returns undefined organization/department when the list is empty', () => {
    expect(getOrganizationFields([])).toEqual({ organization: undefined, department: undefined });
  });

  test('returns undefined organization/department when undefined', () => {
    expect(getOrganizationFields(undefined)).toEqual({
      organization: undefined,
      department: undefined,
    });
  });

  test('withOrganization omits units when department is blank', () => {
    expect(withOrganization('Acme', '')).toEqual([{ name: 'Acme', units: undefined }]);
  });
});

describe('title fields edge cases', () => {
  test('getTitleField(title) matches an entry with no kind at all (legacy data)', () => {
    expect(getTitleField([{ name: 'Legacy Title' }], 'title')).toBe('Legacy Title');
  });

  test('getTitleField returns undefined when nothing matches', () => {
    expect(getTitleField([], 'title')).toBeUndefined();
    expect(getTitleField(undefined, 'role')).toBeUndefined();
  });

  test('withTitles returns an empty array when both inputs are blank', () => {
    expect(withTitles('', '')).toEqual([]);
  });

  test('withTitles adds only the role when job title is blank', () => {
    expect(withTitles('', 'Lead')).toEqual([{ name: 'Lead', kind: 'role' }]);
  });
});

// --- toContactRecordInput edge cases --------------------------------------------

describe('toContactRecordInput edge cases', () => {
  test('falls back to the flat email/phone/address fields when the array fields are absent', () => {
    const input = toContactRecordInput({
      firstname: 'Bob',
      email: 'bob@example.com',
      phone: '555-0199',
      address: '123 Main St',
    });
    expect(input.card.emails).toEqual([{ address: 'bob@example.com', contexts: undefined }]);
    expect(input.card.phones).toEqual([{ number: '555-0199', contexts: undefined }]);
    expect(input.card.addresses?.[0]?.components).toEqual([{ kind: 'name', value: '123 Main St' }]);
  });

  test('ignores the flat email/phone/address fields when the array fields are present', () => {
    const input = toContactRecordInput({
      firstname: 'Bob',
      email: 'ignored@example.com',
      emails: [{ type: 'work', value: 'real@example.com' }],
    });
    expect(input.card.emails).toEqual([{ address: 'real@example.com', contexts: ['work'] }]);
  });

  test('an empty emails/phones/addresses array falls back to the flat field, not to nothing', () => {
    const input = toContactRecordInput({ firstname: 'Bob', emails: [], email: 'bob@example.com' });
    expect(input.card.emails).toEqual([{ address: 'bob@example.com', contexts: undefined }]);
  });

  test('produces no card sub-fields for a completely bare input', () => {
    const input = toContactRecordInput({});
    expect(input.card.name).toBeUndefined();
    expect(input.card.nicknames).toBeUndefined();
    expect(input.card.emails).toBeUndefined();
    expect(input.card.phones).toBeUndefined();
    expect(input.card.links).toBeUndefined();
    expect(input.card.imppAddresses).toBeUndefined();
    expect(input.card.addresses).toBeUndefined();
    expect(input.card.anniversaries).toBeUndefined();
    expect(input.card.organizations).toBeUndefined();
    expect(input.card.titles).toBeUndefined();
    expect(input.crm.kind).toBeUndefined();
  });
});

// --- summaryToLegacyContact falsy-to-undefined mapping --------------------------

describe('summaryToLegacyContact edge cases', () => {
  const base = {
    id: 3,
    uid: 'u3',
    firstname: 'Cara',
    lastname: 'Cee',
    nickname: '',
    fn: 'Cara Cee',
    primary_email: '',
    primary_phone: '',
    birthday: '',
    org: '',
    photo: '',
    archived: false,
    is_favorite: false,
    revision: 1,
  };

  test('maps every empty-string wire field to undefined', () => {
    const contact = summaryToLegacyContact(base);
    expect(contact.nickname).toBeUndefined();
    expect(contact.email).toBeUndefined();
    expect(contact.phone).toBeUndefined();
    expect(contact.birthday).toBeUndefined();
    expect(contact.photo).toBeUndefined();
    expect(contact.photo_thumbnail).toBeUndefined();
    expect(contact.organization).toBeUndefined();
  });

  test('preserves non-empty wire fields as-is', () => {
    const contact = summaryToLegacyContact({
      ...base,
      nickname: 'Cee',
      primary_email: 'cara@example.com',
      primary_phone: '555-0100',
      birthday: '1990-01-01',
      photo: '/p.jpg',
      photo_thumbnail: '/p_thumb.jpg',
      org: 'Acme',
    });
    expect(contact.nickname).toBe('Cee');
    expect(contact.email).toBe('cara@example.com');
    expect(contact.phone).toBe('555-0100');
    expect(contact.birthday).toBe('1990-01-01');
    expect(contact.photo).toBe('/p.jpg');
    expect(contact.photo_thumbnail).toBe('/p_thumb.jpg');
    expect(contact.organization).toBe('Acme');
  });
});

// --- getContacts: remaining query params + error path ---------------------------

describe('getContacts additional params (T17/T103)', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  function okList() {
    return { ok: true, json: async () => ({ contacts: [], next_cursor: '', limit: 25 }) };
  }

  test('appends circle, sort, archived and has_contact_info when given', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce(okList());

    await getContacts({ circle: 'friends', sort: 'name', archived: true, hasContactInfo: true });

    const calledUrl = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(calledUrl).toContain('circle=friends');
    expect(calledUrl).toContain('sort=name');
    expect(calledUrl).toContain('archived=true');
    expect(calledUrl).toContain('has_contact_info=true');
  });

  test('archived=false is still sent explicitly (distinct from "unset")', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce(okList());
    await getContacts({ archived: false });
    const calledUrl = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(calledUrl).toContain('archived=false');
  });

  test('omits optional params entirely when not given', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce(okList());
    await getContacts({});
    const calledUrl = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(calledUrl).not.toContain('cursor=');
    expect(calledUrl).not.toContain('search=');
    expect(calledUrl).not.toContain('circle=');
    expect(calledUrl).not.toContain('sort=');
    expect(calledUrl).not.toContain('order=');
    expect(calledUrl).not.toContain('include_archived=');
    expect(calledUrl).not.toContain('archived=');
    expect(calledUrl).not.toContain('favorites=');
    expect(calledUrl).not.toContain('has_contact_info=');
  });

  test('sends the auth headers', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce(okList());
    await getContacts({});
    const [, init] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(init.headers).toEqual({ 'Content-Type': 'application/json' });
  });

  test('carries hidden_count through when present', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ contacts: [], next_cursor: '', limit: 25, hidden_count: 4 }),
    });
    const result = await getContacts({ hasContactInfo: true });
    expect(result.hidden_count).toBe(4);
  });

  test('throws an ApiError when the response is not ok', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: false,
      status: 400,
      statusText: 'Bad Request',
      json: async () => ({
        error: { code: 'VALIDATION_ERROR', message: 'nope' },
        request_id: 'req-1',
      }),
    });
    await expect(getContacts({})).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});

// --- Card/CRM record CRUD endpoints, none previously covered --------------------

describe('contact record CRUD endpoints', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  function errorResp() {
    return {
      ok: false,
      status: 400,
      statusText: 'Bad Request',
      json: async () => ({
        error: { code: 'VALIDATION_ERROR', message: 'nope' },
        request_id: 'req-1',
      }),
    };
  }

  test('getContactRecord GETs /contacts/:id with auth headers', async () => {
    const record = { card: {}, crm: {} };
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      json: async () => record,
    });
    const result = await getContactRecord(42);
    const [url, init] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain('/contacts/42');
    expect(init.headers).toEqual({ 'Content-Type': 'application/json' });
    expect(result).toEqual(record);
  });

  test('getContactRecord throws an ApiError on failure', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce(errorResp());
    await expect(getContactRecord(42)).rejects.toMatchObject({ code: 'VALIDATION_ERROR' });
  });

  test('updateContactRecord PUTs the input body and returns the updated record', async () => {
    const input = { gender: 'other', card: {}, crm: {} };
    const updated = { card: {}, crm: {} };
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      json: async () => updated,
    });
    const result = await updateContactRecord(42, input);
    const [url, init] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain('/contacts/42');
    expect(init.method).toBe('PUT');
    expect(JSON.parse(init.body)).toEqual(input);
    expect(result).toEqual(updated);
  });

  test('updateContactRecord throws an ApiError on failure', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce(errorResp());
    await expect(updateContactRecord(42, { card: {}, crm: {} })).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
    });
  });

  test('createContactRecord POSTs the input and unwraps { contact }', async () => {
    const input = { card: {}, crm: {} };
    const created = { card: {}, crm: {} };
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ contact: created }),
    });
    const result = await createContactRecord(input);
    const [url, init] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain('/contacts');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual(input);
    expect(result).toEqual(created);
  });

  test('createContactRecord falls back to the raw body when not wrapped in { contact }', async () => {
    const created = { card: {}, crm: {} };
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      json: async () => created,
    });
    const result = await createContactRecord({ card: {}, crm: {} });
    expect(result).toEqual(created);
  });

  test('createContactRecord throws an ApiError on failure', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce(errorResp());
    await expect(createContactRecord({ card: {}, crm: {} })).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
    });
  });

  test('deleteContact DELETEs /contacts/:id', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      json: async () => ({}),
    });
    await deleteContact(42);
    const [url, init] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain('/contacts/42');
    expect(init.method).toBe('DELETE');
  });

  test('deleteContact throws an ApiError on failure', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce(errorResp());
    await expect(deleteContact(42)).rejects.toMatchObject({ code: 'VALIDATION_ERROR' });
  });
});

describe('profile picture endpoints', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  test('getContactProfilePicture GETs the plain URL by default', async () => {
    const blob = new Blob(['x']);
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      blob: async () => blob,
    });
    const result = await getContactProfilePicture(5);
    const [url] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain('/contacts/5/profile_picture');
    expect(url).not.toContain('thumbnail');
    expect(result).toBe(blob);
  });

  test('getContactProfilePicture appends ?thumbnail=true when requested', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      blob: async () => new Blob(['x']),
    });
    await getContactProfilePicture(5, true);
    const [url] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain('thumbnail=true');
  });

  test('getContactProfilePicture returns null (not a throw) when the response is not ok', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: false,
      status: 404,
    });
    const result = await getContactProfilePicture(5);
    expect(result).toBeNull();
  });

  test('uploadProfilePicture POSTs a FormData body with the photo field', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      json: async () => ({}),
    });
    const blob = new Blob(['data']);
    await uploadProfilePicture(5, blob);
    const [url, init] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain('/contacts/5/profile_picture');
    expect(init.method).toBe('POST');
    expect(init.body).toBeInstanceOf(FormData);
    const uploaded = (init.body as FormData).get('photo') as File;
    expect(uploaded.name).toBe('profile.jpg');
    expect(uploaded.size).toBe(blob.size);
  });

  test('uploadProfilePicture throws an ApiError on failure', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: false,
      status: 400,
      statusText: 'Bad Request',
      json: async () => ({ error: { code: 'VALIDATION_ERROR', message: 'nope' }, request_id: 'r' }),
    });
    await expect(uploadProfilePicture(5, new Blob(['x']))).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
    });
  });
});

describe('circles endpoints', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  test('getCircles GETs /contacts/circles and returns the array as-is', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      json: async () => ['friends', 'family'],
    });
    const result = await getCircles();
    const [url] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain('/contacts/circles');
    expect(url).not.toContain('legacy');
    expect(result).toEqual(['friends', 'family']);
  });

  test('getCircles returns [] when the backend does not return an array', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ not: 'an array' }),
    });
    expect(await getCircles()).toEqual([]);
  });

  test('getCircles throws an ApiError on failure', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: false,
      status: 500,
      statusText: 'Internal Server Error',
      json: async () => ({ error: { code: 'INTERNAL', message: 'boom' }, request_id: 'r' }),
    });
    await expect(getCircles()).rejects.toMatchObject({ code: 'INTERNAL' });
  });

  test('getLegacyCircles GETs /contacts/circles?legacy=true', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      json: async () => ['old-circle'],
    });
    const result = await getLegacyCircles();
    const [url] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain('/contacts/circles?legacy=true');
    expect(result).toEqual(['old-circle']);
  });

  test('getLegacyCircles returns [] when the backend does not return an array', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      json: async () => null,
    });
    expect(await getLegacyCircles()).toEqual([]);
  });

  test('getLegacyCircles throws an ApiError on failure', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: false,
      status: 500,
      statusText: 'Internal Server Error',
      json: async () => ({ error: { code: 'INTERNAL', message: 'boom' }, request_id: 'r' }),
    });
    await expect(getLegacyCircles()).rejects.toMatchObject({ code: 'INTERNAL' });
  });

  test('getContactsByLegacyCircle sends limit=500 and circle_legacy=<circle>', async () => {
    const payload = { contacts: [], total: 0 };
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      json: async () => payload,
    });
    const result = await getContactsByLegacyCircle('old-friends');
    const [url] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain('limit=500');
    expect(url).toContain('circle_legacy=old-friends');
    expect(result).toEqual(payload);
  });

  test('getContactsByLegacyCircle throws an ApiError on failure', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: false,
      status: 500,
      statusText: 'Internal Server Error',
      json: async () => ({ error: { code: 'INTERNAL', message: 'boom' }, request_id: 'r' }),
    });
    await expect(getContactsByLegacyCircle('x')).rejects.toMatchObject({ code: 'INTERNAL' });
  });
});

describe('random contacts / upcoming birthdays', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  test('getRandomContacts GETs /contacts/random and returns data.contacts', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ contacts: [{ ID: 1, firstname: 'Al' }] }),
    });
    const result = await getRandomContacts();
    const [url] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain('/contacts/random');
    expect(result).toEqual([{ ID: 1, firstname: 'Al' }]);
  });

  test('getRandomContacts returns [] when data.contacts is absent', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      json: async () => ({}),
    });
    expect(await getRandomContacts()).toEqual([]);
  });

  test('getRandomContacts throws an ApiError on failure', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: false,
      status: 500,
      statusText: 'Internal Server Error',
      json: async () => ({ error: { code: 'INTERNAL', message: 'boom' }, request_id: 'r' }),
    });
    await expect(getRandomContacts()).rejects.toMatchObject({ code: 'INTERNAL' });
  });

  test('getUpcomingBirthdays GETs /contacts/birthdays and returns data.birthdays', async () => {
    const bday = { type: 'contact' as const, name: 'Al', birthday: '01-01', contact_id: 1 };
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ birthdays: [bday] }),
    });
    const result = await getUpcomingBirthdays();
    const [url] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain('/contacts/birthdays');
    expect(result).toEqual([bday]);
  });

  test('getUpcomingBirthdays returns [] when data.birthdays is absent', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      json: async () => ({}),
    });
    expect(await getUpcomingBirthdays()).toEqual([]);
  });

  test('getUpcomingBirthdays throws an ApiError on failure', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: false,
      status: 500,
      statusText: 'Internal Server Error',
      json: async () => ({ error: { code: 'INTERNAL', message: 'boom' }, request_id: 'r' }),
    });
    await expect(getUpcomingBirthdays()).rejects.toMatchObject({ code: 'INTERNAL' });
  });
});

describe('archive / unarchive', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  test('archiveContact POSTs to /contacts/:id/archive', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ ID: 9, archived: true }),
    });
    const result = await archiveContact(9);
    const [url, init] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain('/contacts/9/archive');
    expect(init.method).toBe('POST');
    expect(result.archived).toBe(true);
  });

  test('archiveContact throws an ApiError on failure', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: false,
      status: 500,
      statusText: 'Internal Server Error',
      json: async () => ({ error: { code: 'INTERNAL', message: 'boom' }, request_id: 'r' }),
    });
    await expect(archiveContact(9)).rejects.toMatchObject({ code: 'INTERNAL' });
  });

  test('unarchiveContact POSTs to /contacts/:id/unarchive', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ ID: 9, archived: false }),
    });
    const result = await unarchiveContact(9);
    const [url, init] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain('/contacts/9/unarchive');
    expect(init.method).toBe('POST');
    expect(result.archived).toBe(false);
  });

  test('unarchiveContact throws an ApiError on failure', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: false,
      status: 500,
      statusText: 'Internal Server Error',
      json: async () => ({ error: { code: 'INTERNAL', message: 'boom' }, request_id: 'r' }),
    });
    await expect(unarchiveContact(9)).rejects.toMatchObject({ code: 'INTERNAL' });
  });
});
