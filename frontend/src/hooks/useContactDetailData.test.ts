import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { getContactActivities } from '../api/activities';
import { getCurrentUser } from '../api/admin';
import {
  type ContactRecordResponse,
  getContactProfilePicture,
  getContactRecord,
} from '../api/contacts';
import { getContactNotes, type Note } from '../api/notes';
import { getCompletionsForContact } from '../api/reminders';
import type { User } from '../types';
import {
  type ContactDetailCore,
  fetchContactDetailCore,
  fetchProfilePictureUrl,
  useContactDetailData,
  useContactDetailLoader,
} from './useContactDetailData';

vi.mock('../api/activities', () => ({ getContactActivities: vi.fn() }));
vi.mock('../api/admin', () => ({ getCurrentUser: vi.fn() }));
vi.mock('../api/notes', () => ({ getContactNotes: vi.fn() }));
vi.mock('../api/reminders', () => ({ getCompletionsForContact: vi.fn() }));
vi.mock('../api/contacts', () => ({
  getContactRecord: vi.fn(),
  getContactProfilePicture: vi.fn(),
}));

const record: ContactRecordResponse = {
  id: 1,
  uid: 'alice-uid',
  etag: '',
  revision: 1,
  card: { name: { components: [{ kind: 'given', value: 'Alice' }] } },
  crm: {},
};

const note: Note = {
  ID: 3,
  content: 'hello',
  date: '2024-01-01T00:00:00Z',
  CreatedAt: '2024-01-01T00:00:00Z',
  UpdatedAt: '2024-01-01T00:00:00Z',
};

const user = (overrides: Partial<User> = {}): User => ({
  id: 1,
  email: 'a@example.com',
  username: 'a',
  language: 'en',
  is_admin: false,
  created_at: '',
  updated_at: '',
  ...overrides,
});

let consoleError: ReturnType<typeof vi.spyOn>;

