import { afterEach, describe, expect, test } from 'vitest';
import { type DateFormat, formatDateWithFormat } from '../DateFormatProvider';
import i18n from '../i18n/config';
import { describeJSContact } from './jscontactDisplay';

const t = i18n.t.bind(i18n);

// Pull just the anniversary items out of a card's display model.
function anniversaryItems(card: Record<string, unknown>, dateFormat: DateFormat = 'eu') {
  return (
    describeJSContact(card, t, dateFormat).sections.find((s) => s.token === 'anniversaries')
      ?.items ?? []
  );
}

afterEach(async () => {
  await i18n.changeLanguage('en');
});

describe('describeJSContact', () => {
  test('uses name.full, else joins the name components in order', () => {
    expect(describeJSContact({ name: { full: 'Alice Anderson' } }, t, 'eu').name).toBe(
      'Alice Anderson',
    );
    expect(
      describeJSContact(
        {
          name: {
            components: [
              { kind: 'given', value: 'John' },
              { kind: 'surname', value: 'Doe' },
            ],
          },
        },
        t,
        'eu',
      ).name,
    ).toBe('John Doe');
    expect(describeJSContact({}, t, 'eu').name).toBe('');
  });

  test('summarises each known section by its section token', () => {
    const out = describeJSContact(
      {
        emails: { e1: { address: 'a@example.com' } },
        phones: { p1: { number: '+1 555 0100', label: 'Home' } },
        addresses: { a1: { full: '1 Main St, Springfield' } },
        organizations: { o1: { name: 'Acme' } },
        links: { l1: { uri: 'https://example.com' } },
        onlineServices: { s1: { service: 'Mastodon', user: '@a' } },
        notes: { n1: { note: 'Met at a conference' } },
        keywords: { friend: true, golf: true },
        personalInfo: { i1: { kind: 'hobby', value: 'Chess' } },
      },
      t,
      'eu',
    );
    const byToken = Object.fromEntries(out.sections.map((s) => [s.token, s.items]));
    expect(byToken.emails).toEqual(['a@example.com']);
    expect(byToken.phones).toEqual(['Home: +1 555 0100']);
    expect(byToken.addresses).toEqual(['1 Main St, Springfield']);
    expect(byToken.organizations).toEqual(['Acme']);
    expect(byToken.links).toEqual(['https://example.com']);
    expect(byToken.online_services).toEqual(['Mastodon: @a']);
    expect(byToken.notes).toEqual(['Met at a conference']);
    expect(byToken.keywords).toEqual(['friend', 'golf']);
    expect(byToken.personal_info).toEqual(['hobby: Chess']);
  });

  test('only renders an allowlist: unknown properties, ids and metadata never appear', () => {
    const out = describeJSContact(
      {
        '@type': 'Card',
        uid: 'secret-uid',
        version: '1.0',
        'crm:secret': { x: 'y' },
        secretStuff: { a: { value: 'leak' } },
        emails: { e1: { address: 'a@example.com' } },
      },
      t,
      'eu',
    );
    expect(out.sections.map((s) => s.token)).toEqual(['emails']);
    expect(JSON.stringify(out)).not.toContain('leak');
    expect(JSON.stringify(out)).not.toContain('secret-uid');
  });

  test('a section with no displayable items is omitted', () => {
    expect(describeJSContact({ emails: {} }, t, 'eu').sections).toEqual([]);
    expect(describeJSContact({ emails: { e1: {} } }, t, 'eu').sections).toEqual([]);
  });

  test('tolerates malformed shapes without throwing', () => {
    expect(() =>
      describeJSContact({ emails: 'nope', phones: [1, 2], name: 5, notes: { n: null } }, t, 'eu'),
    ).not.toThrow();
  });

  // #2908: a shared card leaked the raw JSContact kind and ISO date.
  test('renders an anniversary with the localized kind and the viewer date format (#2908)', async () => {
    const card = {
      anniversaries: { a1: { kind: 'birth', date: { year: 1985, month: 11, day: 28 } } },
    };
    const anniversaries = (out: ReturnType<typeof describeJSContact>) =>
      out.sections.find((s) => s.token === 'anniversaries')?.items;

    expect(anniversaries(describeJSContact(card, t, 'eu'))).toEqual(['Birthday: 28.11.1985']);

    await i18n.changeLanguage('de');
    expect(anniversaries(describeJSContact(card, t, 'eu'))).toEqual(['Geburtstag: 28.11.1985']);
  });

  test('pads single-digit month/day and formats a year-less anniversary (#2908)', () => {
    const out = describeJSContact(
      { anniversaries: { a1: { kind: 'birth', date: { month: 3, day: 4 } } } },
      t,
      'eu',
    );
    expect(out.sections.find((s) => s.token === 'anniversaries')?.items).toEqual([
      'Birthday: 04.03.',
    ]);
  });

  test('falls back to the raw kind when it has no translation (#2908)', () => {
    const out = describeJSContact(
      { anniversaries: { a1: { kind: 'anniversary', date: { year: 2000, month: 1, day: 2 } } } },
      t,
      'eu',
    );
    expect(out.sections.find((s) => s.token === 'anniversaries')?.items).toEqual([
      'anniversary: 02.01.2000',
    ]);
  });

  test('renders a utc anniversary date, with and without a kind (#2908)', () => {
    expect(
      anniversaryItems({ anniversaries: { a1: { kind: 'birth', date: { utc: '1990-05-06' } } } }),
    ).toEqual(['Birthday: 06.05.1990']);
    // No kind: the date renders alone rather than as "kind: date".
    expect(anniversaryItems({ anniversaries: { a1: { date: { utc: '2001-02-03' } } } })).toEqual([
      '03.02.2001',
    ]);
  });

  test('labels known kinds and keeps an unknown kind as the raw token (#2908)', () => {
    expect(
      anniversaryItems({
        anniversaries: { a1: { kind: 'death', date: { year: 2000, month: 1, day: 2 } } },
      }),
    ).toEqual(['Date of death: 02.01.2000']);
    expect(
      anniversaryItems({
        anniversaries: { a1: { kind: 'future-kind', date: { year: 2000, month: 1, day: 2 } } },
      }),
    ).toEqual(['future-kind: 02.01.2000']);
    // A full date with no kind renders as the date alone.
    expect(
      anniversaryItems({ anniversaries: { a1: { date: { year: 2000, month: 1, day: 2 } } } }),
    ).toEqual(['02.01.2000']);
  });

  test('omits an anniversary whose date is missing, empty or has no valid parts (#2908)', () => {
    // No date at all, an empty date object, non-numeric/negative parts, and a
    // date supplied as a bare string instead of a JSContact date object.
    expect(anniversaryItems({ anniversaries: { a1: { kind: 'birth' } } })).toEqual([]);
    expect(anniversaryItems({ anniversaries: { a1: { kind: 'birth', date: {} } } })).toEqual([]);
    expect(
      anniversaryItems({
        anniversaries: { a1: { kind: 'birth', date: { year: Number.NaN, month: -1, day: 0 } } },
      }),
    ).toEqual([]);
    expect(
      anniversaryItems({ anniversaries: { a1: { kind: 'birth', date: '1985-11-28' } } }),
    ).toEqual([]);
  });

  test('passes a year-only anniversary through the date formatter (#2908)', () => {
    // With no month/day the partial parts are joined as "YYYY" and handed to
    // the formatter unchanged (it parses a bare year as its own date).
    expect(
      anniversaryItems({ anniversaries: { a1: { kind: 'birth', date: { year: 1985 } } } }),
    ).toEqual([`Birthday: ${formatDateWithFormat('1985', 'eu')}`]);
  });

  // Issue #3014: relatedTo relation keys are the RFC 6350 section 6.6.6
  // RELATED TYPE registry; they must read as labels, not raw tokens.
  describe('relatedTo relation labels (#3014)', () => {
    const REGISTRY = [
      'acquaintance',
      'agent',
      'child',
      'co-resident',
      'co-worker',
      'colleague',
      'contact',
      'crush',
      'date',
      'emergency',
      'friend',
      'kin',
      'me',
      'met',
      'muse',
      'neighbor',
      'parent',
      'sibling',
      'spouse',
      'sweetheart',
    ];
    const relatedItems = (relation: Record<string, boolean>) =>
      describeJSContact({ relatedTo: { 'urn:uuid:x': { relation } } }, t, 'eu').sections.find(
        (s) => s.token === 'related_to',
      )?.items ?? [];

    test('labels every registry key, none left as the raw token', () => {
      const labels = REGISTRY.map((k) => relatedItems({ [k]: true })[0]);
      for (const [i, label] of labels.entries()) {
        expect(label, REGISTRY[i]).not.toBe(REGISTRY[i]);
        expect(label, REGISTRY[i]).not.toMatch(/^standingShares\./);
      }
      expect(new Set(labels).size).toBe(REGISTRY.length);
      expect(relatedItems({ friend: true, 'co-worker': true })).toEqual(['Friend, Coworker']);
    });

    test('matches keys case-insensitively', () => {
      expect(relatedItems({ 'CO-Worker': true })).toEqual(['Coworker']);
    });

    test('humanizes an unknown or vendor key instead of showing it raw', () => {
      expect(relatedItems({ 'x-mentor': true })).toEqual(['X mentor']);
    });

    test('yields no label for an entry without a relation map', () => {
      const sections = describeJSContact(
        { relatedTo: { 'urn:uuid:x': {}, 'urn:uuid:y': { relation: 'friend' } } },
        t,
        'eu',
      ).sections;
      expect(sections.find((s) => s.token === 'related_to')?.items ?? []).toEqual([]);
    });

    test('renders German labels', async () => {
      await i18n.changeLanguage('de');
      expect(relatedItems({ friend: true, 'co-worker': true })).toEqual(['Freund, Arbeitskollege']);
    });
  });
});
