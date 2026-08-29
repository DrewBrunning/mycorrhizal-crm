import { cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { useOIDCConfig } from './useOIDCConfig';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
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

test('keeps the defaults when the endpoint is not ok', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(fetchResponse({}, false)));

  const { result } = renderHook(() => useOIDCConfig());
  await new Promise((resolve) => setTimeout(resolve, 0));
  await waitFor(() => expect(result.current.provider_name).toBe('SSO'));

  expect(result.current).toEqual({
    enabled: false,
    provider_name: 'SSO',
    registration_disabled: false,
  });
});

test('keeps the fail-open defaults when the fetch rejects', async () => {
  vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('network down')));

  const { result } = renderHook(() => useOIDCConfig());
  await new Promise((resolve) => setTimeout(resolve, 0));
  await waitFor(() => expect(result.current.provider_name).toBe('SSO'));

  expect(result.current).toEqual({
    enabled: false,
    provider_name: 'SSO',
    registration_disabled: false,
  });
});
