import { act, cleanup, renderHook } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, beforeEach, expect, type Mock, test, vi } from 'vitest';
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
  expect(updateContactRecord).toHaveBeenCalledWith('1', {
    gender: 'female',
    card: record.card,
    crm: { how_we_met: 'Work' },
  });
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
  expect(updateContactRecord).toHaveBeenCalledWith('1', {
    gender: 'female',
    card: { ...record.card, emails: [{ address: 'a@example.com' }] },
    crm: { how_we_met: 'School', kind: 'human' },
  });
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
