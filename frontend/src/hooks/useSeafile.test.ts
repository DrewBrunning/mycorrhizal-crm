import { act, cleanup, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import {
  deleteSeafileConfig,
  getSeafileConfig,
  getSeafileDir,
  getSeafileLibraries,
  type SeafileConfigResponse,
  type SeafileConnectionTestResult,
  type SeafileItem,
  type SeafileLibrary,
  saveSeafileConfig,
  testSeafileConnection,
} from '../api/seafile';
import { useSeafile } from './useSeafile';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock('../api/seafile', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/seafile')>();
  return {
    ...actual,
    getSeafileConfig: vi.fn(),
    saveSeafileConfig: vi.fn(),
    deleteSeafileConfig: vi.fn(),
    testSeafileConnection: vi.fn(),
    getSeafileLibraries: vi.fn(),
    getSeafileDir: vi.fn(),
  };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  for (const fn of [
    getSeafileConfig,
    saveSeafileConfig,
    deleteSeafileConfig,
    testSeafileConnection,
    getSeafileLibraries,
    getSeafileDir,
  ]) {
    vi.mocked(fn).mockReset();
  }
});

const config: SeafileConfigResponse = { base_url: 'https://seafile.example', has_api_token: true };
const library: SeafileLibrary = { id: 'repo-1', name: 'Docs', type: 'repo' };
const item: SeafileItem = { id: 'f-1', name: 'a.pdf', type: 'file' };

test('starts idle with an empty config and no browsing or test state', () => {
  const { result } = renderHook(() => useSeafile());
  expect(result.current.config).toBeNull();
  expect(result.current.configLoading).toBe(false);
  expect(result.current.configError).toBeNull();
  expect(result.current.libraries).toEqual([]);
  expect(result.current.items).toEqual([]);
  expect(result.current.browsing).toBe(false);
  expect(result.current.testing).toBe(false);
  expect(result.current.testResult).toBeNull();
});

test('refreshConfig loads the config and clears loading', async () => {
  vi.mocked(getSeafileConfig).mockResolvedValue(config);

  const { result } = renderHook(() => useSeafile());
  await act(async () => {
    await result.current.refreshConfig();
  });

  expect(result.current.config).toEqual(config);
  expect(result.current.configLoading).toBe(false);
  expect(result.current.configError).toBeNull();
});

test('refreshConfig surfaces a fetch error and clears loading', async () => {
  vi.mocked(getSeafileConfig).mockRejectedValue(new Error('boom'));

  const { result } = renderHook(() => useSeafile());
  await act(async () => {
    await result.current.refreshConfig();
  });

  expect(result.current.configError).toBe('boom');
  expect(result.current.configLoading).toBe(false);
});

test('saveConfig stores the returned config and returns it', async () => {
  vi.mocked(saveSeafileConfig).mockResolvedValue(config);

  const { result } = renderHook(() => useSeafile());
  let updated: SeafileConfigResponse | undefined;
  await act(async () => {
    updated = await result.current.saveConfig({ base_url: config.base_url, api_token: 'tok' });
  });

  expect(updated).toEqual(config);
  expect(result.current.config).toEqual(config);
  expect(saveSeafileConfig).toHaveBeenCalledWith({ base_url: config.base_url, api_token: 'tok' });
});

test('saveConfig notifies and rethrows on failure', async () => {
  vi.mocked(saveSeafileConfig).mockRejectedValue(new Error('save failed'));
  const showError = vi.fn();

  const { result } = renderHook(() => useSeafile({ showError }));
  await expect(result.current.saveConfig({ base_url: 'x' })).rejects.toThrow('save failed');

  expect(showError).toHaveBeenCalledWith('save failed');
});

test('removeConfig deletes the config and clears it', async () => {
  vi.mocked(getSeafileConfig).mockResolvedValue(config);
  vi.mocked(deleteSeafileConfig).mockResolvedValue();

  const { result } = renderHook(() => useSeafile());
  await act(async () => {
    await result.current.refreshConfig();
  });
  expect(result.current.config).toEqual(config);

  await act(async () => {
    await result.current.removeConfig();
  });

  expect(deleteSeafileConfig).toHaveBeenCalled();
  expect(result.current.config).toBeNull();
});

