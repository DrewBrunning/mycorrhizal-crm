import { cleanup } from '@testing-library/react';
import { afterEach, expect, test } from 'vitest';
import { ApiError } from '../api/client';
import i18n from '../i18n/config';
import { getErrorMessage } from './errorHandler';

const originalOnLine = Object.getOwnPropertyDescriptor(window.navigator, 'onLine');

function setOnline(value: boolean) {
  Object.defineProperty(window.navigator, 'onLine', { configurable: true, value });
}

afterEach(() => {
  cleanup();
  if (originalOnLine) {
    Object.defineProperty(window.navigator, 'onLine', originalOnLine);
  } else {
    delete (window.navigator as unknown as { onLine?: boolean }).onLine;
  }
});

test('names the offline condition instead of showing the raw fetch failure', () => {
  setOnline(false);
  expect(getErrorMessage(new TypeError('Failed to fetch'))).toBe(i18n.t('common.offlineMessage'));
});

test('distinguishes an unreachable server from a browser that is offline', () => {
  setOnline(true);
  expect(getErrorMessage(new TypeError('Failed to fetch'))).toBe(i18n.t('common.networkError'));
  expect(getErrorMessage(new Error('NetworkError when attempting to fetch resource.'))).toBe(
    i18n.t('common.networkError'),
  );
});

test('leaves non-network errors and API error display messages alone', () => {
  setOnline(false);
  expect(getErrorMessage(new Error('Service unavailable'))).toBe('Service unavailable');
  // A programming TypeError must not be mislabelled as an offline condition.
  expect(getErrorMessage(new TypeError('Cannot read properties of undefined'))).toBe(
    'Cannot read properties of undefined',
  );
  expect(getErrorMessage(new ApiError('Contact not found', 'NOT_FOUND', 404))).toBe(
    'Contact not found',
  );
});
