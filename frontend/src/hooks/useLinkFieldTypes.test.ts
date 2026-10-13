import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import {
  createLinkFieldType,
  deleteLinkFieldType,
  getLinkFieldTypes,
  type LinkFieldType,
  type LinkFieldTypeInput,
  reorderLinkFieldTypes,
  updateLinkFieldType,
} from '../api/linkFieldTypes';
import { useLinkFieldTypes } from './useLinkFieldTypes';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock('../api/linkFieldTypes', () => ({
  createLinkFieldType: vi.fn(),
  deleteLinkFieldType: vi.fn(),
  getLinkFieldTypes: vi.fn(),
  reorderLinkFieldTypes: vi.fn(),
  updateLinkFieldType: vi.fn(),
}));

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  for (const fn of [
    createLinkFieldType,
    deleteLinkFieldType,
    getLinkFieldTypes,
    reorderLinkFieldTypes,
    updateLinkFieldType,
  ]) {
    vi.mocked(fn).mockReset();
  }
});

function lt(id: string, category: string): LinkFieldType {
  return { id, category, label: id } as unknown as LinkFieldType;
}

const input = { label: 'x' } as unknown as LinkFieldTypeInput;

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

async function loaded(types: LinkFieldType[], notifier?: { showError: (m: string) => void }) {
  vi.mocked(getLinkFieldTypes).mockResolvedValue(types);
  const hook = renderHook(() => useLinkFieldTypes(notifier));
  await act(async () => {
    await hook.result.current.refreshLinkFieldTypes();
  });
  return hook;
}

test('starts empty with the dialog closed and nothing loading', () => {
  const { result } = renderHook(() => useLinkFieldTypes());
  expect(result.current.linkFieldTypes).toEqual([]);
  expect(result.current.dialogOpen).toBe(false);
  expect(result.current.editingLinkFieldType).toBeNull();
  expect(result.current.loading).toBe(false);
  expect(result.current.error).toBeNull();
});

