import i18next from 'i18next';
import { afterEach, describe, expect, test, vi } from 'vitest';
import '../i18n/config';
import { ApiError, apiFetch, parseErrorResponse } from './client';
import { onSessionExpired } from './sessionExpiry';

describe('ApiError.getDisplayMessage', () => {
  test('returns the message when there are no details', () => {
    const err = new ApiError('Something went wrong', 'INTERNAL', 500);
    expect(err.getDisplayMessage()).toBe('Something went wrong');
  });

  // The code here must be the one the backend actually emits
  // (apperrors.ErrCodeValidation = "VALIDATION_ERROR"). This fixture used a
  // bare "VALIDATION", which no server response ever carries — it only passed
  // because getDisplayMessage folded in `details` for every code. It no longer
  // does: `details` holds field messages for VALIDATION_ERROR/INVALID_INPUT
  // and machine context for everything else, and conflating the two showed
  // users a bare record id instead of "Contact not found".
  test('joins field-level details when present', () => {
    const err = new ApiError('Validation failed', 'VALIDATION_ERROR', 400, {
      email: 'Email is invalid',
      password: 'Password too weak',
    });
    expect(err.getDisplayMessage()).toBe('Email is invalid. Password too weak');
  });

  // apperrors.ErrInvalidInput(field, reason) sends {field, reason}, not a
  // field -> message map; joining every value leaked the raw field name.
  test('INVALID_INPUT shows only the reason', () => {
    const err = new ApiError('Invalid input', 'INVALID_INPUT', 400, {
      field: 'credentials',
      reason: 'Meerkat rejected the credentials',
    });
    expect(err.getDisplayMessage()).toBe('Meerkat rejected the credentials');
  });

  test('INVALID_INPUT without a reason shows the message', () => {
    const err = new ApiError("Invalid value for field 'base_url'", 'INVALID_INPUT', 400, {
      field: 'base_url',
    });
    expect(err.getDisplayMessage()).toBe("Invalid value for field 'base_url'");
  });

  test('INVALID_INPUT with an empty reason shows the message', () => {
    const err = new ApiError('Invalid input', 'INVALID_INPUT', 400, { field: 'x', reason: '' });
    expect(err.getDisplayMessage()).toBe('Invalid input');
  });

  test('INVALID_INPUT ignores the raw-input detail key', () => {
    const err = new ApiError('Invalid input', 'INVALID_INPUT', 400, {
      field: 'id',
      reason: 'must be a positive integer',
      id: 'abc',
    });
    expect(err.getDisplayMessage()).toBe('must be a positive integer');
  });

  test('returns the message for an empty details object', () => {
    const err = new ApiError('Validation failed', 'VALIDATION_ERROR', 400, {});
    expect(err.getDisplayMessage()).toBe('Validation failed');
  });
});

describe('parseErrorResponse', () => {
  test('parses a structured backend error', async () => {
    const response = new Response(
      JSON.stringify({
        error: { code: 'NOT_FOUND', message: 'Contact not found', details: { id: '42' } },
        request_id: 'req-123',
      }),
      { status: 404 },
    );

    const err = await parseErrorResponse(response);
    expect(err).toBeInstanceOf(ApiError);
    expect(err.message).toBe('Contact not found');
    expect(err.code).toBe('NOT_FOUND');
    expect(err.status).toBe(404);
    expect(err.details).toEqual({ id: '42' });
    expect(err.requestId).toBe('req-123');
  });

  test('falls back to status text for a non-JSON body', async () => {
    const response = new Response('<html>Bad Gateway</html>', {
      status: 502,
      statusText: 'Bad Gateway',
    });

    const err = await parseErrorResponse(response);
    expect(err.message).toBe('Bad Gateway');
    expect(err.code).toBe('UNKNOWN_ERROR');
    expect(err.status).toBe(502);
  });

  test('falls back to a generic message when status text is empty', async () => {
    const response = new Response('not json', { status: 500 });

    const err = await parseErrorResponse(response);
    expect(err.message).toBe('An error occurred');
    expect(err.status).toBe(500);
  });
});

