import { act, cleanup, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import {
  deleteGeoPulseConfig,
  type GeoPulseSuggestionsResponse,
  getGeoPulseConfig,
  getGeoPulseSuggestions,
  saveGeoPulseConfig,
  testGeoPulseConnection,
} from '../api/geopulse';
import { useGeoPulse } from './useGeoPulse';

vi.mock('../api/geopulse', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/geopulse')>();
  return {
    ...actual,
    getGeoPulseConfig: vi.fn(),
    saveGeoPulseConfig: vi.fn(),
    deleteGeoPulseConfig: vi.fn(),
    testGeoPulseConnection: vi.fn(),
    getGeoPulseSuggestions: vi.fn(),
  };
});

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
});

beforeEach(() => {
  vi.mocked(getGeoPulseConfig).mockReset();
  vi.mocked(saveGeoPulseConfig).mockReset();
  vi.mocked(deleteGeoPulseConfig).mockReset();
  vi.mocked(testGeoPulseConnection).mockReset();
  vi.mocked(getGeoPulseSuggestions).mockReset();
  vi.spyOn(console, 'error').mockImplementation(() => {});
});

const config = { base_url: 'https://gp.example', has_api_key: true };
const suggestions = (date: string): GeoPulseSuggestionsResponse => ({ date, suggestions: [] });

test('refreshConfig loads the config and clears loading', async () => {
  vi.mocked(getGeoPulseConfig).mockResolvedValue(config);
  const { result } = renderHook(() => useGeoPulse());

  await act(async () => {
    await result.current.refreshConfig();
  });

  expect(result.current.config).toEqual(config);
  expect(result.current.configLoading).toBe(false);
  expect(result.current.configError).toBeNull();
});

test('refreshConfig surfaces a failure as configError', async () => {
  vi.mocked(getGeoPulseConfig).mockRejectedValue(new Error('boom'));
  const { result } = renderHook(() => useGeoPulse());

  await act(async () => {
    await result.current.refreshConfig();
  });

  expect(result.current.configError).toBe('boom');
  expect(result.current.config).toBeNull();
  expect(result.current.configLoading).toBe(false);
});

test('saveConfig stores the returned config', async () => {
  vi.mocked(saveGeoPulseConfig).mockResolvedValue(config);
  const { result } = renderHook(() => useGeoPulse());

  let saved: unknown;
  await act(async () => {
    saved = await result.current.saveConfig({ base_url: 'https://gp.example', api_key: 'k' });
  });

  expect(saved).toEqual(config);
  expect(result.current.config).toEqual(config);
  expect(saveGeoPulseConfig).toHaveBeenCalledWith({ base_url: 'https://gp.example', api_key: 'k' });
});

test('saveConfig notifies and rethrows on failure', async () => {
  const notifier = { showError: vi.fn() };
  vi.mocked(saveGeoPulseConfig).mockRejectedValue(new Error('nope'));
  const { result } = renderHook(() => useGeoPulse(notifier));

  await act(async () => {
    await expect(result.current.saveConfig({ base_url: 'x' })).rejects.toThrow('nope');
  });

  expect(notifier.showError).toHaveBeenCalled();
  expect(result.current.config).toBeNull();
});

test('removeConfig clears the config and any stale test result', async () => {
  vi.mocked(getGeoPulseConfig).mockResolvedValue(config);
  vi.mocked(testGeoPulseConnection).mockResolvedValue({ ok: true, stage: 'ok', message: 'hi' });
  vi.mocked(deleteGeoPulseConfig).mockResolvedValue(undefined);
  const { result } = renderHook(() => useGeoPulse());

  await act(async () => {
    await result.current.refreshConfig();
    await result.current.testConnection();
  });
  expect(result.current.testResult).not.toBeNull();

  await act(async () => {
    await result.current.removeConfig();
  });

  expect(deleteGeoPulseConfig).toHaveBeenCalled();
  expect(result.current.config).toBeNull();
  expect(result.current.testResult).toBeNull();
});

test('testConnection stores the diagnosis and toggles testing', async () => {
  vi.mocked(testGeoPulseConnection).mockResolvedValue({
    ok: false,
    stage: 'auth',
    message: 'rejected',
  });
  const { result } = renderHook(() => useGeoPulse());

  await act(async () => {
    await result.current.testConnection();
  });

  expect(result.current.testResult).toEqual({ ok: false, stage: 'auth', message: 'rejected' });
  expect(result.current.testing).toBe(false);
});

