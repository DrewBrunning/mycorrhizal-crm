import { describe, expect, test } from 'vitest';
import { describeJSContact } from './jscontactDisplay';

describe('describeJSContact', () => {
  test('uses name.full, else joins the name components in order', () => {
    expect(describeJSContact({ name: { full: 'Alice Anderson' } }).name).toBe('Alice Anderson');
    expect(
      describeJSContact({
        name: {
          components: [
            { kind: 'given', value: 'John' },
            { kind: 'surname', value: 'Doe' },
          ],
        },
      }).name,
    ).toBe('John Doe');
    expect(describeJSContact({}).name).toBe('');
  });

  test('summarises each known section by its section token', () => {
    const out = describeJSContact({
      emails: { e1: { address: 'a@example.com' } },
      phones: { p1: { number: '+1 555 0100', label: 'Home' } },
      addresses: { a1: { full: '1 Main St, Springfield' } },
      organizations: { o1: { name: 'Acme' } },
      links: { l1: { uri: 'https://example.com' } },
      onlineServices: { s1: { service: 'Mastodon', user: '@a' } },
      notes: { n1: { note: 'Met at a conference' } },
      keywords: { friend: true, golf: true },
      personalInfo: { i1: { kind: 'hobby', value: 'Chess' } },
    });
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
    const out = describeJSContact({
      '@type': 'Card',
      uid: 'secret-uid',
      version: '1.0',
      'crm:secret': { x: 'y' },
      secretStuff: { a: { value: 'leak' } },
      emails: { e1: { address: 'a@example.com' } },
    });
    expect(out.sections.map((s) => s.token)).toEqual(['emails']);
    expect(JSON.stringify(out)).not.toContain('leak');
    expect(JSON.stringify(out)).not.toContain('secret-uid');
  });

  test('a section with no displayable items is omitted', () => {
    expect(describeJSContact({ emails: {} }).sections).toEqual([]);
    expect(describeJSContact({ emails: { e1: {} } }).sections).toEqual([]);
  });

  test('tolerates malformed shapes without throwing', () => {
    expect(() =>
      describeJSContact({ emails: 'nope', phones: [1, 2], name: 5, notes: { n: null } }),
    ).not.toThrow();
  });
});
