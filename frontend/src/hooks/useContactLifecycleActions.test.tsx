import { act, cleanup, renderHook } from '@testing-library/react';
import type { ReactNode } from 'react';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router';
import { afterEach, beforeEach, describe, expect, type Mock, test, vi } from 'vitest';
import '../i18n/config';
import { ApiError } from '../api/client';
import {
  archiveContact,
  type ContactRecordResponse,
  deleteContact,
  favoriteContact,
  getContactRecord,
  unarchiveContact,
  unfavoriteContact,
} from '../api/contacts';
import { clearTargetCloseness, setTargetCloseness } from '../api/targetCloseness';
import { updateSelfContact } from '../api/users';
import { fetchAndCacheUserInfo } from '../auth';
import { useContactLifecycleActions } from './useContactLifecycleActions';

vi.mock('../api/contacts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/contacts')>();
  return {
    ...actual,
    archiveContact: vi.fn(),
    unarchiveContact: vi.fn(),
    deleteContact: vi.fn(),
    favoriteContact: vi.fn(),
    getContactRecord: vi.fn(),
    unfavoriteContact: vi.fn(),
  };
});
vi.mock('../api/targetCloseness', () => ({
  setTargetCloseness: vi.fn(),
  clearTargetCloseness: vi.fn(),
}));
vi.mock('../api/users', () => ({ updateSelfContact: vi.fn() }));
vi.mock('../auth', () => ({ fetchAndCacheUserInfo: vi.fn() }));

const UPDATE_ERROR = 'Failed to update contact. Please check your input and try again.';

const record: ContactRecordResponse = {
  id: 1,
  uid: 'alice-uid',
  etag: '',
  revision: 1,
  card: {},
  crm: {},
};

let location = '';
function LocationProbe() {
  location = useLocation().pathname;
  return null;
}
const wrapper = ({ children }: { children: ReactNode }) => (
  <MemoryRouter initialEntries={['/contacts/1']}>
    {children}
    <Routes>
      <Route path="*" element={<LocationProbe />} />
    </Routes>
  </MemoryRouter>
);

let setRecord: Mock<(value: unknown) => void>;
let setSelfContactUid: Mock<(value: unknown) => void>;
let showError: Mock<(value: unknown) => void>;
let showSuccess: Mock<(value: unknown) => void>;
let confirmSpy: ReturnType<typeof vi.spyOn>;
let alertSpy: ReturnType<typeof vi.spyOn>;

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
  alertSpy = vi.spyOn(window, 'alert').mockImplementation(() => {});
  setRecord = vi.fn();
  setSelfContactUid = vi.fn();
  showError = vi.fn();
  showSuccess = vi.fn();
  // The server's revision after the action's own write bumped it.
  vi.mocked(getContactRecord).mockResolvedValue({ ...record, revision: record.revision + 1 });
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

function setup(
  opts: {
    r?: ContactRecordResponse | null;
    id?: string;
    selfContactUid?: string | null;
    onConflict?: () => void;
  } = {},
) {
  const r = opts.r === undefined ? record : opts.r;
  const id = 'id' in opts ? opts.id : '1';
  return renderHook(
    () =>
      useContactLifecycleActions({
        id,
        record: r,
        setRecord,
        displayName: 'Alice Wonder',
        selfContactUid: opts.selfContactUid ?? null,
        setSelfContactUid,
        showError,
        showSuccess,
        onConflict: opts.onConflict,
      }),
    { wrapper },
  );
}

// Applies every functional setRecord update to `start`, in order.
function applyUpdates(start: ContactRecordResponse): ContactRecordResponse {
  return setRecord.mock.calls.reduce(
    (acc: ContactRecordResponse, [u]) =>
      typeof u === 'function'
        ? (u as (p: ContactRecordResponse) => ContactRecordResponse)(acc)
        : (u as ContactRecordResponse),
    start,
  );
}

