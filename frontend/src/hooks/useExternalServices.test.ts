import { act, cleanup, renderHook } from '@testing-library/react';
import type { Mock } from 'vitest';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import {
  deleteImmichConfig,
  getImmichConfig,
  getImmichContactSummary,
  getImmichPeople,
  linkImmichPerson,
  saveImmichConfig,
  syncImmich,
  testImmichConnection,
  unlinkImmichPerson,
} from '../api/immich';
import {
  deleteNextcloudConfig,
  getNextcloudConfig,
  getNextcloudDir,
  saveNextcloudConfig,
  testNextcloudConnection,
} from '../api/nextcloud';
import {
  deletePaperlessConfig,
  getPaperlessConfig,
  getPaperlessDocuments,
  savePaperlessConfig,
  testPaperlessConnection,
} from '../api/paperless';
import {
  deleteSeafileConfig,
  getSeafileConfig,
  getSeafileDir,
  getSeafileLibraries,
  saveSeafileConfig,
  testSeafileConnection,
} from '../api/seafile';
import { useImmich } from './useImmich';
import { useNextcloud } from './useNextcloud';
import { usePaperless } from './usePaperless';
import { useSeafile } from './useSeafile';

// This codebase's vitest setup does not auto-cleanup between tests.
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

vi.mock('../api/paperless', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/paperless')>();
  return {
    ...actual,
    getPaperlessConfig: vi.fn(),
    savePaperlessConfig: vi.fn(),
    deletePaperlessConfig: vi.fn(),
    testPaperlessConnection: vi.fn(),
    getPaperlessDocuments: vi.fn(),
  };
});

vi.mock('../api/nextcloud', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/nextcloud')>();
  return {
    ...actual,
    getNextcloudConfig: vi.fn(),
    saveNextcloudConfig: vi.fn(),
    deleteNextcloudConfig: vi.fn(),
    testNextcloudConnection: vi.fn(),
    getNextcloudDir: vi.fn(),
  };
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
  vi.clearAllMocks();
});

// The four external-service hooks are near-identical wrappers over their
// service's config/browse/test endpoints; one table drives the shared
// contract (load/save/remove/test), per-service branches follow.
interface ServiceRow {
  name: string;
  // `use` returns any of the four hooks' return shapes, which differ in the
  // saveConfig input type — the row only exercises the shared contract, so a
  // loose return type is deliberate.
  use: (notifier?: { showError: (m: string) => void }) => any;
  fetchConfig: Mock;
  saveConfigApi: Mock;
  deleteConfigApi: Mock;
  testConnectionApi: Mock;
  saveInput: unknown;
  configFixture: unknown;
  testFixture: unknown;
}

const services: ServiceRow[] = [
  {
    name: 'seafile',
    use: (notifier) => useSeafile(notifier),
    fetchConfig: getSeafileConfig as Mock,
    saveConfigApi: saveSeafileConfig as Mock,
    deleteConfigApi: deleteSeafileConfig as Mock,
    testConnectionApi: testSeafileConnection as Mock,
    saveInput: { base_url: 'https://seafile.example' },
    configFixture: { base_url: 'https://seafile.example', has_api_token: true },
    testFixture: { ok: true, stage: 'done', message: 'connected' },
  },
  {
    name: 'paperless',
    use: (notifier) => usePaperless(notifier),
    fetchConfig: getPaperlessConfig as Mock,
    saveConfigApi: savePaperlessConfig as Mock,
    deleteConfigApi: deletePaperlessConfig as Mock,
    testConnectionApi: testPaperlessConnection as Mock,
    saveInput: { base_url: 'https://paperless.example' },
    configFixture: { base_url: 'https://paperless.example', has_api_token: true },
    testFixture: { ok: true, stage: 'done', message: 'connected' },
  },
  {
    name: 'nextcloud',
    use: (notifier) => useNextcloud(notifier),
    fetchConfig: getNextcloudConfig as Mock,
    saveConfigApi: saveNextcloudConfig as Mock,
    deleteConfigApi: deleteNextcloudConfig as Mock,
    testConnectionApi: testNextcloudConnection as Mock,
    saveInput: { base_url: 'https://nc.example', username: 'alice' },
    configFixture: { base_url: 'https://nc.example', username: 'alice', has_app_password: true },
    testFixture: { ok: true, stage: 'done', message: 'connected' },
  },
  {
    name: 'immich',
    use: (notifier) => useImmich(notifier),
    fetchConfig: getImmichConfig as Mock,
    saveConfigApi: saveImmichConfig as Mock,
    deleteConfigApi: deleteImmichConfig as Mock,
    testConnectionApi: testImmichConnection as Mock,
    saveInput: { base_url: 'https://immich.example', sync_enabled: true },
    configFixture: {
      base_url: 'https://immich.example',
      has_api_key: true,
      sync_enabled: true,
      last_sync_status: 'ok',
      last_sync_error: '',
    },
    testFixture: { ok: true, stage: 'done', message: 'connected' },
  },
];

