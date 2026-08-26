import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { type Activity, getActivities, getContactActivities } from '../api/activities';
import { useActivities } from './useActivities';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  localStorage.clear();
  vi.restoreAllMocks();
});

vi.mock('../api/activities', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/activities')>();
  return { ...actual, getActivities: vi.fn(), getContactActivities: vi.fn() };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(getActivities).mockReset();
  vi.mocked(getContactActivities).mockReset();
});

function activity(id: number, title: string): Activity {
  return {
    ID: id,
    title,
    date: '2026-08-12T10:00:00Z',
    CreatedAt: '2026-08-12T10:00:00Z',
    UpdatedAt: '2026-08-12T10:00:00Z',
  };
}

test('initial fetch populates activities and the next cursor', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  vi.mocked(getActivities).mockResolvedValue({
    activities: [activity(1, 'call with alice'), activity(2, 'coffee with bob')],
    next_cursor: 'cursor-1',
    limit: 25,
  });

  const { result } = renderHook(() => useActivities());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getActivities).toHaveBeenCalledWith({
    limit: undefined,
    includeContacts: undefined,
    search: undefined,
    fromDate: undefined,
    toDate: undefined,
  });
  expect(result.current.activities.map((a) => a.ID)).toEqual([1, 2]);
  expect(result.current.nextCursor).toBe('cursor-1');
  expect(result.current.limit).toBe(25);
  expect(result.current.error).toBeNull();
});

test('loadMore appends rows and follows the next cursor', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  vi.mocked(getActivities)
    .mockResolvedValueOnce({
      activities: [activity(1, 'page one')],
      next_cursor: 'cursor-1',
      limit: 25,
    })
    .mockResolvedValueOnce({
      activities: [activity(2, 'page two')],
      next_cursor: '',
      limit: 25,
    });

  const { result } = renderHook(() => useActivities());
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.activities).toHaveLength(1);

  await act(async () => {
    await result.current.loadMore();
  });

  expect(getActivities).toHaveBeenLastCalledWith({
    cursor: 'cursor-1',
    limit: undefined,
    includeContacts: undefined,
    search: undefined,
    fromDate: undefined,
    toDate: undefined,
  });
  expect(result.current.activities.map((a) => a.ID)).toEqual([1, 2]);
  expect(result.current.nextCursor).toBe('');
});

test('loadMore is a no-op when there is no next cursor', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  vi.mocked(getActivities).mockResolvedValue({
    activities: [activity(1, 'only page')],
    next_cursor: '',
    limit: 25,
  });

  const { result } = renderHook(() => useActivities());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.loadMore();
  });

  expect(getActivities).toHaveBeenCalledTimes(1);
});

test('refetch replaces the list', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  vi.mocked(getActivities)
    .mockResolvedValueOnce({ activities: [activity(1, 'one')], next_cursor: '', limit: 25 })
    .mockResolvedValueOnce({ activities: [activity(2, 'two')], next_cursor: '', limit: 25 });

  const { result } = renderHook(() => useActivities());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.refetch();
  });

  expect(getActivities).toHaveBeenCalledTimes(2);
  expect(result.current.activities.map((a) => a.ID)).toEqual([2]);
});

test('uses getContactActivities when a contactId is given', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  vi.mocked(getContactActivities).mockResolvedValue({ activities: [activity(1, 'contact one')] });

  const { result } = renderHook(() => useActivities({}, 'contact-7'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getContactActivities).toHaveBeenCalledWith('contact-7');
  expect(getActivities).not.toHaveBeenCalled();
  expect(result.current.activities.map((a) => a.ID)).toEqual([1]);
  expect(result.current.nextCursor).toBe('');
});

test('sets an auth error when no token exists', async () => {
  localStorage.clear();

  const { result } = renderHook(() => useActivities());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.error).toBe('No authentication token found');
  expect(result.current.activities).toEqual([]);
  expect(getActivities).not.toHaveBeenCalled();
});

test('sets error when the fetch fails', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  vi.mocked(getActivities).mockRejectedValue(new Error('boom'));

  const { result } = renderHook(() => useActivities());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.error).toBe('boom');
  expect(result.current.activities).toEqual([]);
});

test('sets error when loading more fails', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  vi.mocked(getActivities)
    .mockResolvedValueOnce({ activities: [activity(1, 'one')], next_cursor: 'cursor-1', limit: 25 })
    .mockRejectedValueOnce(new Error('boom'));

  const { result } = renderHook(() => useActivities());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.loadMore();
  });

  expect(result.current.error).toBe('boom');
  expect(result.current.activities).toHaveLength(1);
});
