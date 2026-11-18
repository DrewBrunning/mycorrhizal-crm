import { describe, expect, test } from 'vitest';
import i18n from '../i18n/config';
import { matchReason } from './searchMatch';

const t = i18n.t.bind(i18n);

describe('matchReason (#2744)', () => {
  test('is nothing when matched_on is absent or empty', () => {
    expect(matchReason(t, undefined)).toBeUndefined();
    expect(matchReason(t, [])).toBeUndefined();
  });

  test('is nothing for a name-only hit: the title already says why it matched', () => {
    expect(matchReason(t, ['name'])).toBeUndefined();
  });

  test('lists the localized fields in the order given', () => {
    expect(matchReason(t, ['organization'])).toBe('Matches: organisation');
    expect(matchReason(t, ['name', 'email', 'phone', 'address'])).toBe(
      'Matches: name, email, phone, address',
    );
  });

  test('ignores a token this client does not know instead of showing a raw key', () => {
    expect(matchReason(t, ['entity'])).toBeUndefined();
    expect(matchReason(t, ['email', 'entity'])).toBe('Matches: email');
  });
});