test('refreshConfig loads the saved config', async () => {
  for (const s of services) {
    vi.mocked(s.fetchConfig).mockResolvedValue(s.configFixture);

    const { result } = renderHook(() => s.use());
    await act(async () => {
      await result.current.refreshConfig();
    });

    expect(s.fetchConfig).toHaveBeenCalledTimes(1);
    expect(result.current.config).toEqual(s.configFixture);
    expect(result.current.configLoading).toBe(false);
    expect(result.current.configError).toBeNull();
  }
});

test('refreshConfig surfaces a fetch error', async () => {
  for (const s of services) {
    vi.mocked(s.fetchConfig).mockRejectedValue(new Error('boom'));

    const { result } = renderHook(() => s.use());
    await act(async () => {
      await result.current.refreshConfig();
    });

    expect(result.current.configError).toBe('boom');
    expect(result.current.config).toBeNull();
    expect(result.current.configLoading).toBe(false);
  }
});

test('saveConfig persists, stores and returns the config', async () => {
  for (const s of services) {
    vi.mocked(s.saveConfigApi).mockResolvedValue(s.configFixture);

    const { result } = renderHook(() => s.use());
    let saved: unknown;
    await act(async () => {
      saved = await result.current.saveConfig(s.saveInput);
    });

    expect(s.saveConfigApi).toHaveBeenCalledWith(s.saveInput);
    expect(saved).toEqual(s.configFixture);
    expect(result.current.config).toEqual(s.configFixture);
  }
});

test('saveConfig notifies and rethrows on failure', async () => {
  for (const s of services) {
    vi.mocked(s.saveConfigApi).mockRejectedValue(new Error('save failed'));
    const notifier = { showError: vi.fn() };

    const { result } = renderHook(() => s.use(notifier));
    await expect(
      act(async () => {
        await result.current.saveConfig(s.saveInput);
      }),
    ).rejects.toThrow('save failed');

    expect(notifier.showError).toHaveBeenCalledWith('save failed');
    expect(result.current.config).toBeNull();
  }
});

test('removeConfig deletes the remote config and clears local state', async () => {
  for (const s of services) {
    vi.mocked(s.fetchConfig).mockResolvedValue(s.configFixture);
    vi.mocked(s.deleteConfigApi).mockResolvedValue(undefined);

    const { result } = renderHook(() => s.use());
    await act(async () => {
      await result.current.refreshConfig();
    });
    expect(result.current.config).toEqual(s.configFixture);

    await act(async () => {
      await result.current.removeConfig();
    });
    expect(s.deleteConfigApi).toHaveBeenCalledTimes(1);
    expect(result.current.config).toBeNull();
  }
});

test('testConnection stores and returns the diagnostic result', async () => {
  for (const s of services) {
    vi.mocked(s.testConnectionApi).mockResolvedValue(s.testFixture);

    const { result } = renderHook(() => s.use());
    let tested: unknown;
    await act(async () => {
      tested = await result.current.testConnection();
    });

    expect(s.testConnectionApi).toHaveBeenCalledTimes(1);
    expect(tested).toEqual(s.testFixture);
    expect(result.current.testResult).toEqual(s.testFixture);
    expect(result.current.testing).toBe(false);
  }
});