test('browseLibraries loads, stores and returns the libraries', async () => {
  vi.mocked(getSeafileLibraries).mockResolvedValue([library]);

  const { result } = renderHook(() => useSeafile());
  let fetched: SeafileLibrary[] | undefined;
  await act(async () => {
    fetched = await result.current.browseLibraries();
  });

  expect(fetched).toEqual([library]);
  expect(result.current.libraries).toEqual([library]);
  expect(result.current.browsing).toBe(false);
});

test('browseLibraries notifies and rethrows on failure', async () => {
  vi.mocked(getSeafileLibraries).mockRejectedValue(new Error('browse failed'));
  const showError = vi.fn();

  const { result } = renderHook(() => useSeafile({ showError }));
  await expect(result.current.browseLibraries()).rejects.toThrow('browse failed');

  expect(showError).toHaveBeenCalledWith('browse failed');
  expect(result.current.browsing).toBe(false);
});

test('browseDir loads a folder listing and stores the items', async () => {
  vi.mocked(getSeafileDir).mockResolvedValue([item]);

  const { result } = renderHook(() => useSeafile());
  let fetched: SeafileItem[] | undefined;
  await act(async () => {
    fetched = await result.current.browseDir('repo-1', '/docs');
  });

  expect(getSeafileDir).toHaveBeenCalledWith('repo-1', '/docs');
  expect(fetched).toEqual([item]);
  expect(result.current.items).toEqual([item]);
});

test('browseDir notifies and rethrows on failure', async () => {
  vi.mocked(getSeafileDir).mockRejectedValue(new Error('dir failed'));
  const showError = vi.fn();

  const { result } = renderHook(() => useSeafile({ showError }));
  await expect(result.current.browseDir('repo-1', '/')).rejects.toThrow('dir failed');

  expect(showError).toHaveBeenCalledWith('dir failed');
});

test('testConnection stores the result and clears testing', async () => {
  const result_: SeafileConnectionTestResult = { ok: true, stage: 'auth', message: 'fine' };
  vi.mocked(testSeafileConnection).mockResolvedValue(result_);

  const { result } = renderHook(() => useSeafile());
  let fetched: SeafileConnectionTestResult | undefined;
  await act(async () => {
    fetched = await result.current.testConnection();
  });

  expect(fetched).toEqual(result_);
  expect(result.current.testResult).toEqual(result_);
  expect(result.current.testing).toBe(false);
});

test('testConnection resets a stale result and notifies on failure', async () => {
  const showError = vi.fn();
  vi.mocked(testSeafileConnection).mockRejectedValue(new Error('conn failed'));

  const { result } = renderHook(() => useSeafile({ showError }));
  await expect(result.current.testConnection()).rejects.toThrow('conn failed');

  expect(showError).toHaveBeenCalledWith('conn failed');
  expect(result.current.testResult).toBeNull();
  expect(result.current.testing).toBe(false);
});

test('browsing toggles true while a library fetch is in flight', async () => {
  let resolve!: (v: SeafileLibrary[]) => void;
  vi.mocked(getSeafileLibraries).mockReturnValue(new Promise((r) => (resolve = r)));

  const { result } = renderHook(() => useSeafile());
  let pending!: Promise<SeafileLibrary[]>;
  act(() => {
    pending = result.current.browseLibraries();
  });
  expect(result.current.browsing).toBe(true);

  await act(async () => {
    resolve([library]);
    await pending;
  });
  expect(result.current.browsing).toBe(false);
});

test('testing toggles true while a connection test is in flight', async () => {
  let resolve!: (v: SeafileConnectionTestResult) => void;
  vi.mocked(testSeafileConnection).mockReturnValue(new Promise((r) => (resolve = r)));

  const { result } = renderHook(() => useSeafile());
  let pending!: Promise<SeafileConnectionTestResult>;
  act(() => {
    pending = result.current.testConnection();
  });
  expect(result.current.testing).toBe(true);

  await act(async () => {
    resolve({ ok: true, stage: 'auth', message: 'ok' });
    await pending;
  });
  expect(result.current.testing).toBe(false);
});