test('refresh toggles loading, stores the list, and a failure sets (and a retry clears) the error', async () => {
  const d = deferred<LinkFieldType[]>();
  vi.mocked(getLinkFieldTypes).mockReturnValueOnce(d.promise);
  const { result } = renderHook(() => useLinkFieldTypes());
  act(() => {
    void result.current.refreshLinkFieldTypes();
  });
  await waitFor(() => expect(result.current.loading).toBe(true));
  await act(async () => {
    d.resolve([lt('a', 'x')]);
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.linkFieldTypes.map((t) => t.id)).toEqual(['a']);

  vi.mocked(getLinkFieldTypes).mockRejectedValueOnce(new Error('offline'));
  await act(async () => {
    await result.current.refreshLinkFieldTypes();
  });
  expect(result.current.error).toBe('offline');
  expect(result.current.loading).toBe(false);
  expect(console.error).toHaveBeenCalledWith(
    '[fetching link field types] Error:',
    expect.anything(),
  );

  const d2 = deferred<LinkFieldType[]>();
  vi.mocked(getLinkFieldTypes).mockReturnValueOnce(d2.promise);
  act(() => {
    void result.current.refreshLinkFieldTypes();
  });
  await waitFor(() => expect(result.current.loading).toBe(true));
  expect(result.current.error).toBeNull();
  await act(async () => {
    d2.resolve([]);
  });
});

test('add opens the dialog for a new type, edit opens it for the chosen one', async () => {
  const { result } = renderHook(() => useLinkFieldTypes());
  act(() => result.current.handleEditLinkFieldType(lt('a', 'x')));
  expect(result.current.dialogOpen).toBe(true);
  expect(result.current.editingLinkFieldType?.id).toBe('a');

  act(() => result.current.handleAddLinkFieldType());
  expect(result.current.dialogOpen).toBe(true);
  expect(result.current.editingLinkFieldType).toBeNull();
});

test('saving a new type creates it, reloads, and closes the dialog', async () => {
  vi.mocked(createLinkFieldType).mockResolvedValue(lt('n', 'x'));
  vi.mocked(getLinkFieldTypes).mockResolvedValue([lt('n', 'x')]);
  const { result } = renderHook(() => useLinkFieldTypes());
  act(() => result.current.handleAddLinkFieldType());
  await act(async () => {
    await result.current.handleSaveLinkFieldType(input);
  });
  expect(createLinkFieldType).toHaveBeenCalledWith(input);
  expect(updateLinkFieldType).not.toHaveBeenCalled();
  expect(getLinkFieldTypes).toHaveBeenCalledTimes(1);
  expect(result.current.dialogOpen).toBe(false);
  expect(result.current.editingLinkFieldType).toBeNull();
  expect(result.current.linkFieldTypes.map((t) => t.id)).toEqual(['n']);
});

test('saving an edited type updates it by id and closes the dialog', async () => {
  vi.mocked(updateLinkFieldType).mockResolvedValue(lt('a', 'x'));
  vi.mocked(getLinkFieldTypes).mockResolvedValue([lt('a', 'x')]);
  const { result } = renderHook(() => useLinkFieldTypes());
  act(() => result.current.handleEditLinkFieldType(lt('a', 'x')));
  await act(async () => {
    await result.current.handleSaveLinkFieldType(input);
  });
  expect(updateLinkFieldType).toHaveBeenCalledWith('a', input);
  expect(createLinkFieldType).not.toHaveBeenCalled();
  expect(result.current.dialogOpen).toBe(false);
  expect(result.current.editingLinkFieldType).toBeNull();
});

test('a failed save keeps the dialog open, notifies, logs, and rethrows', async () => {
  vi.mocked(createLinkFieldType).mockRejectedValue(new Error('dup'));
  const showError = vi.fn();
  const { result } = renderHook(() => useLinkFieldTypes({ showError }));
  act(() => result.current.handleAddLinkFieldType());
  await act(async () => {
    await expect(result.current.handleSaveLinkFieldType(input)).rejects.toThrow('dup');
  });
  expect(showError).toHaveBeenCalledWith('dup');
  expect(console.error).toHaveBeenCalledWith('[saving link field type] Error:', expect.anything());
  expect(result.current.dialogOpen).toBe(true);
  expect(getLinkFieldTypes).not.toHaveBeenCalled();
});

test('delete calls the API then reloads; a failure notifies, logs, and rethrows', async () => {
  vi.mocked(deleteLinkFieldType).mockResolvedValueOnce(undefined);
  vi.mocked(getLinkFieldTypes).mockResolvedValue([]);
  const showError = vi.fn();
  const { result } = renderHook(() => useLinkFieldTypes({ showError }));
  await act(async () => {
    await result.current.handleDeleteLinkFieldType('a');
  });
  expect(deleteLinkFieldType).toHaveBeenCalledWith('a');
  expect(getLinkFieldTypes).toHaveBeenCalledTimes(1);

  vi.mocked(deleteLinkFieldType).mockRejectedValueOnce(new Error('in use'));
  await act(async () => {
    await expect(result.current.handleDeleteLinkFieldType('a')).rejects.toThrow('in use');
  });
  expect(showError).toHaveBeenCalledWith('in use');
  expect(console.error).toHaveBeenCalledWith(
    '[deleting link field type] Error:',
    expect.anything(),
  );
  expect(getLinkFieldTypes).toHaveBeenCalledTimes(1);
});

test('move swaps within a category and splices the group back into the full order', async () => {
  const { result } = await loaded([lt('a1', 'A'), lt('b1', 'B'), lt('a2', 'A'), lt('b2', 'B')]);
  vi.mocked(reorderLinkFieldTypes).mockResolvedValue([lt('a2', 'A'), lt('b1', 'B')]);
  await act(async () => {
    await result.current.handleMoveLinkFieldType('a1', 1);
  });
  expect(reorderLinkFieldTypes).toHaveBeenLastCalledWith(['a2', 'b1', 'a1', 'b2']);
  expect(result.current.linkFieldTypes.map((t) => t.id)).toEqual(['a2', 'b1']);
});

test('move up swaps with the previous item of the same category', async () => {
  const { result } = await loaded([lt('a1', 'A'), lt('b1', 'B'), lt('a2', 'A')]);
  vi.mocked(reorderLinkFieldTypes).mockResolvedValue([]);
  await act(async () => {
    await result.current.handleMoveLinkFieldType('a2', -1);
  });
  expect(reorderLinkFieldTypes).toHaveBeenLastCalledWith(['a2', 'b1', 'a1']);
});

test('move is a no-op for an unknown id or past either end of its category', async () => {
  const { result } = await loaded([lt('a1', 'A'), lt('b1', 'B'), lt('a2', 'A')]);
  await act(async () => {
    await result.current.handleMoveLinkFieldType('nope', 1);
    await result.current.handleMoveLinkFieldType('a1', -1);
    await result.current.handleMoveLinkFieldType('a2', 1);
    await result.current.handleMoveLinkFieldType('b1', 1);
    await result.current.handleMoveLinkFieldType('b1', -1);
  });
  expect(reorderLinkFieldTypes).not.toHaveBeenCalled();
});

test('a failed move notifies, logs, keeps the list, and rethrows', async () => {
  const showError = vi.fn();
  const { result } = await loaded([lt('a1', 'A'), lt('a2', 'A')], { showError });
  vi.mocked(reorderLinkFieldTypes).mockRejectedValue(new Error('conflict'));
  await act(async () => {
    await expect(result.current.handleMoveLinkFieldType('a1', 1)).rejects.toThrow('conflict');
  });
  expect(showError).toHaveBeenCalledWith('conflict');
  expect(console.error).toHaveBeenCalledWith(
    '[reordering link field types] Error:',
    expect.anything(),
  );
  expect(result.current.linkFieldTypes.map((t) => t.id)).toEqual(['a1', 'a2']);
});
