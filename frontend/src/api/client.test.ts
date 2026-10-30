import { afterEach, describe, expect, test, vi } from 'vitest';
import i18n from '../i18n/config';
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

  // Issue #2105: field_reasons is the translatable companion to details.
  test('joins translated field_reasons for a validation error', () => {
    const err = new ApiError(
      'Request validation failed',
      'VALIDATION_ERROR',
      400,
      { Name: 'Name is required', Email: 'Email must be a valid email address' },
      undefined,
      undefined,
      undefined,
      {
        Name: { reason_code: 'validation_required', reason_params: { field: 'Name' } },
        Email: {
          reason_code: 'validation_email',
          reason_params: { field: 'Email' },
        },
      },
    );
    expect(err.getDisplayMessage()).toBe('Name is required. Email must be a valid email address');
  });

  test('falls back to details when field_reasons is empty', () => {
    const err = new ApiError(
      'Request validation failed',
      'VALIDATION_ERROR',
      400,
      { Name: 'Name is required' },
      undefined,
      undefined,
      undefined,
      {},
    );
    expect(err.getDisplayMessage()).toBe('Name is required');
  });

  test('falls back to details when one field reason is unknown', () => {
    const err = new ApiError(
      'Request validation failed',
      'VALIDATION_ERROR',
      400,
      { Name: 'Name is required', Mystery: 'Mystery is invalid' },
      undefined,
      undefined,
      undefined,
      {
        Name: { reason_code: 'validation_required', reason_params: { field: 'Name' } },
        Mystery: { reason_code: 'not_a_real_code', reason_params: {} },
      },
    );
    expect(err.getDisplayMessage()).toBe('Name is required. Mystery is invalid');
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

  test('parses field_reasons on a validation error', async () => {
    const response = new Response(
      JSON.stringify({
        error: {
          code: 'VALIDATION_ERROR',
          message: 'Request validation failed',
          details: { Name: 'Name is required' },
          field_reasons: {
            Name: { reason_code: 'validation_required', reason_params: { field: 'Name' } },
          },
        },
      }),
      { status: 400 },
    );

    const err = await parseErrorResponse(response);
    expect(err.fieldReasons).toEqual({
      Name: { reason_code: 'validation_required', reason_params: { field: 'Name' } },
    });
    expect(err.getDisplayMessage()).toBe('Name is required');
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

// Issue #2096 / ADR 0038: the server sends reason_code + reason_params so the
// client can translate; an unknown code or a missing param falls back to the
// server's English message.
describe('ApiError reason_code localization', () => {
  afterEach(async () => {
    await i18n.changeLanguage('en');
  });

  test('known code with complete params is translated', async () => {
    const err = new ApiError(
      'Test 3',
      'INVALID_INPUT',
      400,
      undefined,
      undefined,
      'common_test_only',
      {
        n: 3,
      },
    );
    await i18n.changeLanguage('en');
    expect(err.getDisplayMessage()).toBe('Test 3');
    await i18n.changeLanguage('de');
    expect(err.getDisplayMessage()).toBe('Prüfung 3');
  });

  test('unknown code shows the server message', () => {
    const err = new ApiError(
      'Server English',
      'INVALID_INPUT',
      400,
      undefined,
      undefined,
      'nope_nope',
      {
        n: 3,
      },
    );
    expect(err.getDisplayMessage()).toBe('Server English');
  });

  test('known code with a missing param shows the server message, not a raw template', async () => {
    await i18n.changeLanguage('de');
    const missing = new ApiError(
      'Server English',
      'INVALID_INPUT',
      400,
      undefined,
      undefined,
      'common_test_only',
      {},
    );
    expect(missing.getDisplayMessage()).toBe('Server English');
    const none = new ApiError(
      'Server English',
      'INVALID_INPUT',
      400,
      undefined,
      undefined,
      'common_test_only',
    );
    expect(none.getDisplayMessage()).toBe('Server English');
  });

  test('5xx appends the translated request ref', async () => {
    const err = new ApiError('Boom', 'INTERNAL_ERROR', 503, undefined, 'abc');
    expect(err.getDisplayMessage()).toBe('Boom (ref: abc)');
    await i18n.changeLanguage('de');
    expect(err.getDisplayMessage()).toBe('Boom (Ref.: abc)');
  });

  test('a reason on a 5xx still gets the ref suffix', () => {
    const err = new ApiError(
      'Test 3',
      'INTERNAL_ERROR',
      500,
      undefined,
      'abc',
      'common_test_only',
      {
        n: 3,
      },
    );
    expect(err.getDisplayMessage()).toBe('Test 3 (ref: abc)');
  });

  test('4xx never shows the ref', () => {
    const err = new ApiError('Bad', 'INVALID_INPUT', 400, undefined, 'abc');
    expect(err.getDisplayMessage()).toBe('Bad');
  });

  test('parseErrorResponse populates reasonCode/reasonParams', async () => {
    const response = new Response(
      JSON.stringify({
        error: {
          code: 'INVALID_INPUT',
          message: 'Test 3',
          reason_code: 'common_test_only',
          reason_params: { n: 3 },
        },
      }),
      { status: 400 },
    );
    const err = await parseErrorResponse(response);
    expect(err.reasonCode).toBe('common_test_only');
    expect(err.reasonParams).toEqual({ n: 3 });
  });

  test('parseErrorResponse leaves them undefined when absent', async () => {
    const response = new Response(JSON.stringify({ error: { code: 'X', message: 'm' } }), {
      status: 400,
    });
    const err = await parseErrorResponse(response);
    expect(err.reasonCode).toBeUndefined();
    expect(err.reasonParams).toBeUndefined();
  });
});
