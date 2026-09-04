import { afterEach, describe, expect, test, vi } from 'vitest';
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
