import { act, cleanup, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import {
  deleteImmichConfig,
  getImmichConfig,
  getImmichContactSummary,
  getImmichPeople,
  type ImmichConfigResponse,
  type ImmichConnectionTestResult,
  type ImmichPerson,
  type ImmichPersonSummary,
  linkImmichPerson,
  saveImmichConfig,
  syncImmich,
  testImmichConnection,
  unlinkImmichPerson,
} from '../api/immich';
import { useImmich } from './useImmich';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock('../api/immich', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/immich')>();
  return {
    ...actual,
    getImmichConfig: vi.fn(),
    saveImmichConfig: vi.fn(),
    deleteImmichConfig: vi.fn(),
    testImmichConnection: vi.fn(),
    getImmichPeople: vi.fn(),
    getImmichContactSummary: vi.fn(),
    linkImmichPerson: vi.fn(),
    unlinkImmichPerson: vi.fn(),
    syncImmich: vi.fn(),
  };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  for (const fn of [
    getImmichConfig,
    saveImmichConfig,
    deleteImmichConfig,
    testImmichConnection,
    getImmichPeople,
    getImmichContactSummary,
    linkImmichPerson,
    unlinkImmichPerson,
    syncImmich,
  ]) {
    vi.mocked(fn).mockReset();
  }
});

const config: ImmichConfigResponse = {
  base_url: 'https://immich.example',
  has_api_key: true,
  sync_enabled: true,
  last_sync_status: 'success',
  last_sync_error: '',
};

const person: ImmichPerson = { id: 'p-1', name: 'Ada' };

const summary: ImmichPersonSummary = {
  identity: {
    id: 'i-1',
    entity_id: 'uid-1',
    system: 'immich',
    external_id: 'p-1',
    sync_status: 'ok',
  },
  person_name: 'Ada',
  photo_count: 3,
};

test('starts idle with an empty config, summary and people', () => {
  const { result } = renderHook(() => useImmich());
  expect(result.current.config).toBeNull();
  expect(result.current.configLoading).toBe(false);
  expect(result.current.configError).toBeNull();
  expect(result.current.summary).toBeNull();
  expect(result.current.summaryLoading).toBe(false);
  expect(result.current.people).toEqual([]);
  expect(result.current.peopleLoading).toBe(false);
  expect(result.current.syncing).toBe(false);
  expect(result.current.testing).toBe(false);
  expect(result.current.testResult).toBeNull();
});

test('refreshConfig loads the config and clears loading', async () => {
  vi.mocked(getImmichConfig).mockResolvedValue(config);

  const { result } = renderHook(() => useImmich());
  await act(async () => {
    await result.current.refreshConfig();
  });

  expect(result.current.config).toEqual(config);
  expect(result.current.configLoading).toBe(false);
  expect(result.current.configError).toBeNull();
});

test('refreshConfig surfaces a fetch error and clears loading', async () => {
  vi.mocked(getImmichConfig).mockRejectedValue(new Error('boom'));

  const { result } = renderHook(() => useImmich());
  await act(async () => {
    await result.current.refreshConfig();
  });

  expect(result.current.configError).toBe('boom');
  expect(result.current.configLoading).toBe(false);
});

test('saveConfig stores the returned config and returns it', async () => {
  vi.mocked(saveImmichConfig).mockResolvedValue(config);

  const { result } = renderHook(() => useImmich());
  let updated: ImmichConfigResponse | undefined;
  await act(async () => {
    updated = await result.current.saveConfig({ base_url: config.base_url, api_key: 'k' });
  });

  expect(updated).toEqual(config);
  expect(result.current.config).toEqual(config);
  expect(saveImmichConfig).toHaveBeenCalledWith({ base_url: config.base_url, api_key: 'k' });
});

test('saveConfig notifies and rethrows on failure', async () => {
  vi.mocked(saveImmichConfig).mockRejectedValue(new Error('save failed'));
  const showError = vi.fn();

  const { result } = renderHook(() => useImmich({ showError }));
  await expect(result.current.saveConfig({ base_url: 'x' })).rejects.toThrow('save failed');

  expect(showError).toHaveBeenCalledWith('save failed');
});

test('removeConfig deletes the config and clears it', async () => {
  vi.mocked(getImmichConfig).mockResolvedValue(config);
  vi.mocked(deleteImmichConfig).mockResolvedValue();

  const { result } = renderHook(() => useImmich());
  await act(async () => {
    await result.current.refreshConfig();
  });
  expect(result.current.config).toEqual(config);

  await act(async () => {
    await result.current.removeConfig();
  });

  expect(deleteImmichConfig).toHaveBeenCalled();
  expect(result.current.config).toBeNull();
});

test('browsePeople loads, stores and returns the people', async () => {
  vi.mocked(getImmichPeople).mockResolvedValue([person]);

  const { result } = renderHook(() => useImmich());
  let fetched: ImmichPerson[] | undefined;
  await act(async () => {
    fetched = await result.current.browsePeople();
  });

  expect(fetched).toEqual([person]);
  expect(result.current.people).toEqual([person]);
  expect(result.current.peopleLoading).toBe(false);
});

test('browsePeople notifies and rethrows on failure', async () => {
  vi.mocked(getImmichPeople).mockRejectedValue(new Error('people failed'));
  const showError = vi.fn();

  const { result } = renderHook(() => useImmich({ showError }));
  await expect(result.current.browsePeople()).rejects.toThrow('people failed');

  expect(showError).toHaveBeenCalledWith('people failed');
});