beforeEach(() => {
  consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
  localStorage.clear();
  vi.mocked(getContactRecord).mockResolvedValue(record);
  vi.mocked(getContactNotes).mockResolvedValue({ notes: [note] } as never);
  vi.mocked(getContactActivities).mockResolvedValue({ activities: [] });
  vi.mocked(getCompletionsForContact).mockResolvedValue([]);
  vi.mocked(getCurrentUser).mockResolvedValue(user());
  vi.mocked(getContactProfilePicture).mockResolvedValue(null as never);
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

describe('fetchContactDetailCore', () => {
  test('returns every batch with no aux failure', async () => {
    const core = await fetchContactDetailCore('1');
    expect(core).toEqual({
      record,
      notes: [note],
      activities: [],
      completions: [],
      user: user(),
      auxFetchFailed: false,
    });
    expect(getCompletionsForContact).toHaveBeenCalledWith(1);
  });

  test('normalizes missing list keys to empty arrays', async () => {
    vi.mocked(getContactNotes).mockResolvedValue({} as never);
    vi.mocked(getContactActivities).mockResolvedValue({} as never);
    vi.mocked(getCompletionsForContact).mockResolvedValue(undefined as never);
    const core = await fetchContactDetailCore('1');
    expect(core.notes).toEqual([]);
    expect(core.activities).toEqual([]);
    expect(core.completions).toEqual([]);
  });

  test.each([
    ['notes', () => vi.mocked(getContactNotes).mockRejectedValue(new Error('boom'))],
    ['activities', () => vi.mocked(getContactActivities).mockRejectedValue(new Error('boom'))],
    ['completions', () => vi.mocked(getCompletionsForContact).mockRejectedValue(new Error('boom'))],
  ])('a failed %s fetch falls back to empty and flags auxFetchFailed (#958)', async (_n, fail) => {
    fail();
    const core = await fetchContactDetailCore('1');
    expect(core.record).toBe(record);
    expect(core.auxFetchFailed).toBe(true);
  });

  test('a failed /users/me falls back to null without flagging the timeline', async () => {
    vi.mocked(getCurrentUser).mockRejectedValue(new Error('nope'));
    const core = await fetchContactDetailCore('1');
    expect(core.user).toBeNull();
    expect(core.auxFetchFailed).toBe(false);
  });

  test('a failed record fetch rejects the whole batch', async () => {
    vi.mocked(getContactRecord).mockRejectedValue(new Error('404'));
    await expect(fetchContactDetailCore('1')).rejects.toThrow('404');
  });
});

describe('fetchProfilePictureUrl', () => {
  test('skips the fetch when the record has no photo', async () => {
    expect(await fetchProfilePictureUrl('1', false)).toBe('');
    expect(getContactProfilePicture).not.toHaveBeenCalled();
  });

  test('returns an object URL for a blob', async () => {
    const createObjectURL = vi.fn(() => 'blob:pic');
    vi.stubGlobal('URL', { ...URL, createObjectURL, revokeObjectURL: vi.fn() });
    vi.mocked(getContactProfilePicture).mockResolvedValue(new Blob(['x']));
    expect(await fetchProfilePictureUrl('1', true)).toBe('blob:pic');
    vi.unstubAllGlobals();
  });

  test('returns empty when there is no blob, undefined when the fetch fails', async () => {
    expect(await fetchProfilePictureUrl('1', true)).toBe('');
    vi.mocked(getContactProfilePicture).mockRejectedValue(new Error('x'));
    expect(await fetchProfilePictureUrl('1', true)).toBeUndefined();
    expect(consoleError).toHaveBeenCalled();
  });
});

describe('useContactDetailData', () => {
  test('starts loading with defaults, seeding selfContactUid from the cache', () => {
    localStorage.setItem('user_info', JSON.stringify({ self_contact_vcard_uid: 'me-uid' }));
    const { result } = renderHook(() => useContactDetailData('1'));
    expect(result.current.loading).toBe(true);
    expect(result.current.record).toBeNull();
    expect(result.current.notes).toEqual([]);
    expect(result.current.timelineRevision).toBe(0);
    expect(result.current.selfContactUid).toBe('me-uid');
  });

  test('applyCore sets state and takes the fetched self-contact only when the user loaded', () => {
    const { result } = renderHook(() => useContactDetailData('1'));
    const core: ContactDetailCore = {
      record,
      notes: [note],
      activities: [],
      completions: [],
      user: user({
        self_contact_vcard_uid: 'alice-uid',
        enabled_contact_fields: ['organizations'],
      }),
      auxFetchFailed: false,
    };
    act(() => result.current.applyCore(core));
    expect(result.current.record).toBe(record);
    expect(result.current.notes).toEqual([note]);
    expect(result.current.selfContactUid).toBe('alice-uid');
    expect([...result.current.enabledFields]).toEqual(['organizations']);

    act(() => result.current.applyCore({ ...core, user: null }));
    // A failed /users/me keeps the last known "Me" pointer.
    expect(result.current.selfContactUid).toBe('alice-uid');

    act(() => result.current.applyCore({ ...core, user: user() }));
    expect(result.current.selfContactUid).toBeNull();
  });

  test('refreshNotesAndActivities reloads the timeline lists and bumps the revision', async () => {
    const { result } = renderHook(() => useContactDetailData('1'));
    await act(() => result.current.refreshNotesAndActivities());
    expect(result.current.notes).toEqual([note]);
    expect(result.current.timelineRevision).toBe(1);
  });

  test('refreshNotesAndActivities keeps state when a fetch fails', async () => {
    vi.mocked(getContactActivities).mockRejectedValue(new Error('x'));
    const { result } = renderHook(() => useContactDetailData('1'));
    await act(() => result.current.refreshNotesAndActivities());
    expect(result.current.notes).toEqual([]);
    expect(result.current.timelineRevision).toBe(0);
  });

  test('refresh and reload are no-ops without an id', async () => {
    const { result } = renderHook(() => useContactDetailData(undefined));
    await act(() => result.current.refreshNotesAndActivities());
    await act(() => result.current.reloadRecord());
    expect(getContactNotes).not.toHaveBeenCalled();
    expect(getContactRecord).not.toHaveBeenCalled();
  });

  test('reloadRecord replaces the record, and leaves it on failure', async () => {
    const { result } = renderHook(() => useContactDetailData('1'));
    await act(() => result.current.reloadRecord());
    expect(result.current.record).toBe(record);
    vi.mocked(getContactRecord).mockRejectedValue(new Error('x'));
    await act(() => result.current.reloadRecord());
    expect(result.current.record).toBe(record);
  });
});

describe('useContactDetailLoader', () => {
  function setup(id: string | undefined, loadDependents = vi.fn(async () => {})) {
    const applyCore = vi.fn();
    const setProfilePic = vi.fn();
    const setLoading = vi.fn();
    const onAuxFetchFailed = vi.fn();
    const hook = renderHook(
      ({ currentId }: { currentId: string | undefined }) =>
        useContactDetailLoader(currentId, {
          applyCore,
          setProfilePic,
          setLoading,
          loadDependents,
          onAuxFetchFailed,
        }),
      { initialProps: { currentId: id } },
    );
    return { ...hook, applyCore, setProfilePic, setLoading, onAuxFetchFailed, loadDependents };
  }

  test('does nothing without an id', () => {
    const { applyCore } = setup(undefined);
    expect(getContactRecord).not.toHaveBeenCalled();
    expect(applyCore).not.toHaveBeenCalled();
  });

  test('loads core, then dependents with the fetched record, then clears loading', async () => {
    const { applyCore, loadDependents, setProfilePic, setLoading, onAuxFetchFailed } = setup('1');
    await waitFor(() => expect(setLoading).toHaveBeenCalledWith(false));
    expect(applyCore).toHaveBeenCalledWith(expect.objectContaining({ record }));
    expect(loadDependents).toHaveBeenCalledWith(record);
    expect(setProfilePic).toHaveBeenCalledWith('');
    expect(onAuxFetchFailed).not.toHaveBeenCalled();
  });

  test('reports an aux timeline failure but still loads the page', async () => {
    vi.mocked(getContactNotes).mockRejectedValue(new Error('500'));
    const { onAuxFetchFailed, setLoading, loadDependents } = setup('1');
    await waitFor(() => expect(setLoading).toHaveBeenCalledWith(false));
    expect(onAuxFetchFailed).toHaveBeenCalledTimes(1);
    expect(loadDependents).toHaveBeenCalled();
  });

  test('a failed record fetch clears loading without applying anything', async () => {
    vi.mocked(getContactRecord).mockRejectedValue(new Error('404'));
    const { applyCore, setLoading, loadDependents } = setup('1');
    await waitFor(() => expect(setLoading).toHaveBeenCalledWith(false));
    expect(applyCore).not.toHaveBeenCalled();
    expect(loadDependents).not.toHaveBeenCalled();
  });

  test('a failed profile picture leaves the current picture untouched', async () => {
    vi.mocked(getContactRecord).mockResolvedValue({ ...record, photo: 'p.jpg' });
    vi.mocked(getContactProfilePicture).mockRejectedValue(new Error('x'));
    const { setProfilePic, setLoading } = setup('1');
    await waitFor(() => expect(setLoading).toHaveBeenCalledWith(false));
    expect(setProfilePic).not.toHaveBeenCalled();
  });

  test('revokes the loaded blob URL on unmount', async () => {
    const revokeObjectURL = vi.fn();
    vi.stubGlobal('URL', { ...URL, createObjectURL: () => 'blob:pic', revokeObjectURL });
    vi.mocked(getContactRecord).mockResolvedValue({ ...record, photo: 'p.jpg' });
    vi.mocked(getContactProfilePicture).mockResolvedValue(new Blob(['x']));
    const { setProfilePic, unmount } = setup('1');
    await waitFor(() => expect(setProfilePic).toHaveBeenCalledWith('blob:pic'));
    unmount();
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:pic');
    vi.unstubAllGlobals();
  });

  test('re-runs when the id changes', async () => {
    const { rerender, setLoading } = setup('1');
    await waitFor(() => expect(setLoading).toHaveBeenCalledWith(false));
    rerender({ currentId: '2' });
    await waitFor(() => expect(getContactRecord).toHaveBeenCalledWith('2'));
  });

  test('a new loadDependents identity does not refetch the page', async () => {
    const applyCore = vi.fn();
    const setProfilePic = vi.fn();
    const setLoading = vi.fn();
    const first = vi.fn(async () => {});
    const second = vi.fn(async () => {});
    const { rerender } = renderHook(
      ({ deps }: { deps: typeof first }) =>
        useContactDetailLoader('1', {
          applyCore,
          setProfilePic,
          setLoading,
          loadDependents: deps,
          onAuxFetchFailed: () => {},
        }),
      { initialProps: { deps: first } },
    );
    await waitFor(() => expect(setLoading).toHaveBeenCalledWith(false));
    rerender({ deps: second });
    await act(async () => {});
    expect(getContactRecord).toHaveBeenCalledTimes(1);
    expect(first).toHaveBeenCalledTimes(1);
    expect(second).not.toHaveBeenCalled();
  });

  test("a slow response for the previous id never overwrites the new contact's state", async () => {
    let resolveFirst: (r: ContactRecordResponse) => void = () => {};
    vi.mocked(getContactRecord).mockImplementation((id: string | number) =>
      String(id) === '1'
        ? new Promise<ContactRecordResponse>((resolve) => {
            resolveFirst = resolve;
          })
        : Promise.resolve({ ...record, id: 2, uid: 'bob-uid' }),
    );
    const { rerender, applyCore, setLoading } = setup('1');
    rerender({ currentId: '2' });
    await waitFor(() => expect(setLoading).toHaveBeenCalledWith(false));
    expect(applyCore).toHaveBeenCalledTimes(1);

    await act(async () => {
      resolveFirst(record);
    });
    expect(applyCore).toHaveBeenCalledTimes(1);
    expect(applyCore).toHaveBeenLastCalledWith(
      expect.objectContaining({ record: expect.objectContaining({ uid: 'bob-uid' }) }),
    );
  });
});
