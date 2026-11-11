import { expect, test } from 'vitest';
import { activityHref, contactHref, noteHref, searchResultsHref } from './searchLinks';

test('searchResultsHref encodes the query and optionally selects a tab', () => {
  expect(searchResultsHref('a b&c')).toBe('/search?q=a%20b%26c');
  expect(searchResultsHref('alice', 'notes')).toBe('/search?q=alice&tab=notes');
  // The default tab is implicit.
  expect(searchResultsHref('alice', 'contacts')).toBe('/search?q=alice');
});

test('contactHref opens the contact detail page', () => {
  expect(contactHref(7)).toBe('/contacts/7');
});

test('noteHref opens the owning contact, or the notes page filtered by the query when unfiled', () => {
  expect(noteHref({ contact_id: 3 }, 'x')).toBe('/contacts/3');
  expect(noteHref({ contact_id: null }, 'a b')).toBe('/notes?search=a%20b');
  expect(noteHref({}, 'q')).toBe('/notes?search=q');
});

test('activityHref opens the activities page filtered by the query', () => {
  expect(activityHref('a&b')).toBe('/activities?search=a%26b');
});