test('loadSummary stores and returns a person summary', async () => {
  vi.mocked(getImmichContactSummary).mockResolvedValue(summary);

  const { result } = renderHook(() => useImmich());
  let fetched: ImmichPersonSummary | null = null;
  await act(async () => {
    fetched = await result.current.loadSummary('uid-1');
  });

  expect(fetched).toEqual(summary);
  expect(result.current.summary).toEqual(summary);
  expect(result.current.summaryLoading).toBe(false);
});

test('loadSummary returns null on failure without throwing', async () => {
  vi.mocked(getImmichContactSummary).mockRejectedValue(new Error('nope'));

  const { result } = renderHook(() => useImmich());
  let fetched: ImmichPersonSummary | null = summary;
  await act(async () => {
    fetched = await result.current.loadSummary('uid-1');
  });

  expect(fetched).toBeNull();
  expect(result.current.summaryLoading).toBe(false);
});

test('linkPerson calls the link API with the id and name', async () => {
  vi.mocked(linkImmichPerson).mockResolvedValue();

  const { result } = renderHook(() => useImmich());
  await act(async () => {
    await result.current.linkPerson('uid-1', 'p-1', 'Ada');
  });

  expect(linkImmichPerson).toHaveBeenCalledWith('uid-1', 'p-1', 'Ada');
});

test('unlinkPerson unlinks and clears the summary', async () => {
  vi.mocked(getImmichContactSummary).mockResolvedValue(summary);
  vi.mocked(unlinkImmichPerson).mockResolvedValue();

  const { result } = renderHook(() => useImmich());
  await act(async () => {
    await result.current.loadSummary('uid-1');
  });
  expect(result.current.summary).toEqual(summary);

  await act(async () => {
    await result.current.unlinkPerson('uid-1');
  });

  expect(unlinkImmichPerson).toHaveBeenCalledWith('uid-1');
  expect(result.current.summary).toBeNull();
});

test('runSync syncs, refreshes the config and clears syncing', async () => {
  vi.mocked(syncImmich).mockResolvedValue();
  vi.mocked(getImmichConfig).mockResolvedValue(config);

  const { result } = renderHook(() => useImmich());
  await act(async () => {
    await result.current.runSync();
  });

  expect(syncImmich).toHaveBeenCalled();
  expect(getImmichConfig).toHaveBeenCalled();
  expect(result.current.syncing).toBe(false);
});

test('runSync notifies and rethrows on failure', async () => {
  vi.mocked(syncImmich).mockRejectedValue(new Error('sync failed'));
  const showError = vi.fn();

  const { result } = renderHook(() => useImmich({ showError }));
  await expect(result.current.runSync()).rejects.toThrow('sync failed');

  expect(showError).toHaveBeenCalledWith('sync failed');
  expect(result.current.syncing).toBe(false);
});

test('testConnection stores the diagnostic result and clears testing', async () => {
  const result_: ImmichConnectionTestResult = { ok: true, stage: 'auth', message: 'fine' };
  vi.mocked(testImmichConnection).mockResolvedValue(result_);

  const { result } = renderHook(() => useImmich());
  let fetched: ImmichConnectionTestResult | undefined;
  await act(async () => {
    fetched = await result.current.testConnection();
  });

  expect(fetched).toEqual(result_);
  expect(result.current.testResult).toEqual(result_);
  expect(result.current.testing).toBe(false);
});

test('testConnection resets a stale result and notifies on failure', async () => {
  const showError = vi.fn();
  vi.mocked(testImmichConnection).mockRejectedValue(new Error('conn failed'));

  const { result } = renderHook(() => useImmich({ showError }));
  await expect(result.current.testConnection()).rejects.toThrow('conn failed');

  expect(showError).toHaveBeenCalledWith('conn failed');
  expect(result.current.testResult).toBeNull();
  expect(result.current.testing).toBe(false);
});

test('runSync leaves syncing true while the sync is in flight', async () => {
  let resolveSync!: () => void;
  vi.mocked(syncImmich).mockReturnValue(new Promise<void>((r) => (resolveSync = () => r())));
  vi.mocked(getImmichConfig).mockResolvedValue(config);

  const { result } = renderHook(() => useImmich());
  let pending!: Promise<void>;
  act(() => {
    pending = result.current.runSync();
  });
  expect(result.current.syncing).toBe(true);

  await act(async () => {
    resolveSync();
    await pending;
  });
  expect(result.current.syncing).toBe(false);
});

test('peopleLoading toggles true while a people fetch is in flight', async () => {
  let resolve!: (v: ImmichPerson[]) => void;
  vi.mocked(getImmichPeople).mockReturnValue(new Promise((r) => (resolve = r)));

  const { result } = renderHook(() => useImmich());
  let pending!: Promise<ImmichPerson[]>;
  act(() => {
    pending = result.current.browsePeople();
  });
  expect(result.current.peopleLoading).toBe(true);

  await act(async () => {
    resolve([person]);
    await pending;
  });
  expect(result.current.peopleLoading).toBe(false);
});

test('summaryLoading toggles true while a summary fetch is in flight', async () => {
  let resolve!: (v: ImmichPersonSummary) => void;
  vi.mocked(getImmichContactSummary).mockReturnValue(new Promise((r) => (resolve = r)));

  const { result } = renderHook(() => useImmich());
  let pending!: Promise<ImmichPersonSummary | null>;
  act(() => {
    pending = result.current.loadSummary('uid-1');
  });
  expect(result.current.summaryLoading).toBe(true);

  await act(async () => {
    resolve(summary);
    await pending;
  });
  expect(result.current.summaryLoading).toBe(false);
});
