import { act, cleanup, renderHook } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, beforeEach, describe, expect, type Mock, test, vi } from 'vitest';
import '../i18n/config';
import { ApiError } from '../api/client';
import { type ContactRecordResponse, updateContactRecord } from '../api/contacts';
import { DateFormatProvider } from '../DateFormatProvider';
import { useContactFieldEditing } from './useContactFieldEditing';

vi.mock('../api/contacts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/contacts')>();
  return { ...actual, updateContactRecord: vi.fn() };
});

const record: ContactRecordResponse = {
  id: 1,
  uid: 'alice-uid',
  etag: '',
  revision: 1,
  gender: 'female',
  card: { organizations: [{ name: 'Acme', units: [{ name: 'Sales' }] }] },
  crm: { how_we_met: 'School' },
};

const UPDATE_ERROR = 'Failed to update contact. Please check your input and try again.';

const wrapper = ({ children }: { children: ReactNode }) => (
  <DateFormatProvider>{children}</DateFormatProvider>
);

let setRecord: Mock<(value: unknown) => void>;
let refreshLifeEvents: ReturnType<typeof vi.fn<(uid: string) => Promise<void>>>;
let showError: Mock<(value: unknown) => void>;

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  localStorage.clear();
  setRecord = vi.fn();
  refreshLifeEvents = vi.fn(async () => {});
  showError = vi.fn();
  vi.mocked(updateContactRecord).mockImplementation(async (_id, input) => ({
    ...record,
    ...input,
  }));
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

function setup(r: ContactRecordResponse | null = record, id: string | undefined = '1') {
  return renderHook(
    () => useContactFieldEditing({ id, record: r, setRecord, refreshLifeEvents, showError }),
    { wrapper },
  );
}

test('start seeds the editor; a date field is converted to the display format', () => {
  const { result } = setup();
  act(() => result.current.handleEditStart('how_we_met', 'School'));
  expect(result.current.editingField).toBe('how_we_met');
  expect(result.current.editValue).toBe('School');

  act(() => result.current.handleEditStart('birthday', '1990-04-30'));
  expect(result.current.editValue).toBe('30.04.1990');

  act(() => result.current.handleEditStart('anniversary', ''));
  expect(result.current.editValue).toBe('');
});

test('value changes auto-format only for date fields and clear the validation error', () => {
  const { result } = setup();
  act(() => result.current.handleEditStart('how_we_met', ''));
  act(() => result.current.handleEditValueChange('30041990'));
  expect(result.current.editValue).toBe('30041990');

  act(() => result.current.handleEditStart('birthday', ''));
  act(() => result.current.handleEditValueChange('30041990'));
  expect(result.current.editValue).toBe('30.04.1990');
});

test('cancel resets the editor', () => {
  const { result } = setup();
  act(() => result.current.handleEditStart('how_we_met', 'School'));
  act(() => result.current.handleEditCancel());
  expect(result.current.editingField).toBeNull();
  expect(result.current.editValue).toBe('');
});

test('saving a CRM field PUTs the merged record, closes the editor, refreshes life events', async () => {
  const { result } = setup();
  act(() => result.current.handleEditStart('how_we_met', 'School'));
  act(() => result.current.handleEditValueChange('Work'));
  await act(() => result.current.handleEditSave('how_we_met'));
  expect(updateContactRecord).toHaveBeenCalledWith(
    '1',
    {
      gender: 'female',
      card: record.card,
      crm: { how_we_met: 'Work' },
    },
    1,
  );
  expect(setRecord).toHaveBeenCalled();
  expect(result.current.editingField).toBeNull();
  expect(refreshLifeEvents).toHaveBeenCalledWith('alice-uid');
});

test('a valid date is saved as ISO; a blank date clears it', async () => {
  const { result } = setup();
  act(() => result.current.handleEditStart('birthday', ''));
  act(() => result.current.handleEditValueChange('30.04.1990'));
  await act(() => result.current.handleEditSave('birthday'));
  expect(vi.mocked(updateContactRecord).mock.calls[0][1].card.anniversaries).toEqual([
    { kind: 'birth', date: { partial: { year: 1990, month: 4, day: 30 } } },
  ]);

  act(() => result.current.handleEditStart('birthday', ''));
  await act(() => result.current.handleEditSave('birthday'));
  expect(vi.mocked(updateContactRecord).mock.calls[1][1].card.anniversaries).toEqual([]);
});

test('an invalid date is rejected before any request', async () => {
  const { result } = setup();
  act(() => result.current.handleEditStart('anniversary', ''));
  act(() => result.current.handleEditValueChange('01.13.1990'));
  await act(() => result.current.handleEditSave('anniversary'));
  expect(result.current.validationError).toBe(
    'Invalid date format. Check your date format setting.',
  );
  expect(updateContactRecord).not.toHaveBeenCalled();
});

