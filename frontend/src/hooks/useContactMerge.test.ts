import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import {
  type ContactMergePreviewResponse,
  commitContactMerge,
  previewContactMerge,
} from '../api/contactMerge';
import { useContactMerge } from './useContactMerge';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock('../api/contactMerge', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/contactMerge')>();
  return {
    ...actual,
    previewContactMerge: vi.fn(),
    commitContactMerge: vi.fn(),
  };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(previewContactMerge).mockReset();
  vi.mocked(commitContactMerge).mockReset();
});

// A realistically-populated preview: every resolution collection present.
function fullPreview(overrides: Partial<ContactMergePreviewResponse['resolution']> = {}) {
  return {
    keep_id: 1,
    merge_id: 2,
    resolution: {
      emails: [],
      phones: [],
      addresses: [],
      urls: [],
      impps: [],
      resolved_scalars: {},
      conflicts: [],
      field_value_conflicts: [],
      ...overrides,
    },
    association_counts: {},
  } as unknown as ContactMergePreviewResponse;
}

test('loadPreview normalizes bare-null collections the backend sends for empty Go slices', async () => {
  // The backend sends `null` (not `[]`) for an empty slice, and `null` for an
  // empty map — the hook normalizes each to [] / {} exactly once so render
  // sites never have to guard. Returning every field as null pins each `|| []`
  // and `|| {}` individually.
  vi.mocked(previewContactMerge).mockResolvedValue({
    keep_id: 7,
    merge_id: 8,
    resolution: {
      emails: null,
      phones: null,
      addresses: null,
      urls: null,
      impps: null,
      resolved_scalars: null,
      conflicts: null,
      field_value_conflicts: null,
    },
    association_counts: {},
  } as unknown as ContactMergePreviewResponse);

  const { result } = renderHook(() => useContactMerge());
  await act(async () => {
    await result.current.loadPreview(7, 8);
  });

  expect(result.current.preview?.keep_id).toBe(7);
  const resolution = result.current.preview?.resolution;
  expect(resolution?.emails).toEqual([]);
  expect(resolution?.phones).toEqual([]);
  expect(resolution?.addresses).toEqual([]);
  expect(resolution?.urls).toEqual([]);
  expect(resolution?.impps).toEqual([]);
  expect(resolution?.resolved_scalars).toEqual({});
  expect(resolution?.conflicts).toEqual([]);
  expect(resolution?.field_value_conflicts).toEqual([]);
  expect(result.current.error).toBeNull();
  expect(result.current.loading).toBe(false);
});

test('loadPreview does not clobber a populated resolution', async () => {
  const conflict = { field: 'name', label: 'Name', keeper_value: 'a', loser_value: 'b' };
  vi.mocked(previewContactMerge).mockResolvedValue(
    fullPreview({
      emails: [{ type: 'email', value: 'a@example.com' }],
      resolved_scalars: { name: 'a' },
      conflicts: [conflict],
    }),
  );

  const { result } = renderHook(() => useContactMerge());
  await act(async () => {
    await result.current.loadPreview(1, 2);
  });

  expect(result.current.preview?.resolution.emails).toHaveLength(1);
  expect(result.current.preview?.resolution.resolved_scalars).toEqual({ name: 'a' });
  expect(result.current.preview?.resolution.conflicts).toEqual([conflict]);
});

test('loadPreview surfaces the fetch error and clears the preview', async () => {
  vi.mocked(previewContactMerge).mockRejectedValue(new Error('boom'));

  const { result } = renderHook(() => useContactMerge());
  await act(async () => {
    await result.current.loadPreview(1, 2);
  });

  expect(result.current.error).toBe('boom');
  expect(result.current.preview).toBeNull();
  expect(result.current.loading).toBe(false);
});

