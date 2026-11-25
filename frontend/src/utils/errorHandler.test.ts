import { cleanup } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import { ApiError } from '../api/client';
import i18n from '../i18n/config';
import { getErrorMessage, getErrorMessageOr, handleError, handleFetchError } from './errorHandler';
import { UserFacingError } from './userFacingError';

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

test('leaves API error display messages and user-facing errors alone', () => {
  setOnline(false);
  expect(getErrorMessage(new ApiError('Contact not found', 'NOT_FOUND', 404))).toBe(
    'Contact not found',
  );
  // A UserFacingError carries text its thrower already translated (or a
  // server message meant for display), so it is shown as-is.
  expect(getErrorMessage(new UserFacingError('Bitte erneut versuchen'))).toBe(
    'Bitte erneut versuchen',
  );
});

test('never shows a plain Error raw message to the user (#2920)', () => {
  setOnline(false);
  // A plain Error's message is developer text, usually English; a
  // programming TypeError must not be mislabelled as an offline condition
  // either.
  expect(getErrorMessage(new Error('raw english'))).toBe(i18n.t('common.unexpectedError'));
  expect(getErrorMessage(new TypeError('Cannot read properties of undefined'))).toBe(
    i18n.t('common.unexpectedError'),
  );
  expect(i18n.t('common.unexpectedError')).not.toBe('common.unexpectedError');
});

test('the generic fallback is translated, not hardcoded English (#2920)', async () => {
  const before = i18n.language;
  try {
    await i18n.changeLanguage('de');
    const de = getErrorMessage(new Error('raw english'));
    expect(de).toBe(i18n.t('common.unexpectedError'));
    expect(de).not.toBe('An unexpected error occurred');
    expect(getErrorMessage(undefined)).toBe(de);
  } finally {
    await i18n.changeLanguage(before);
  }
});

test('returns string errors verbatim and a fallback for non-errors', () => {
  expect(getErrorMessage('plain failure')).toBe('plain failure');
  expect(getErrorMessage({ detail: 'not an Error' })).toBe(i18n.t('common.unexpectedError'));
  expect(getErrorMessage(undefined)).toBe(i18n.t('common.unexpectedError'));
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
    new UserFacingError('Save failed'),
    { operation: 'saving note', silent: true },
    { showError },
  );

  expect(message).toBe('Save failed');
  expect(showError).toHaveBeenCalledExactlyOnceWith('Save failed');
  expect(log).not.toHaveBeenCalled();
});

test('handleError without a notifier still logs and returns the message', () => {
  const log = vi.spyOn(console, 'error').mockImplementation(() => {});

  expect(handleError(new Error('Boom'), { operation: 'loading' })).toBe(
    i18n.t('common.unexpectedError'),
  );
  expect(log).toHaveBeenCalledOnce();
  // The log keeps the raw message for debugging; only the UI text is mapped.
  expect(log.mock.calls[0]?.[1]).toMatchObject({
    message: i18n.t('common.unexpectedError'),
    rawMessage: 'Boom',
  });
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

test('getErrorMessageOr maps an Error and falls back for anything else (#2920)', () => {
  setOnline(true);
  expect(getErrorMessageOr(new TypeError('Failed to fetch'), 'Save failed')).toBe(
    i18n.t('common.networkError'),
  );
  expect(getErrorMessageOr(new ApiError('Contact not found', 'NOT_FOUND', 404), 'x')).toBe(
    'Contact not found',
  );
  expect(getErrorMessageOr({ not: 'an error' }, 'Save failed')).toBe('Save failed');
  expect(getErrorMessageOr('oops', 'Save failed')).toBe('Save failed');
  expect(getErrorMessageOr(new UserFacingError('Name taken'), 'Save failed')).toBe('Name taken');
});

test('getErrorMessageOr shows the caller fallback, not a plain Error raw message (#2920)', () => {
  setOnline(true);
  expect(getErrorMessageOr(new Error('raw english'), 'Speichern fehlgeschlagen')).toBe(
    'Speichern fehlgeschlagen',
  );
});

test('an empty UserFacingError falls back instead of showing a blank message (#2920)', () => {
  expect(getErrorMessage(new UserFacingError(''))).toBe(i18n.t('common.unexpectedError'));
  expect(getErrorMessageOr(new UserFacingError(''), 'Save failed')).toBe('Save failed');
});

test('a non-Error value logs no raw message (#2920)', () => {
  const log = vi.spyOn(console, 'error').mockImplementation(() => {});

  expect(handleError({ detail: 'x' }, { operation: 'loading' })).toBe(
    i18n.t('common.unexpectedError'),
  );
  expect(log.mock.calls[0]?.[1]).toMatchObject({ rawMessage: undefined });
});