test('a failing life-event refresh after a save is swallowed', async () => {
  refreshLifeEvents.mockRejectedValue(new Error('x'));
  const { result } = setup();
  act(() => result.current.handleEditStart('gender', ''));
  await act(() => result.current.handleEditSave('gender'));
  expect(showError).not.toHaveBeenCalled();
});

test('an ApiError on save is shown inline and as a toast; other errors get the generic toast', async () => {
  vi.mocked(updateContactRecord).mockRejectedValueOnce(
    new ApiError('Bad value', 'VALIDATION', 400),
  );
  const { result } = setup();
  act(() => result.current.handleEditStart('how_we_met', 'x'));
  await act(() => result.current.handleEditSave('how_we_met'));
  expect(result.current.validationError).toBe('Bad value');
  expect(showError).toHaveBeenLastCalledWith('Bad value');
  expect(result.current.editingField).toBe('how_we_met');

  vi.mocked(updateContactRecord).mockRejectedValueOnce(new Error('network'));
  act(() => result.current.handleEditStart('how_we_met', 'x'));
  await act(() => result.current.handleEditSave('how_we_met'));
  expect(result.current.validationError).toBe('');
  expect(showError).toHaveBeenLastCalledWith(UPDATE_ERROR);
});

test('save and card updates are no-ops without a record', async () => {
  const { result } = setup(null);
  await act(() => result.current.handleEditSave('gender'));
  await act(() => result.current.handleUpdateCard({ emails: [] }));
  expect(updateContactRecord).not.toHaveBeenCalled();
});

test('handleUpdateCard merges the card and CRM patches and refreshes life events', async () => {
  const { result } = setup();
  await act(() =>
    result.current.handleUpdateCard({ emails: [{ address: 'a@example.com' }] }, { kind: 'human' }),
  );
  expect(updateContactRecord).toHaveBeenCalledWith(
    '1',
    {
      gender: 'female',
      card: { ...record.card, emails: [{ address: 'a@example.com' }] },
      crm: { how_we_met: 'School', kind: 'human' },
    },
    1,
  );
  expect(refreshLifeEvents).toHaveBeenCalledWith('alice-uid');
});

test('handleUpdateCard reports and rethrows a failure', async () => {
  const { result } = setup();
  vi.mocked(updateContactRecord).mockRejectedValueOnce(new ApiError('Conflict', 'CONFLICT', 409));
  await act(async () => {
    await expect(result.current.handleUpdateCard({})).rejects.toThrow('Conflict');
  });
  expect(showError).toHaveBeenLastCalledWith('Conflict');

  vi.mocked(updateContactRecord).mockRejectedValueOnce(new Error('network'));
  await act(async () => {
    await expect(result.current.handleUpdateCard({})).rejects.toThrow('network');
  });
  expect(showError).toHaveBeenLastCalledWith(UPDATE_ERROR);
  expect(setRecord).not.toHaveBeenCalled();
});

function setupWithConflict(onConflict: () => void) {
  return renderHook(
    () =>
      useContactFieldEditing({
        id: '1',
        record,
        setRecord,
        refreshLifeEvents,
        showError,
        onConflict,
      }),
    { wrapper },
  );
}

test('a 412 on an inline save calls onConflict, with no toast or inline error', async () => {
  const onConflict = vi.fn();
  const { result } = setupWithConflict(onConflict);
  act(() => result.current.handleEditStart('how_we_met', 'School'));
  vi.mocked(updateContactRecord).mockRejectedValueOnce(
    new ApiError('stale', 'PRECONDITION_FAILED', 412),
  );
  await act(() => result.current.handleEditSave('how_we_met'));
  expect(onConflict).toHaveBeenCalledTimes(1);
  expect(showError).not.toHaveBeenCalled();
  expect(result.current.validationError).toBe('');
  expect(result.current.editingField).toBe('how_we_met');
});

test('a 412 on a structured save calls onConflict and still rethrows', async () => {
  const onConflict = vi.fn();
  const { result } = setupWithConflict(onConflict);
  vi.mocked(updateContactRecord).mockRejectedValueOnce(
    new ApiError('stale', 'PRECONDITION_FAILED', 412),
  );
  await act(async () => {
    await expect(result.current.handleUpdateCard({})).rejects.toThrow('stale');
  });
  expect(onConflict).toHaveBeenCalledTimes(1);
  expect(showError).not.toHaveBeenCalled();
});

// --- behavior pins (issue #1662) ---

test('starts with no field being edited and nothing entered', () => {
  const { result } = setup();
  expect(result.current.editingField).toBeNull();
  expect(result.current.editValue).toBe('');
  expect(result.current.validationError).toBe('');
});

test('starting an edit seeds an empty string for a missing value and clears an old validation error', async () => {
  const { result } = setup();
  act(() => result.current.handleEditStart('birthday', ''));
  act(() => result.current.handleEditValueChange('99.99.9999'));
  await act(() => result.current.handleEditSave('birthday'));
  expect(result.current.validationError).not.toBe('');

  act(() => result.current.handleEditStart('how_we_met', undefined as unknown as string));
  expect(result.current.editValue).toBe('');
  expect(result.current.validationError).toBe('');
});

