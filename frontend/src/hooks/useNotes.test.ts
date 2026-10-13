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

// --- behavior pins (issue #1662) ---

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

function signIn() {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
}

test('starts empty with the requested page size before the first response lands', () => {
  signIn();
  vi.mocked(getUnassignedNotes).mockReturnValue(new Promise(() => {}));
  const plain = renderHook(() => useNotes());
  expect(plain.result.current.notes).toEqual([]);
  expect(plain.result.current.nextCursor).toBe('');
  expect(plain.result.current.total).toBe(0);
  expect(plain.result.current.limit).toBe(25);
  expect(plain.result.current.error).toBeNull();
  const sized = renderHook(() => useNotes(undefined, { limit: 7 }));
  expect(sized.result.current.limit).toBe(7);
});

test('a contact-scoped list sizes the page from its rows and reports their count as the total', async () => {
  signIn();
  vi.mocked(getContactNotes).mockResolvedValue({ notes: [note, { ...note, ID: 2 }] });
  const { result } = renderHook(() => useNotes(42, { limit: 9 }));
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.limit).toBe(2);
  expect(result.current.total).toBe(2);
  expect(result.current.nextCursor).toBe('');

  vi.mocked(getContactNotes).mockResolvedValue({ notes: [] });
  const empty = renderHook(() => useNotes(42, { limit: 9 }));
  await waitFor(() => expect(empty.result.current.loading).toBe(false));
  expect(empty.result.current.limit).toBe(9);

  const bare = renderHook(() => useNotes(42));
  await waitFor(() => expect(bare.result.current.loading).toBe(false));
  expect(bare.result.current.limit).toBe(25);
});

test('the unassigned list prefers the server limit, and keeps a total of zero', async () => {
  signIn();
  vi.mocked(getUnassignedNotes).mockResolvedValue(
    unassignedResponse({ notes: [], total: 0, limit: 40 }),
  );
  const { result } = renderHook(() => useNotes(undefined, { limit: 10 }));
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.limit).toBe(40);
  expect(result.current.total).toBe(0);
});

test('reports loading while in flight, and a refetch clears an earlier error', async () => {
  signIn();
  vi.mocked(getUnassignedNotes).mockRejectedValueOnce(new Error('boom'));
  const { result } = renderHook(() => useNotes());
  await waitFor(() => expect(result.current.error).toBe('boom'));
  expect(console.error).toHaveBeenCalledWith('[fetching notes] Error:', expect.anything());

  const d = deferred<NotesResponse>();
  vi.mocked(getUnassignedNotes).mockReturnValueOnce(d.promise);
  act(() => {
    void result.current.refetch();
  });
  await waitFor(() => expect(result.current.loading).toBe(true));
  expect(result.current.error).toBeNull();
  await act(async () => {
    d.resolve(unassignedResponse());
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
});

test('loadMore reports loading, keeps the total when the page omits it, and logs failures by operation', async () => {
  signIn();
  vi.mocked(getUnassignedNotes).mockResolvedValueOnce(
    unassignedResponse({ next_cursor: 'c1', total: 5 }),
  );
  const { result } = renderHook(() => useNotes());
  await waitFor(() => expect(result.current.nextCursor).toBe('c1'));

  const d = deferred<NotesResponse>();
  vi.mocked(getUnassignedNotes).mockReturnValueOnce(d.promise);
  act(() => {
    void result.current.loadMore();
  });
  await waitFor(() => expect(result.current.loading).toBe(true));
  await act(async () => {
    d.resolve({ notes: [{ ...note, ID: 2 }], next_cursor: 'c2' } as NotesResponse);
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.total).toBe(5);
  expect(result.current.nextCursor).toBe('c2');

  vi.mocked(getUnassignedNotes).mockRejectedValueOnce(new Error('later'));
  await act(async () => {
    await result.current.loadMore();
  });
  expect(result.current.error).toBe('later');
  expect(console.error).toHaveBeenCalledWith('[loading more notes] Error:', expect.anything());

  vi.mocked(getUnassignedNotes).mockResolvedValueOnce(
    unassignedResponse({ next_cursor: '', total: 0 }),
  );
  await act(async () => {
    await result.current.loadMore();
  });
  expect(result.current.error).toBeNull();
  expect(result.current.total).toBe(0);
});