test('every action is a no-op without a record', async () => {
  const { result } = setup({ r: null });
  await act(async () => {
    await result.current.handleDeleteContact();
    await result.current.handleArchiveContact();
    await result.current.handleUnarchiveContact();
    await result.current.handleToggleFavorite();
    await result.current.handleSetTargetCloseness('close');
    await result.current.handleToggleMe();
  });
  expect(confirmSpy).not.toHaveBeenCalled();
  expect(updateSelfContact).not.toHaveBeenCalled();
  expect(setRecord).not.toHaveBeenCalled();
});

describe('delete', () => {
  test('confirms with the display name, deletes, and navigates to the list', async () => {
    const { result } = setup();
    await act(() => result.current.handleDeleteContact());
    expect(confirmSpy.mock.calls[0][0]).toContain('Alice Wonder');
    expect(deleteContact).toHaveBeenCalledWith('1', record.revision);
    expect(location).toBe('/contacts');
  });

  test('a 412 (stale revision) calls onConflict instead of alerting and stays', async () => {
    vi.mocked(deleteContact).mockRejectedValue(new ApiError('stale', 'PRECONDITION_FAILED', 412));
    const onConflict = vi.fn();
    const { result } = setup({ onConflict });
    await act(() => result.current.handleDeleteContact());
    expect(onConflict).toHaveBeenCalledTimes(1);
    expect(alertSpy).not.toHaveBeenCalled();
    expect(location).toBe('/contacts/1');
  });

  test('declining makes no request', async () => {
    confirmSpy.mockReturnValue(false);
    const { result } = setup();
    await act(() => result.current.handleDeleteContact());
    expect(deleteContact).not.toHaveBeenCalled();
  });

  test('a failure alerts and stays', async () => {
    vi.mocked(deleteContact).mockRejectedValue(new Error('x'));
    const { result } = setup();
    await act(() => result.current.handleDeleteContact());
    expect(alertSpy).toHaveBeenCalledWith('Failed to delete contact. Please try again.');
    expect(location).toBe('/contacts/1');
  });
});

describe('archive / unarchive', () => {
  test('archive confirms and takes only the archived flag', async () => {
    vi.mocked(archiveContact).mockResolvedValue({ archived: true } as never);
    const { result } = setup();
    await act(() => result.current.handleArchiveContact());
    expect(setRecord).toHaveBeenCalledWith({ ...record, archived: true });
  });

  test('declining the archive makes no request', async () => {
    confirmSpy.mockReturnValue(false);
    const { result } = setup();
    await act(() => result.current.handleArchiveContact());
    expect(archiveContact).not.toHaveBeenCalled();
  });

  test('archive failures report the ApiError message, else the generic one', async () => {
    const { result } = setup();
    vi.mocked(archiveContact).mockRejectedValueOnce(new ApiError('locked', 'X', 409));
    await act(() => result.current.handleArchiveContact());
    expect(showError).toHaveBeenLastCalledWith('locked');
    vi.mocked(archiveContact).mockRejectedValueOnce(new Error('x'));
    await act(() => result.current.handleArchiveContact());
    expect(showError).toHaveBeenLastCalledWith(UPDATE_ERROR);
    expect(setRecord).not.toHaveBeenCalled();
  });

  test('unarchive needs no confirmation and reports failures', async () => {
    vi.mocked(unarchiveContact).mockResolvedValueOnce({ archived: false } as never);
    const { result } = setup({ r: { ...record, archived: true } });
    await act(() => result.current.handleUnarchiveContact());
    expect(confirmSpy).not.toHaveBeenCalled();
    expect(setRecord).toHaveBeenCalledWith({ ...record, archived: false });

    vi.mocked(unarchiveContact).mockRejectedValueOnce(new ApiError('nope', 'X', 400));
    await act(() => result.current.handleUnarchiveContact());
    expect(showError).toHaveBeenLastCalledWith('nope');
  });

  test('actions need an id as well as a record', async () => {
    const { result } = setup({ id: undefined });
    await act(async () => {
      await result.current.handleArchiveContact();
      await result.current.handleUnarchiveContact();
      await result.current.handleToggleFavorite();
      await result.current.handleSetTargetCloseness('close');
      await result.current.handleDeleteContact();
    });
    expect(archiveContact).not.toHaveBeenCalled();
    expect(setTargetCloseness).not.toHaveBeenCalled();
    expect(unarchiveContact).not.toHaveBeenCalled();
    expect(favoriteContact).not.toHaveBeenCalled();
    expect(deleteContact).not.toHaveBeenCalled();
  });
});