test('testConnection notifies and rethrows when the check cannot run', async () => {
  for (const s of services) {
    vi.mocked(s.testConnectionApi).mockRejectedValue(new Error('no connection saved'));
    const notifier = { showError: vi.fn() };

    const { result } = renderHook(() => s.use(notifier));
    await expect(
      act(async () => {
        await result.current.testConnection();
      }),
    ).rejects.toThrow('no connection saved');

    expect(notifier.showError).toHaveBeenCalledWith('no connection saved');
    expect(result.current.testing).toBe(false);
  }
});

// --- Seafile: library/folder browsing ---

test('seafile browseLibraries loads libraries and toggles browsing', async () => {
  const libraries = [{ id: 'lib-1', name: 'Docs', type: 'library' }];
  vi.mocked(getSeafileLibraries).mockResolvedValue(libraries);

  const { result } = renderHook(() => useSeafile());
  await act(async () => {
    await result.current.browseLibraries();
  });

  expect(getSeafileLibraries).toHaveBeenCalledTimes(1);
  expect(result.current.libraries).toEqual(libraries);
  expect(result.current.browsing).toBe(false);
});

test('seafile browseLibraries rethrows on failure', async () => {
  vi.mocked(getSeafileLibraries).mockRejectedValue(new Error('browse failed'));

  const { result } = renderHook(() => useSeafile());
  await expect(
    act(async () => {
      await result.current.browseLibraries();
    }),
  ).rejects.toThrow('browse failed');
  expect(result.current.browsing).toBe(false);
});

test('seafile browseDir loads a folder listing', async () => {
  const items = [{ id: 'f-1', name: 'report.pdf', type: 'file' as const }];
  vi.mocked(getSeafileDir).mockResolvedValue(items);

  const { result } = renderHook(() => useSeafile());
  await act(async () => {
    await result.current.browseDir('lib-1', '/Contracts');
  });

  expect(getSeafileDir).toHaveBeenCalledWith('lib-1', '/Contracts');
  expect(result.current.items).toEqual(items);
});

// --- Paperless: document search ---

test('paperless browseDocuments passes the query through and stores results', async () => {
  const docs = [{ id: 1, title: 'invoice' }];
  vi.mocked(getPaperlessDocuments).mockResolvedValue(docs);

  const { result } = renderHook(() => usePaperless());
  await act(async () => {
    await result.current.browseDocuments('tax');
  });

  expect(getPaperlessDocuments).toHaveBeenCalledWith('tax');
  expect(result.current.documents).toEqual(docs);
  expect(result.current.documentsLoading).toBe(false);
});

test('paperless browseDocuments omits the query when none given', async () => {
  vi.mocked(getPaperlessDocuments).mockResolvedValue([]);

  const { result } = renderHook(() => usePaperless());
  await act(async () => {
    await result.current.browseDocuments();
  });

  expect(getPaperlessDocuments).toHaveBeenCalledWith(undefined);
});

// --- Nextcloud: folder browsing ---

test('nextcloud browseDir passes the path through and stores results', async () => {
  const items = [{ name: 'docs', path: '/docs', type: 'dir' as const }];
  vi.mocked(getNextcloudDir).mockResolvedValue(items);

  const { result } = renderHook(() => useNextcloud());
  await act(async () => {
    await result.current.browseDir('/files');
  });

  expect(getNextcloudDir).toHaveBeenCalledWith('/files');
  expect(result.current.items).toEqual(items);
  expect(result.current.browsing).toBe(false);
});

test('nextcloud browseDir defaults to the root when no path given', async () => {
  vi.mocked(getNextcloudDir).mockResolvedValue([]);

  const { result } = renderHook(() => useNextcloud());
  await act(async () => {
    await result.current.browseDir();
  });

  expect(getNextcloudDir).toHaveBeenCalledWith(undefined);
});

// --- Immich: people, summary, linking, sync ---