// Issue #557: apiFetch used to react to a 401 with
// `window.location.href = '/login'` from inside this shared wrapper --
// unconditional, on any request. That tore the whole React tree down (and
// every unsaved field in it) with no prompt. These pin the replacement: no
// navigation, no localStorage mutation, just a thrown, distinguishable error
// plus a sessionExpiry notification the caller's own dialog survives to
// catch.
describe('apiFetch 401 handling', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  test('does not navigate or touch localStorage on a 401', async () => {
    localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(null, { status: 401 })));
    const originalHref = window.location.href;

    await expect(apiFetch('/api/v1/contacts')).rejects.toThrow(ApiError);

    expect(window.location.href).toBe(originalHref);
    expect(localStorage.getItem('user_info')).not.toBeNull();
  });

  test('throws a distinguishable SESSION_EXPIRED ApiError', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(null, { status: 401 })));

    try {
      await apiFetch('/api/v1/contacts');
      expect.unreachable('apiFetch should have thrown');
    } catch (err) {
      expect(err).toBeInstanceOf(ApiError);
      expect((err as ApiError).code).toBe('SESSION_EXPIRED');
      expect((err as ApiError).status).toBe(401);
    }
  });

  test('a GET 401 (a background poll) notifies passive', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(null, { status: 401 })));
    const events: string[] = [];
    const unsubscribe = onSessionExpired(({ mode }) => events.push(mode));

    await expect(apiFetch('/api/v1/dashboard')).rejects.toThrow(ApiError);

    expect(events).toEqual(['passive']);
    unsubscribe();
  });

  test('a mutating 401 (an explicit save) notifies blocking', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(null, { status: 401 })));
    const events: string[] = [];
    const unsubscribe = onSessionExpired(({ mode }) => events.push(mode));

    await expect(apiFetch('/api/v1/notes', { method: 'POST', body: '{}' })).rejects.toThrow(
      ApiError,
    );

    expect(events).toEqual(['blocking']);
    unsubscribe();
  });

  test('a non-401 response is returned normally, untouched', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('{"ok":true}', { status: 200 })));

    const response = await apiFetch('/api/v1/contacts');
    expect(response.status).toBe(200);
  });
});

describe('parseErrorResponse / apiFetch fallbacks', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  test('a JSON body without an error envelope falls back to status text', async () => {
    const response = {
      status: 502,
      statusText: 'Bad Gateway',
      json: async () => ({ unexpected: true }),
    } as unknown as Response;
    const err = await parseErrorResponse(response);
    expect(err).toMatchObject({ message: 'Bad Gateway', code: 'UNKNOWN_ERROR', status: 502 });
  });

  test('an aborted request is reported as a timeout, not a raw AbortError', async () => {
    const abort = Object.assign(new Error('aborted'), { name: 'AbortError' });
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(abort));
    await expect(apiFetch('/api/v1/anything')).rejects.toThrow(/Request timeout after \d+ seconds/);
  });
});

// ADR 0038 / issue #2096: a server reason code is translated when this build
// knows it and every param is present; otherwise the server message stands.
describe('ApiError.getDisplayMessage reason codes', () => {
  afterEach(async () => {
    vi.restoreAllMocks();
    await i18next.changeLanguage('en');
  });

  test('renders the translation for a known code with complete params', async () => {
    await i18next.changeLanguage('de');
    const err = new ApiError(
      'must be at most 10 characters',
      'INVALID_INPUT',
      400,
      undefined,
      undefined,
      'contact_bulk_value_too_long',
      { field: 'organization', limit: 10 },
    );
    expect(err.getDisplayMessage()).toBe(
      i18next.t('errors.reason.contact_bulk_value_too_long', { field: 'organization', limit: 10 }),
    );
    expect(err.getDisplayMessage()).not.toBe('must be at most 10 characters');
  });

  test('falls back to the server message for an unknown code', () => {
    const err = new ApiError(
      'Server says no',
      'INVALID_INPUT',
      400,
      undefined,
      undefined,
      'not_a_real_code',
      {},
    );
    expect(err.getDisplayMessage()).toBe('Server says no');
  });

  test('falls back to the server message when a param is missing', () => {
    const err = new ApiError(
      'Server says no',
      'INVALID_INPUT',
      400,
      undefined,
      undefined,
      'contact_bulk_value_too_long',
      { field: 'organization' },
    );
    expect(err.getDisplayMessage()).toBe('Server says no');
  });

  test('appends the localized request reference on a 5xx', async () => {
    await i18next.changeLanguage('de');
    const err = new ApiError('Something broke', 'INTERNAL', 503, undefined, 'abc');
    expect(err.getDisplayMessage()).toBe(
      `Something broke ${i18next.t('errors.requestRef', { id: 'abc' })}`,
    );
  });

  test('keeps the English request reference when i18n is not ready', () => {
    vi.spyOn(i18next, 'exists').mockReturnValue(false);
    const err = new ApiError('Something broke', 'INTERNAL', 503, undefined, 'abc');
    expect(err.getDisplayMessage()).toBe('Something broke (ref: abc)');
  });

  test('parseErrorResponse carries reason_code and reason_params from the envelope', async () => {
    const response = new Response(
      JSON.stringify({
        error: {
          code: 'INVALID_INPUT',
          message: 'Invalid value',
          reason_code: 'relationship_date_month_range',
          reason_params: { field: 'start_date' },
        },
        request_id: 'req-1',
      }),
      { status: 400 },
    );
    const err = await parseErrorResponse(response);
    expect(err.reasonCode).toBe('relationship_date_month_range');
    expect(err.reasonParams).toEqual({ field: 'start_date' });
    expect(err.requestId).toBe('req-1');
  });

  test('parseErrorResponse leaves the reason fields unset for a plain envelope', async () => {
    const response = new Response(
      JSON.stringify({ error: { code: 'NOT_FOUND', message: 'Missing' } }),
      {
        status: 404,
      },
    );
    const err = await parseErrorResponse(response);
    expect(err.reasonCode).toBeUndefined();
    expect(err.reasonParams).toBeUndefined();
  });
});
