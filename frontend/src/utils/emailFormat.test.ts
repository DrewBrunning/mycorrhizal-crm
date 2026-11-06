import { expect, test } from 'vitest';
import { looksLikeEmail } from './emailFormat';

test.each([
  'alice@example.com',
  'a.b+tag@sub.example.co.uk',
  '  padded@example.com  ',
  'üser@exämple.de',
])('accepts %j', (v) => {
  expect(looksLikeEmail(v)).toBe(true);
});

test.each([
  'not-an-email',
  'missing@tld',
  '@example.com',
  'two@@example.com',
  'spa ce@example.com',
  'alice@',
])('rejects %j', (v) => {
  expect(looksLikeEmail(v)).toBe(false);
});

test('an empty value is not flagged (nothing to check yet)', () => {
  expect(looksLikeEmail('')).toBe(true);
  expect(looksLikeEmail('   ')).toBe(true);
});