// ADR 0008: favorite/archive persist the contact and bump its revision, but
// their flat response has none -- the held revision must be re-read or the
// next save/delete would 412 against our own toggle.
describe('revision stays current after whole-contact actions', () => {
  const bumped = record.revision + 1;

  test('favorite re-reads the revision', async () => {
    vi.mocked(favoriteContact).mockResolvedValue({ is_favorite: true } as never);
    const { result } = setup();
    await act(() => result.current.handleToggleFavorite());
    expect(getContactRecord).toHaveBeenCalledWith('1');
    expect(applyUpdates(record).revision).toBe(bumped);
  });

  test('unfavorite re-reads the revision', async () => {
    vi.mocked(unfavoriteContact).mockResolvedValue({ is_favorite: false } as never);
    const { result } = setup({ r: { ...record, is_favorite: true } });
    await act(() => result.current.handleToggleFavorite());
    expect(applyUpdates({ ...record, is_favorite: true }).revision).toBe(bumped);
  });

  test('archive and unarchive re-read the revision', async () => {
    vi.mocked(archiveContact).mockResolvedValue({ archived: true } as never);
    vi.mocked(unarchiveContact).mockResolvedValue({ archived: false } as never);
    const { result } = setup();
    await act(() => result.current.handleArchiveContact());
    expect(applyUpdates(record).revision).toBe(bumped);
    setRecord.mockClear();
    await act(() => result.current.handleUnarchiveContact());
    expect(applyUpdates(record).revision).toBe(bumped);
  });

  test('the re-read adopts the whole fresh card, not just the revision', async () => {
    vi.mocked(favoriteContact).mockResolvedValue({ is_favorite: true } as never);
    const remoteEdited = {
      ...record,
      revision: bumped,
      is_favorite: true,
      card: { name: { full: 'Edited elsewhere' } },
    } as unknown as ContactRecordResponse;
    vi.mocked(getContactRecord).mockResolvedValue(remoteEdited);
    const { result } = setup();
    await act(() => result.current.handleToggleFavorite());
    expect(applyUpdates(record)).toEqual(remoteEdited);
  });

  test('a failed re-read leaves the held revision alone', async () => {
    vi.mocked(favoriteContact).mockResolvedValue({ is_favorite: true } as never);
    vi.mocked(getContactRecord).mockRejectedValue(new Error('offline'));
    const { result } = setup();
    await act(() => result.current.handleToggleFavorite());
    expect(applyUpdates(record).revision).toBe(record.revision);
    expect(showError).not.toHaveBeenCalled();
  });
});

