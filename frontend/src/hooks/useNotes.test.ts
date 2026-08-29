import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { getContactNotes, getUnassignedNotes, type Note, type NotesResponse } from '../api/notes';
import { useNotes } from './useNotes';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  localStorage.clear();
  vi.restoreAllMocks();
});

vi.mock('../api/notes', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/notes')>();
  return { ...actual, getContactNotes: vi.fn(), getUnassignedNotes: vi.fn() };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(getContactNotes).mockReset();
  vi.mocked(getUnassignedNotes).mockReset();
});

const note: Note = {
  ID: 1,
  content: 'call mom',
  date: '2026-01-01',
  CreatedAt: '2026-01-01T00:00:00Z',
  UpdatedAt: '2026-01-01T00:00:00Z',
};

function unassignedResponse(overrides: Partial<NotesResponse> = {}): NotesResponse {
  return { notes: [note], next_cursor: '', limit: 25, total: 1, ...overrides };
}

test('loads contact notes on mount', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  vi.mocked(getContactNotes).mockResolvedValue({ notes: [note] });

  const { result } = renderHook(() => useNotes(42));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getContactNotes).toHaveBeenCalledWith(42);
  expect(result.current.notes).toEqual([note]);
  expect(result.current.nextCursor).toBe('');
  expect(result.current.total).toBe(1);
  expect(result.current.limit).toBe(1);
  expect(result.current.error).toBeNull();
});

test('loads unassigned notes with the requested params', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  vi.mocked(getUnassignedNotes).mockResolvedValue(
    unassignedResponse({ next_cursor: 'abc', total: 7 }),
  );

  const { result } = renderHook(() => useNotes(undefined, { limit: 50, search: 'mom' }));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getUnassignedNotes).toHaveBeenCalledWith({
    limit: 50,
    search: 'mom',
    fromDate: undefined,
    toDate: undefined,
  });
  expect(result.current.notes).toEqual([note]);
  expect(result.current.nextCursor).toBe('abc');
  expect(result.current.total).toBe(7);
  expect(result.current.limit).toBe(25);
});

test('normalizes a bare array response for contact notes', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  vi.mocked(getContactNotes).mockResolvedValue([note] as unknown as NotesResponse);

  const { result } = renderHook(() => useNotes('a-uid'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.notes).toEqual([note]);
});

test('treats an object contact-notes response without a notes key as empty', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  vi.mocked(getContactNotes).mockResolvedValue({} as NotesResponse);

  const { result } = renderHook(() => useNotes(3));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.notes).toEqual([]);
  expect(result.current.total).toBe(0);
  expect(result.current.limit).toBe(25);
});

test('falls back to defaults when an unassigned response omits every optional key', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  vi.mocked(getUnassignedNotes).mockResolvedValue({} as NotesResponse);

  const { result } = renderHook(() => useNotes());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.notes).toEqual([]);
  expect(result.current.nextCursor).toBe('');
  expect(result.current.total).toBe(0);
  expect(result.current.limit).toBe(25);
});

test('falls back to empty lists and the param limit when the response has no notes', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  vi.mocked(getContactNotes).mockResolvedValue({ notes: [] });

  const { result } = renderHook(() => useNotes(7, { limit: 10 }));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.notes).toEqual([]);
  expect(result.current.total).toBe(0);
  expect(result.current.limit).toBe(10);
});

test('loadMore appends the next page and refreshes the cursor and total', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  vi.mocked(getUnassignedNotes)
    .mockResolvedValueOnce(unassignedResponse({ next_cursor: 'abc', total: 3 }))
    .mockResolvedValueOnce(
      unassignedResponse({ notes: [{ ...note, ID: 2 }], next_cursor: '', total: 3 }),
    );

  const { result } = renderHook(() => useNotes());
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.notes).toHaveLength(1);

  await act(async () => {
    await result.current.loadMore();
  });

  expect(getUnassignedNotes).toHaveBeenLastCalledWith({
    cursor: 'abc',
    limit: undefined,
    search: undefined,
    fromDate: undefined,
    toDate: undefined,
  });
  expect(result.current.notes).toHaveLength(2);
  expect(result.current.nextCursor).toBe('');
  expect(result.current.total).toBe(3);
});

test('loadMore tolerates a page that omits notes and next_cursor', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  vi.mocked(getUnassignedNotes)
    .mockResolvedValueOnce(unassignedResponse({ next_cursor: 'abc' }))
    .mockResolvedValueOnce({} as NotesResponse);

  const { result } = renderHook(() => useNotes());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.loadMore();
  });

  expect(result.current.notes).toHaveLength(1);
  expect(result.current.nextCursor).toBe('');
  expect(result.current.total).toBe(1);
});

test('loadMore is a no-op when there is no next cursor', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  vi.mocked(getUnassignedNotes).mockResolvedValue(unassignedResponse({ next_cursor: '' }));

  const { result } = renderHook(() => useNotes());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.loadMore();
  });

  expect(getUnassignedNotes).toHaveBeenCalledTimes(1);
  expect(result.current.notes).toHaveLength(1);
});

test('sets an auth error when not authenticated', async () => {
  localStorage.clear();

  const { result } = renderHook(() => useNotes());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.error).toBe('No authentication token found');
  expect(result.current.notes).toEqual([]);
  expect(getUnassignedNotes).not.toHaveBeenCalled();
});

test('sets an error when the first fetch fails', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  vi.mocked(getUnassignedNotes).mockRejectedValue(new Error('boom'));

  const { result } = renderHook(() => useNotes());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.error).toBe('boom');
  expect(result.current.notes).toEqual([]);
});

test('sets an error when loadMore fails', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  vi.mocked(getUnassignedNotes)
    .mockResolvedValueOnce(unassignedResponse({ next_cursor: 'abc' }))
    .mockRejectedValueOnce(new Error('paging failed'));

  const { result } = renderHook(() => useNotes());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.loadMore();
  });

  expect(result.current.error).toBe('paging failed');
  expect(result.current.notes).toHaveLength(1);
});

test('refetch re-runs the first-page load', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  vi.mocked(getUnassignedNotes).mockResolvedValue(unassignedResponse());

  const { result } = renderHook(() => useNotes());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.refetch();
  });

  expect(getUnassignedNotes).toHaveBeenCalledTimes(2);
  expect(result.current.error).toBeNull();
});