test('cancel and value changes both clear a validation error', async () => {
  const { result } = setup();
  act(() => result.current.handleEditStart('birthday', ''));
  act(() => result.current.handleEditValueChange('99.99.9999'));
  await act(() => result.current.handleEditSave('birthday'));
  expect(result.current.validationError).not.toBe('');
  act(() => result.current.handleEditValueChange('30.04.1990'));
  expect(result.current.validationError).toBe('');

  act(() => result.current.handleEditValueChange('99.99.9999'));
  await act(() => result.current.handleEditSave('birthday'));
  expect(result.current.validationError).not.toBe('');
  act(() => result.current.handleEditCancel());
  expect(result.current.validationError).toBe('');
  expect(result.current.editingField).toBeNull();
  expect(result.current.editValue).toBe('');
});

test('a successful save after an error clears the field, value, and validation error', async () => {
  const { result } = setup();
  act(() => result.current.handleEditStart('how_we_met', 'School'));
  vi.mocked(updateContactRecord).mockRejectedValueOnce(
    new ApiError('bad input', 'VALIDATION', 400),
  );
  await act(() => result.current.handleEditSave('how_we_met'));
  expect(result.current.validationError).not.toBe('');
  expect(console.error).toHaveBeenCalledWith('Error updating contact:', expect.anything());

  await act(() => result.current.handleEditSave('how_we_met'));
  expect(result.current.editingField).toBeNull();
  expect(result.current.editValue).toBe('');
  expect(result.current.validationError).toBe('');
});

test('a non-date field is saved verbatim without date parsing', async () => {
  const { result } = setup();
  act(() => result.current.handleEditStart('how_we_met', 'not a date 99.99'));
  await act(() => result.current.handleEditSave('how_we_met'));
  expect(updateContactRecord).toHaveBeenCalledWith(
    '1',
    expect.objectContaining({ crm: { how_we_met: 'not a date 99.99' } }),
    1,
  );
});

test('a whitespace-only date is treated as blank (clears), not as invalid', async () => {
  const { result } = setup();
  act(() => result.current.handleEditStart('birthday', ''));
  act(() => result.current.handleEditValueChange('   '));
  await act(() => result.current.handleEditSave('birthday'));
  expect(result.current.validationError).toBe('');
  expect(updateContactRecord).toHaveBeenCalledTimes(1);
});

describe('conflict routing and guards', () => {
  test('a non-412 failure with onConflict set still shows the normal error', async () => {
    const onConflict = vi.fn();
    const { result } = setupWithConflict(onConflict);
    act(() => result.current.handleEditStart('how_we_met', 'School'));
    vi.mocked(updateContactRecord).mockRejectedValueOnce(
      new ApiError('Bad value', 'VALIDATION', 400),
    );
    await act(() => result.current.handleEditSave('how_we_met'));
    expect(onConflict).not.toHaveBeenCalled();
    expect(showError).toHaveBeenLastCalledWith('Bad value');
    expect(result.current.validationError).toBe('Bad value');

    vi.mocked(updateContactRecord).mockRejectedValueOnce(
      new ApiError('Bad value', 'VALIDATION', 400),
    );
    await act(async () => {
      await expect(result.current.handleUpdateCard({})).rejects.toThrow('Bad value');
    });
    expect(onConflict).not.toHaveBeenCalled();
  });

  test('a 412 without an onConflict handler is shown like any other API error', async () => {
    const { result } = setup();
    act(() => result.current.handleEditStart('how_we_met', 'School'));
    vi.mocked(updateContactRecord).mockRejectedValueOnce(
      new ApiError('stale', 'PRECONDITION_FAILED', 412),
    );
    await act(() => result.current.handleEditSave('how_we_met'));
    expect(showError).toHaveBeenLastCalledWith('stale');
    expect(result.current.validationError).toBe('stale');
  });

  test('a non-date field value is seeded verbatim even when it looks like an ISO date', () => {
    const { result } = setup();
    act(() => result.current.handleEditStart('how_we_met', '1990-04-30'));
    expect(result.current.editValue).toBe('1990-04-30');
  });

  test('saving needs both a record and an id', async () => {
    const { result } = renderHook(
      () =>
        useContactFieldEditing({
          id: undefined,
          record,
          setRecord,
          refreshLifeEvents,
          showError,
        }),
      { wrapper },
    );
    await act(async () => {
      await result.current.handleEditSave('how_we_met');
      await result.current.handleUpdateCard({});
    });
    expect(updateContactRecord).not.toHaveBeenCalled();
  });

  test('a structured save stores the server record and refreshes life events', async () => {
    const updated = { ...record, revision: 9 };
    vi.mocked(updateContactRecord).mockResolvedValueOnce(updated);
    const { result } = setup();
    await act(() => result.current.handleUpdateCard({}));
    expect(setRecord).toHaveBeenCalledWith(updated);
    expect(refreshLifeEvents).toHaveBeenCalledWith('alice-uid');
  });
});