describe('favorite (#173)', () => {
  test('flips optimistically, then takes the server value', async () => {
    vi.mocked(favoriteContact).mockResolvedValue({ is_favorite: true } as never);
    // The authoritative re-read reflects the flip.
    vi.mocked(getContactRecord).mockResolvedValue({ ...record, revision: 2, is_favorite: true });
    const { result } = setup();
    await act(() => result.current.handleToggleFavorite());
    expect(favoriteContact).toHaveBeenCalledWith('1');
    expect(applyUpdates(record).is_favorite).toBe(true);
  });

  test('unfavorites a favorite', async () => {
    vi.mocked(unfavoriteContact).mockResolvedValue({ is_favorite: false } as never);
    const fav = { ...record, is_favorite: true };
    vi.mocked(getContactRecord).mockResolvedValue({ ...record, revision: 2, is_favorite: false });
    const { result } = setup({ r: fav });
    await act(() => result.current.handleToggleFavorite());
    expect(unfavoriteContact).toHaveBeenCalledWith('1');
    expect(applyUpdates(fav).is_favorite).toBe(false);
  });

  test('rolls back on failure and reports', async () => {
    vi.mocked(favoriteContact).mockRejectedValueOnce(new ApiError('boom', 'X', 500));
    const { result } = setup();
    await act(() => result.current.handleToggleFavorite());
    expect(applyUpdates(record).is_favorite).toBe(false);
    expect(showError).toHaveBeenLastCalledWith('boom');

    vi.mocked(favoriteContact).mockRejectedValueOnce(new Error('x'));
    await act(() => result.current.handleToggleFavorite());
    expect(showError).toHaveBeenLastCalledWith(UPDATE_ERROR);
  });

  test('functional updates leave a cleared record alone', async () => {
    vi.mocked(favoriteContact).mockResolvedValue({ is_favorite: true } as never);
    const { result } = setup();
    await act(() => result.current.handleToggleFavorite());
    for (const [u] of setRecord.mock.calls) {
      expect((u as (p: null) => null)(null)).toBeNull();
    }
  });
});

describe('target closeness (#1797)', () => {
  const targetResponse = {
    contact_id: 1,
    target_closeness: 'close' as const,
    default_interval_days: 14,
    cadence_policy_offer: { entity_id: 'alice-uid', target_interval_days: 14 },
  };

  test('setting a tier updates crm.target_closeness, re-reads the revision, and returns the response', async () => {
    vi.mocked(setTargetCloseness).mockResolvedValueOnce(targetResponse);
    vi.mocked(getContactRecord).mockResolvedValueOnce({
      ...record,
      revision: record.revision + 1,
      crm: { target_closeness: 'close' },
    });
    const { result } = setup();

    let returned: unknown;
    await act(async () => {
      returned = await result.current.handleSetTargetCloseness('close');
    });

    expect(setTargetCloseness).toHaveBeenCalledWith('1', 'close');
    expect(clearTargetCloseness).not.toHaveBeenCalled();
    const updated = applyUpdates(record);
    expect(updated.crm.target_closeness).toBe('close');
    expect(updated.revision).toBe(record.revision + 1);
    expect(getContactRecord).toHaveBeenCalledWith('1');
    expect(returned).toEqual(targetResponse);
  });

  test('clearing sends DELETE and removes the target from the record', async () => {
    vi.mocked(clearTargetCloseness).mockResolvedValueOnce({
      contact_id: 1,
      target_closeness: null,
      default_interval_days: null,
      cadence_policy_offer: null,
    });
    vi.mocked(getContactRecord).mockResolvedValueOnce({
      ...record,
      revision: record.revision + 1,
      crm: {},
    });
    const { result } = setup({ r: { ...record, crm: { target_closeness: 'close' } } });

    let returned: unknown;
    await act(async () => {
      returned = await result.current.handleSetTargetCloseness(null);
    });

    expect(clearTargetCloseness).toHaveBeenCalledWith('1');
    expect(setTargetCloseness).not.toHaveBeenCalled();
    expect(applyUpdates({ ...record, crm: { target_closeness: 'close' } }).crm).toEqual({});
    expect(returned).toMatchObject({ target_closeness: null, cadence_policy_offer: null });
  });

  test('a failure returns null, leaves the record unchanged, and reports the ApiError', async () => {
    vi.mocked(setTargetCloseness).mockRejectedValueOnce(new ApiError('Nope', 'INVALID_INPUT', 400));
    const { result } = setup();

    let returned: unknown;
    await act(async () => {
      returned = await result.current.handleSetTargetCloseness('casual');
    });

    expect(returned).toBeNull();
    expect(setRecord).not.toHaveBeenCalled();
    expect(getContactRecord).not.toHaveBeenCalled();
    expect(showError).toHaveBeenCalledWith('Nope');
  });

  test('a non-ApiError failure reports the generic update error', async () => {
    vi.mocked(setTargetCloseness).mockRejectedValueOnce(new Error('offline'));
    const { result } = setup();

    await act(async () => {
      await result.current.handleSetTargetCloseness('casual');
    });

    expect(showError).toHaveBeenCalledWith(UPDATE_ERROR);
  });

  test('is a no-op without an id', async () => {
    const { result } = setup({ id: undefined });

    let returned: unknown;
    await act(async () => {
      returned = await result.current.handleSetTargetCloseness('close');
    });

    expect(returned).toBeNull();
    expect(setTargetCloseness).not.toHaveBeenCalled();
    expect(clearTargetCloseness).not.toHaveBeenCalled();
  });
});

