import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { useOIDCConfig } from './useOIDCConfig';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  // Restore the console.error no-op so the framework-warning gate in
  // setupTests.ts is active for every later test.
  vi.restoreAllMocks();
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
});

function fetchResponse(body: unknown, ok = true): Response {
  return { ok, json: async () => body } as unknown as Response;
}

test('keeps the fail-open defaults until the discovery fetch resolves', () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(fetchResponse({})));

  const { result } = renderHook(() => useOIDCConfig());

  expect(result.current).toEqual({
    enabled: false,
    provider_name: 'SSO',
    registration_disabled: false,
  });
});

test('loads the discovery config when the fetch succeeds', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(
      fetchResponse({
        enabled: true,
        provider_name: 'Authentik',
        registration_disabled: true,
      }),
    ),
  );

  const { result } = renderHook(() => useOIDCConfig());
  await waitFor(() => expect(result.current.provider_name).toBe('Authentik'));

  expect(result.current).toEqual({
    enabled: true,
    provider_name: 'Authentik',
    registration_disabled: true,
  });
});

test('coerces non-boolean truthy flags to false and defaults the provider name', async () => {
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockResolvedValue(
        fetchResponse({ enabled: 1, provider_name: '', registration_disabled: 'yes' }),
      ),
  );

  const { result } = renderHook(() => useOIDCConfig());
  await waitFor(() => expect(result.current.provider_name).toBe('SSO'));

  expect(result.current.enabled).toBe(false);
  expect(result.current.registration_disabled).toBe(false);
});

const FAIL_OPEN_DEFAULTS = {
  enabled: false,
  provider_name: 'SSO',
  registration_disabled: false,
};

test('keeps the defaults when the endpoint is not ok', async () => {
  // A would-be-success payload on a non-ok response: if the res.ok guard is
  // removed, this flips the hook away from its fail-open defaults and fails.
  const fetchMock = vi
    .fn()
    .mockResolvedValue(
      fetchResponse(
        { enabled: true, provider_name: 'Should not load', registration_disabled: true },
        false,
      ),
    );
  vi.stubGlobal('fetch', fetchMock);

  const { result } = renderHook(() => useOIDCConfig());
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
  // Flush the fetch handler chain rather than waiting on an initial-state
  // predicate that was already true before the request settled.
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });

  expect(result.current).toEqual(FAIL_OPEN_DEFAULTS);
  expect(result.current.provider_name).not.toBe('Should not load');
});

test('keeps the fail-open defaults when the fetch rejects', async () => {
  const unhandled: unknown[] = [];
  const onUnhandled = (reason: unknown) => unhandled.push(reason);
  process.on('unhandledRejection', onUnhandled);
  try {
    const fetchMock = vi.fn().mockRejectedValue(new Error('network down'));
    vi.stubGlobal('fetch', fetchMock);

    const { result } = renderHook(() => useOIDCConfig());
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    // Flush the rejection handler. If its .catch were removed this becomes an
    // unhandled rejection, which the assertion below turns into a failure.
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    expect(unhandled).toEqual([]);
    expect(result.current).toEqual(FAIL_OPEN_DEFAULTS);
  } finally {
    process.off('unhandledRejection', onUnhandled);
  }
});

// --- behavior pins (issue #1662) ---

test('requests the OIDC discovery endpoint', async () => {
  const fetchMock = vi.fn().mockResolvedValue(fetchResponse({}));
  vi.stubGlobal('fetch', fetchMock);
  renderHook(() => useOIDCConfig());
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
  expect(String(fetchMock.mock.calls[0][0])).toMatch(/\/auth\/oidc\/config$/);
});

test('a non-ok response keeps the defaults; a missing provider name falls back to SSO', async () => {
  const fetchMock = vi
    .fn()
    .mockResolvedValueOnce(fetchResponse({ enabled: true, provider_name: 'X' }, false))
    .mockResolvedValueOnce(fetchResponse({ enabled: true }));
  vi.stubGlobal('fetch', fetchMock);
  const a = renderHook(() => useOIDCConfig());
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
  await new Promise((r) => setTimeout(r, 0));
  expect(a.result.current.enabled).toBe(false);
  cleanup();

  const b = renderHook(() => useOIDCConfig());
  await waitFor(() => expect(b.result.current.enabled).toBe(true));
  expect(b.result.current.provider_name).toBe('SSO');
  expect(b.result.current.registration_disabled).toBe(false);
});
