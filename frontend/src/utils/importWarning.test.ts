import { expect, test } from 'vitest';
import { importWarningKey } from './importWarning';

test('known concepts map to their key, anything else to undefined', () => {
  expect(importWarningKey('birthday')).toBe('contacts.import.warning.birthday');
  expect(importWarningKey('anniversary')).toBe('contacts.import.warning.anniversary');
  expect(importWarningKey('nickname')).toBeUndefined();
  expect(importWarningKey('')).toBeUndefined();
  expect(importWarningKey(undefined)).toBeUndefined();
});