describe('toggle Me (T90)', () => {
  test('sets this contact as Me, refreshes the cache, and confirms', async () => {
    const { result } = setup();
    await act(() => result.current.handleToggleMe());
    expect(updateSelfContact).toHaveBeenCalledWith('alice-uid');
    expect(setSelfContactUid).toHaveBeenCalledWith('alice-uid');
    expect(fetchAndCacheUserInfo).toHaveBeenCalled();
    expect(showSuccess).toHaveBeenCalledWith('Self contact updated.');
  });

  test('clears Me when this contact already is Me', async () => {
    const { result } = setup({ selfContactUid: 'alice-uid' });
    await act(() => result.current.handleToggleMe());
    expect(updateSelfContact).toHaveBeenCalledWith(null);
    expect(setSelfContactUid).toHaveBeenCalledWith(null);
    expect(showSuccess).toHaveBeenCalledWith('Self contact cleared.');
  });

  test('a failed PATCH changes nothing and reports', async () => {
    vi.mocked(updateSelfContact).mockRejectedValue(new Error('x'));
    const { result } = setup();
    await act(() => result.current.handleToggleMe());
    expect(setSelfContactUid).not.toHaveBeenCalled();
    expect(showError).toHaveBeenCalledWith("Couldn't update your self contact.");
  });
});

// --- behavior pins (issue #1662) ---

describe('failure logging', () => {
  test('each failing action logs its own label with the error', async () => {
    const boom = new Error('x');
    vi.mocked(deleteContact).mockRejectedValue(boom);
    vi.mocked(archiveContact).mockRejectedValue(boom);
    vi.mocked(unarchiveContact).mockRejectedValue(boom);
    vi.mocked(favoriteContact).mockRejectedValue(boom);
    vi.mocked(setTargetCloseness).mockRejectedValue(boom);
    vi.mocked(updateSelfContact).mockRejectedValue(boom);
    const { result } = setup();
    await act(async () => {
      await result.current.handleDeleteContact();
      await result.current.handleArchiveContact();
      await result.current.handleUnarchiveContact();
      await result.current.handleToggleFavorite();
      await result.current.handleSetTargetCloseness('close');
      await result.current.handleToggleMe();
    });
    for (const label of [
      'Error deleting contact:',
      'Error archiving contact:',
      'Error unarchiving contact:',
      'Error toggling favorite:',
      'Error setting target closeness:',
      'Error updating self contact:',
    ]) {
      expect(console.error).toHaveBeenCalledWith(label, boom);
    }
  });

  test('a failed Me toggle shows the settings error and leaves the pointer alone', async () => {
    vi.mocked(updateSelfContact).mockRejectedValue(new Error('x'));
    const { result } = setup();
    await act(() => result.current.handleToggleMe());
    expect(showError).toHaveBeenCalledWith(expect.any(String));
    expect(setSelfContactUid).not.toHaveBeenCalled();
    expect(showSuccess).not.toHaveBeenCalled();
  });

  test('a failed unfavorite rolls a favorite back to favorite', async () => {
    vi.mocked(unfavoriteContact).mockRejectedValue(new Error('x'));
    const fav = { ...record, is_favorite: true };
    const { result } = setup({ r: fav });
    await act(() => result.current.handleToggleFavorite());
    expect(applyUpdates(fav).is_favorite).toBe(true);
  });
});
