import { cleanup } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import { ApiError } from '../api/client';
import i18n from '../i18n/config';
import { getErrorMessage, handleError, handleFetchError } from './errorHandler';

const originalOnLine = Object.getOwnPropertyDescriptor(window.navigator, 'onLine');

function setOnline(value: boolean) {
  Object.defineProperty(window.navigator, 'onLine', { configurable: true, value });
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
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

test('returns string errors verbatim and a fallback for non-errors', () => {
  expect(getErrorMessage('plain failure')).toBe('plain failure');
  expect(getErrorMessage({ detail: 'not an Error' })).toBe('An unexpected error occurred');
  expect(getErrorMessage(undefined)).toBe('An unexpected error occurred');
});

test('handleError logs the operation, code and request id, then notifies', () => {
  const log = vi.spyOn(console, 'error').mockImplementation(() => {});
  const showError = vi.fn();
  const error = new ApiError('Contact not found', 'NOT_FOUND', 404, undefined, 'req-42');

  const message = handleError(error, { operation: 'loading contact' }, { showError });

  expect(message).toBe('Contact not found');
  expect(showError).toHaveBeenCalledExactlyOnceWith('Contact not found');
  expect(log).toHaveBeenCalledOnce();
  expect(log.mock.calls[0]?.[0]).toBe('[loading contact] Error:');
  expect(log.mock.calls[0]?.[1]).toMatchObject({
    message: 'Contact not found',
    code: 'NOT_FOUND',
    requestId: 'req-42',
    error,
  });
});

test('handleError still logs a SESSION_EXPIRED 401 but never raises a second toast', () => {
  const log = vi.spyOn(console, 'error').mockImplementation(() => {});
  const showError = vi.fn();
  const error = new ApiError('Session expired', 'SESSION_EXPIRED', 401);

  const message = handleError(error, { operation: 'polling dashboard' }, { showError });

  expect(message).toBe('Session expired');
  expect(showError).not.toHaveBeenCalled();
  expect(log).toHaveBeenCalledOnce();
  expect(log.mock.calls[0]?.[1]).toMatchObject({ code: 'SESSION_EXPIRED' });
});

test('handleError with silent skips logging but still notifies', () => {
  const log = vi.spyOn(console, 'error').mockImplementation(() => {});
  const showError = vi.fn();

  const message = handleError(
    new Error('Save failed'),
    { operation: 'saving note', silent: true },
    { showError },
  );

  expect(message).toBe('Save failed');
  expect(showError).toHaveBeenCalledExactlyOnceWith('Save failed');
  expect(log).not.toHaveBeenCalled();
});

test('handleError without a notifier still logs and returns the message', () => {
  const log = vi.spyOn(console, 'error').mockImplementation(() => {});

  expect(handleError(new Error('Boom'), { operation: 'loading' })).toBe('Boom');
  expect(log).toHaveBeenCalledOnce();
});

test('handleFetchError logs and returns the user-facing network message', () => {
  const log = vi.spyOn(console, 'error').mockImplementation(() => {});
  setOnline(true);

  const message = handleFetchError(new TypeError('Failed to fetch'), 'fetching contacts');

  expect(message).toBe(i18n.t('common.networkError'));
  expect(log).toHaveBeenCalledOnce();
  expect(log.mock.calls[0]?.[0]).toBe('[fetching contacts] Error:');
  expect(log.mock.calls[0]?.[1]).toMatchObject({ message: i18n.t('common.networkError') });
});