test('loadPreview starts a fresh preview with no carried-over resolutions', async () => {
  vi.mocked(previewContactMerge).mockResolvedValue(fullPreview());

  const { result } = renderHook(() => useContactMerge());
  await act(async () => {
    result.current.setResolution('name', 'kept');
  });
  expect(result.current.resolutions).toEqual({ name: 'kept' });

  await act(async () => {
    await result.current.loadPreview(1, 2);
  });

  expect(result.current.resolutions).toEqual({});
});

test('allConflictsResolved is false before any preview is loaded', () => {
  const { result } = renderHook(() => useContactMerge());
  expect(result.current.allConflictsResolved).toBe(false);
});

test('allConflictsResolved requires every scalar AND field-value conflict to be chosen', async () => {
  vi.mocked(previewContactMerge).mockResolvedValue(
    fullPreview({
      conflicts: [{ field: 'name', label: 'Name', keeper_value: 'a', loser_value: 'b' }],
      field_value_conflicts: [
        { field: 'phones', label: 'Phones', keeper_value: 'a', loser_value: 'b' },
      ],
    }),
  );

  const { result } = renderHook(() => useContactMerge());
  await act(async () => {
    await result.current.loadPreview(1, 2);
  });

  // A preview with unresolved conflicts must keep the commit disabled.
  expect(result.current.allConflictsResolved).toBe(false);

  await act(async () => {
    result.current.setResolution('name', 'a');
  });
  // The scalar is resolved, but the field-value conflict still is not.
  expect(result.current.allConflictsResolved).toBe(false);

  await act(async () => {
    result.current.setResolution('phones', 'b');
  });
  expect(result.current.allConflictsResolved).toBe(true);
});

test('allConflictsResolved is true for a preview with no conflicts at all', async () => {
  vi.mocked(previewContactMerge).mockResolvedValue(fullPreview());

  const { result } = renderHook(() => useContactMerge());
  await act(async () => {
    await result.current.loadPreview(1, 2);
  });

  expect(result.current.allConflictsResolved).toBe(true);
});

test('setResolution merges each choice without dropping earlier ones', async () => {
  const { result } = renderHook(() => useContactMerge());
  await act(async () => {
    result.current.setResolution('name', 'a');
  });
  await act(async () => {
    result.current.setResolution('phones', 'b');
  });
  expect(result.current.resolutions).toEqual({ name: 'a', phones: 'b' });
});

test('commit passes the resolutions through and clears committing afterwards', async () => {
  vi.mocked(commitContactMerge).mockResolvedValue({ message: 'merged', contact: { id: 1 } });

  const { result } = renderHook(() => useContactMerge());
  await act(async () => {
    result.current.setResolution('name', 'a');
  });

  let outcome: { message: string } | undefined;
  await act(async () => {
    outcome = await result.current.commit(1, 2);
  });

  expect(outcome).toEqual({ message: 'merged', contact: { id: 1 } });
  expect(commitContactMerge).toHaveBeenCalledWith(1, 2, { name: 'a' });
  expect(result.current.committing).toBe(false);
});

test('commit notifies the notifier and rethrows on failure', async () => {
  vi.mocked(commitContactMerge).mockRejectedValue(new Error('merge failed'));
  const showError = vi.fn();

  const { result } = renderHook(() => useContactMerge({ showError }));
  await expect(result.current.commit(1, 2)).rejects.toThrow('merge failed');

  expect(showError).toHaveBeenCalledWith('merge failed');
  await waitFor(() => expect(result.current.committing).toBe(false));
});