test('testConnection notifies and rethrows when the check cannot run', async () => {
  const notifier = { showError: vi.fn() };
  vi.mocked(testGeoPulseConnection).mockRejectedValue(new Error('not configured'));
  const { result } = renderHook(() => useGeoPulse(notifier));

  await act(async () => {
    await expect(result.current.testConnection()).rejects.toThrow('not configured');
  });

  expect(notifier.showError).toHaveBeenCalled();
  expect(result.current.testing).toBe(false);
  expect(result.current.testResult).toBeNull();
});

test('lookup stores the suggestions', async () => {
  vi.mocked(getGeoPulseSuggestions).mockResolvedValue(suggestions('2026-09-20'));
  const { result } = renderHook(() => useGeoPulse());

  await act(async () => {
    await result.current.lookup('2026-09-20', 'UTC');
  });

  expect(getGeoPulseSuggestions).toHaveBeenCalledWith('2026-09-20', 'UTC');
  expect(result.current.suggestions).toEqual(suggestions('2026-09-20'));
  expect(result.current.suggestionsLoading).toBe(false);
  expect(result.current.suggestionsError).toBeNull();
});

test('lookup surfaces a failure inline (not toasted) and returns null', async () => {
  vi.mocked(getGeoPulseSuggestions).mockRejectedValue(new Error('GeoPulse is not configured'));
  const notifier = { showError: vi.fn() };
  const { result } = renderHook(() => useGeoPulse(notifier));

  let returned: unknown = 'unset';
  await act(async () => {
    returned = await result.current.lookup('2026-09-20');
  });

  expect(returned).toBeNull();
  expect(result.current.suggestionsError).toBe('GeoPulse is not configured');
  expect(result.current.suggestions).toBeNull();
  expect(notifier.showError).not.toHaveBeenCalled();
});

test('a slower, older lookup cannot overwrite a newer one', async () => {
  let resolveFirst: (v: GeoPulseSuggestionsResponse) => void = () => {};
  vi.mocked(getGeoPulseSuggestions)
    .mockImplementationOnce(() => new Promise((resolve) => (resolveFirst = resolve)))
    .mockResolvedValueOnce(suggestions('2026-09-21'));
  const { result } = renderHook(() => useGeoPulse());

  let first: Promise<unknown> = Promise.resolve();
  act(() => {
    first = result.current.lookup('2026-09-20');
  });
  await act(async () => {
    await result.current.lookup('2026-09-21');
  });
  expect(result.current.suggestions?.date).toBe('2026-09-21');

  await act(async () => {
    resolveFirst(suggestions('2026-09-20'));
    await first;
  });

  expect(result.current.suggestions?.date).toBe('2026-09-21');
  expect(result.current.suggestionsLoading).toBe(false);
});

test('a slower, older lookup failure cannot overwrite a newer result either', async () => {
  let rejectFirst: (e: Error) => void = () => {};
  vi.mocked(getGeoPulseSuggestions)
    .mockImplementationOnce(() => new Promise((_, reject) => (rejectFirst = reject)))
    .mockResolvedValueOnce(suggestions('2026-09-21'));
  const { result } = renderHook(() => useGeoPulse());

  let first: Promise<unknown> = Promise.resolve();
  act(() => {
    first = result.current.lookup('2026-09-20');
  });
  await act(async () => {
    await result.current.lookup('2026-09-21');
  });
  await act(async () => {
    rejectFirst(new Error('stale failure'));
    await first;
  });

  expect(result.current.suggestionsError).toBeNull();
  expect(result.current.suggestions?.date).toBe('2026-09-21');
});

test('clearSuggestions drops the list, any error, and an in-flight lookup', async () => {
  let resolveLookup: (v: GeoPulseSuggestionsResponse) => void = () => {};
  vi.mocked(getGeoPulseSuggestions).mockImplementationOnce(
    () => new Promise((resolve) => (resolveLookup = resolve)),
  );
  const { result } = renderHook(() => useGeoPulse());

  let pending: Promise<unknown> = Promise.resolve();
  act(() => {
    pending = result.current.lookup('2026-09-20');
  });
  expect(result.current.suggestionsLoading).toBe(true);

  act(() => {
    result.current.clearSuggestions();
  });
  expect(result.current.suggestionsLoading).toBe(false);

  await act(async () => {
    resolveLookup(suggestions('2026-09-20'));
    await pending;
  });
  expect(
    result.current.suggestions,
    'a lookup that finishes after clear must not repopulate',
  ).toBeNull();
});
