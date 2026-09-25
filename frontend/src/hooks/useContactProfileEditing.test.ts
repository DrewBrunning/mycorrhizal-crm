import { act, cleanup, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, expect, type Mock, test, vi } from 'vitest';
import '../i18n/config';
import { ApiError } from '../api/client';
import { type ContactRecordResponse, updateContactRecord } from '../api/contacts';
import { EMPTY_PROFILE_VALUES } from '../utils/contactDetailPayloads';
import { useContactProfileEditing } from './useContactProfileEditing';

vi.mock('../api/contacts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/contacts')>();
  return { ...actual, updateContactRecord: vi.fn() };
});

const record: ContactRecordResponse = {
  id: 1,
  uid: 'alice-uid',
  etag: '',
  revision: 1,
  card: {
    name: {
      components: [
        { kind: 'given', value: 'Alice' },
        { kind: 'surname', value: 'Wonder' },
      ],
    },
  },
  crm: { kind: 'human' },
};

let setRecord: Mock<(value: unknown) => void>;
let showError: Mock<(value: unknown) => void>;
let alertSpy: ReturnType<typeof vi.spyOn>;

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  alertSpy = vi.spyOn(window, 'alert').mockImplementation(() => {});
  setRecord = vi.fn();
  showError = vi.fn();
  vi.mocked(updateContactRecord).mockImplementation(async () => record);
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

const setup = (r: ContactRecordResponse | null = record, ...rest: [] | [undefined]) =>
  renderHook(() =>
    useContactProfileEditing({
      id: rest.length ? undefined : '1',
      record: r,
      setRecord,
      showError,
    }),
  );

test('start seeds the form from the record; cancel resets it', () => {
  const { result } = setup();
  act(() => result.current.handleStartEditProfile());
  expect(result.current.editingProfile).toBe(true);
  expect(result.current.profileValues.firstname).toBe('Alice');
  expect(result.current.profileValues.lastname).toBe('Wonder');

  act(() => result.current.handleCancelEditProfile());
  expect(result.current.editingProfile).toBe(false);
  expect(result.current.profileValues).toEqual(EMPTY_PROFILE_VALUES);
});

test('start is a no-op without a record', () => {
  const { result } = setup(null);
  act(() => result.current.handleStartEditProfile());
  expect(result.current.editingProfile).toBe(false);
});

test('a blank first name alerts and never saves', async () => {
  const { result } = setup();
  act(() => result.current.handleStartEditProfile());
  act(() => result.current.setProfileValues({ ...result.current.profileValues, firstname: ' ' }));
  await act(() => result.current.handleSaveProfile());
  expect(alertSpy).toHaveBeenCalledWith('First name is required');
  expect(updateContactRecord).not.toHaveBeenCalled();
});

test('a save with no record alerts too', async () => {
  const { result } = setup(null);
  await act(() => result.current.handleSaveProfile());
  expect(alertSpy).toHaveBeenCalled();
});

test('a save without an id is a no-op', async () => {
  const { result } = setup(record, undefined);
  act(() => result.current.handleStartEditProfile());
  await act(() => result.current.handleSaveProfile());
  expect(updateContactRecord).not.toHaveBeenCalled();
});

test('a successful save PUTs the built name and leaves edit mode', async () => {
  const { result } = setup();
  act(() => result.current.handleStartEditProfile());
  act(() =>
    result.current.setProfileValues({ ...result.current.profileValues, firstname: 'Alicia' }),
  );
  await act(() => result.current.handleSaveProfile());
  expect(vi.mocked(updateContactRecord).mock.calls[0][0]).toBe('1');
  expect(vi.mocked(updateContactRecord).mock.calls[0][1].card.name?.components?.[0]).toMatchObject({
    kind: 'given',
    value: 'Alicia',
  });
  expect(setRecord).toHaveBeenCalledWith(record);
  expect(result.current.editingProfile).toBe(false);
});

test('a failed save stays in edit mode and reports the error', async () => {
  const { result } = setup();
  act(() => result.current.handleStartEditProfile());
  vi.mocked(updateContactRecord).mockRejectedValueOnce(new ApiError('Nope', 'X', 400));
  await act(() => result.current.handleSaveProfile());
  expect(showError).toHaveBeenLastCalledWith('Nope');
  expect(result.current.editingProfile).toBe(true);

  vi.mocked(updateContactRecord).mockRejectedValueOnce(new Error('network'));
  await act(() => result.current.handleSaveProfile());
  expect(showError).toHaveBeenLastCalledWith(
    'Failed to update contact. Please check your input and try again.',
  );
});