test('reset clears the preview, resolutions and error', async () => {
  vi.mocked(previewContactMerge).mockResolvedValue(fullPreview());
  const { result } = renderHook(() => useContactMerge());
  await act(async () => {
    await result.current.loadPreview(1, 2);
    result.current.setResolution('name', 'a');
  });
  expect(result.current.preview).not.toBeNull();

  await act(async () => {
    result.current.reset();
  });

  expect(result.current.preview).toBeNull();
  expect(result.current.resolutions).toEqual({});
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

test('starts with no preview, no resolutions, and nothing in flight', () => {
  const { result } = renderHook(() => useContactMerge());
  expect(result.current.preview).toBeNull();
  expect(result.current.resolutions).toEqual({});
  expect(result.current.loading).toBe(false);
  expect(result.current.committing).toBe(false);
  expect(result.current.error).toBeNull();
});

test('loadPreview reports loading while in flight and clears a previous error', async () => {
  vi.mocked(previewContactMerge).mockRejectedValueOnce(new Error('first'));
  const { result } = renderHook(() => useContactMerge());
  await act(async () => {
    await result.current.loadPreview(1, 2);
  });
  expect(result.current.error).toBe('first');
  expect(console.error).toHaveBeenCalledWith(
    '[previewing contact merge] Error:',
    expect.anything(),
  );

  const d = deferred<ContactMergePreviewResponse>();
  vi.mocked(previewContactMerge).mockReturnValueOnce(d.promise);
  act(() => {
    void result.current.loadPreview(1, 2);
  });
  await waitFor(() => expect(result.current.loading).toBe(true));
  expect(result.current.error).toBeNull();
  expect(previewContactMerge).toHaveBeenLastCalledWith(1, 2);
  await act(async () => {
    d.resolve(fullPreview());
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.preview).not.toBeNull();
});

test('committing is true while the merge request is in flight, and failures are logged by operation', async () => {
  const d = deferred<{ message: string; contact: { id: number } }>();
  vi.mocked(commitContactMerge).mockReturnValueOnce(d.promise as never);
  const { result } = renderHook(() => useContactMerge());
  act(() => {
    void result.current.commit(1, 2);
  });
  await waitFor(() => expect(result.current.committing).toBe(true));
  await act(async () => {
    d.resolve({ message: 'ok', contact: { id: 1 } });
  });
  await waitFor(() => expect(result.current.committing).toBe(false));

  vi.mocked(commitContactMerge).mockRejectedValueOnce(new Error('nope'));
  await act(async () => {
    await expect(result.current.commit(1, 2)).rejects.toThrow('nope');
  });
  expect(console.error).toHaveBeenCalledWith('[merging contacts] Error:', expect.anything());
});

test('commit sends the resolutions chosen after the hook first rendered, and the latest notifier', async () => {
  vi.mocked(commitContactMerge).mockRejectedValue(new Error('merge failed'));
  const first = { showError: vi.fn() };
  const second = { showError: vi.fn() };
  const { result, rerender } = renderHook(({ n }) => useContactMerge(n), {
    initialProps: { n: first },
  });
  await act(async () => {
    result.current.setResolution('name', 'b');
  });
  rerender({ n: second });
  await act(async () => {
    await expect(result.current.commit(3, 4)).rejects.toThrow();
  });
  expect(commitContactMerge).toHaveBeenCalledWith(3, 4, { name: 'b' });
  expect(first.showError).not.toHaveBeenCalled();
  expect(second.showError).toHaveBeenCalledWith('merge failed');
});

test('reset clears an error left by a failed preview', async () => {
  vi.mocked(previewContactMerge).mockRejectedValueOnce(new Error('x'));
  const { result } = renderHook(() => useContactMerge());
  await act(async () => {
    await result.current.loadPreview(1, 2);
  });
  expect(result.current.error).toBe('x');
  act(() => result.current.reset());
  expect(result.current.error).toBeNull();
});

test('a failed preview after a successful one clears the stale preview', async () => {
  vi.mocked(previewContactMerge).mockResolvedValueOnce(fullPreview());
  const { result } = renderHook(() => useContactMerge());
  await act(async () => {
    await result.current.loadPreview(1, 2);
  });
  expect(result.current.preview).not.toBeNull();
  vi.mocked(previewContactMerge).mockRejectedValueOnce(new Error('gone'));
  await act(async () => {
    await result.current.loadPreview(1, 3);
  });
  expect(result.current.preview).toBeNull();
  expect(result.current.error).toBe('gone');
});