test('immich browsePeople loads people', async () => {
  const people = [{ id: 'p-1', name: 'Ada' }];
  vi.mocked(getImmichPeople).mockResolvedValue(people);

  const { result } = renderHook(() => useImmich());
  await act(async () => {
    await result.current.browsePeople();
  });

  expect(getImmichPeople).toHaveBeenCalledTimes(1);
  expect(result.current.people).toEqual(people);
  expect(result.current.peopleLoading).toBe(false);
});

test('immich loadSummary loads and stores the per-contact summary', async () => {
  const summary = {
    identity: {
      id: 'i-1',
      entity_id: 'c-1',
      system: 'immich',
      external_id: 'p-1',
      sync_status: 'ok',
    },
    person_name: 'Ada',
    photo_count: 3,
  };
  vi.mocked(getImmichContactSummary).mockResolvedValue(summary);

  const { result } = renderHook(() => useImmich());
  await act(async () => {
    await result.current.loadSummary('c-1');
  });

  expect(getImmichContactSummary).toHaveBeenCalledWith('c-1');
  expect(result.current.summary).toEqual(summary);
});

test('immich loadSummary returns null (not a throw) when no link exists', async () => {
  vi.mocked(getImmichContactSummary).mockRejectedValue(new Error('not linked'));

  const { result } = renderHook(() => useImmich());
  let loaded: unknown;
  await act(async () => {
    loaded = await result.current.loadSummary('c-1');
  });

  expect(loaded).toBeNull();
  expect(result.current.summary).toBeNull();
  expect(result.current.summaryLoading).toBe(false);
});

test('immich linkPerson and unlinkPerson update the summary state', async () => {
  vi.mocked(linkImmichPerson).mockResolvedValue(undefined);
  vi.mocked(unlinkImmichPerson).mockResolvedValue(undefined);
  vi.mocked(getImmichContactSummary).mockResolvedValue(null);

  const { result } = renderHook(() => useImmich());
  await act(async () => {
    await result.current.linkPerson('c-1', 'p-1', 'Ada');
  });
  expect(linkImmichPerson).toHaveBeenCalledWith('c-1', 'p-1', 'Ada');

  const summary = {
    identity: {
      id: 'i-1',
      entity_id: 'c-1',
      system: 'immich',
      external_id: 'p-1',
      sync_status: 'ok',
    },
    person_name: 'Ada',
    photo_count: 3,
  };
  vi.mocked(getImmichContactSummary).mockResolvedValue(summary);
  await act(async () => {
    await result.current.loadSummary('c-1');
  });
  expect(result.current.summary).toEqual(summary);

  await act(async () => {
    await result.current.unlinkPerson('c-1');
  });
  expect(unlinkImmichPerson).toHaveBeenCalledWith('c-1');
  expect(result.current.summary).toBeNull();
});

test('immich runSync syncs and refreshes the config', async () => {
  vi.mocked(syncImmich).mockResolvedValue(undefined);
  vi.mocked(getImmichConfig).mockResolvedValue({
    base_url: 'https://immich.example',
    has_api_key: true,
    sync_enabled: true,
    last_sync_status: 'ok',
    last_sync_error: '',
  });

  const { result } = renderHook(() => useImmich());
  await act(async () => {
    await result.current.runSync();
  });

  expect(syncImmich).toHaveBeenCalledTimes(1);
  expect(getImmichConfig).toHaveBeenCalledTimes(1);
  expect(result.current.config).toEqual({
    base_url: 'https://immich.example',
    has_api_key: true,
    sync_enabled: true,
    last_sync_status: 'ok',
    last_sync_error: '',
  });
  expect(result.current.syncing).toBe(false);
});

test('immich runSync rethrows on failure', async () => {
  vi.mocked(syncImmich).mockRejectedValue(new Error('sync failed'));

  const { result } = renderHook(() => useImmich());
  await expect(
    act(async () => {
      await result.current.runSync();
    }),
  ).rejects.toThrow('sync failed');
  expect(result.current.syncing).toBe(false);
});
