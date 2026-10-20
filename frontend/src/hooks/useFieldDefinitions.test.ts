import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import type { FieldDefinition } from '../api/fieldDefinitions';
import {
  createFieldDefinition,
  deleteFieldDefinition,
  getContactFieldValues,
  getFieldDefinitions,
  reorderFieldDefinitions,
  replaceContactFieldValues,
  updateFieldDefinition,
} from '../api/fieldDefinitions';
import { useContactFieldValues, useFieldDefinitions } from './useFieldDefinitions';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(cleanup);

vi.mock('../api/fieldDefinitions', () => ({
  getContactFieldValues: vi.fn().mockResolvedValue([]),
  replaceContactFieldValues: vi.fn().mockResolvedValue([]),
  getFieldDefinitions: vi.fn().mockResolvedValue({ field_definitions: [] }),
  createFieldDefinition: vi.fn(),
  updateFieldDefinition: vi.fn(),
  deleteFieldDefinition: vi.fn(),
  reorderFieldDefinitions: vi.fn(),
}));

function definition(id: string, label: string, position: number): FieldDefinition {
  return {
    id,
    label,
    key: id,
    target: 'contact',
    type: 'string',
    projection: 'internal-only',
    sensitivity: 'normal',
    position,
    group: '',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  };
}

/**
 * These pin an identity contract, not behaviour, because that is what broke:
 * `refresh` is listed in ContactDetailPage's main fetch effect's dependency
 * array, and that effect's body calls setRecord/setNotes/setActivities. If
 * `refresh` changes identity on every render, the effect re-runs on every
 * render, its own setState calls trigger another render, and the page enters
 * an unconditional render->fetch loop -- measured at ~600 API requests during
 * a single 1.3s e2e test, sustained until the page unmounts.
 *
 * The trigger is that every caller passes the notifier as an inline
 * `{ showError }` object literal, so `notifier` is a fresh identity on every
 * render even though `showError` itself is a stable useCallback. Holding it in
 * a dep array is therefore never safe here. The `rerender` calls below pass a
 * new literal each time on purpose -- that is exactly what a real re-render
 * does.
 */
test('refresh keeps a stable identity when the caller passes a fresh notifier literal', async () => {
  const showError = vi.fn();
  const { result, rerender } = renderHook(({ notifier }) => useContactFieldValues(1, notifier), {
    initialProps: { notifier: { showError } },
  });

  await waitFor(() => expect(result.current.loading).toBe(false));
  const first = result.current.refresh;

  rerender({ notifier: { showError } });
  rerender({ notifier: { showError } });

  expect(result.current.refresh).toBe(first);
});

test('save keeps a stable identity when the caller passes a fresh notifier literal', async () => {
  const showError = vi.fn();
  const { result, rerender } = renderHook(({ notifier }) => useContactFieldValues(1, notifier), {
    initialProps: { notifier: { showError } },
  });

  await waitFor(() => expect(result.current.loading).toBe(false));
  const first = result.current.save;

  rerender({ notifier: { showError } });

  expect(result.current.save).toBe(first);
});

// The identity contract above is worthless if it holds by simply ignoring the
// contactId, so pin the other half: a genuine id change must still produce a
// new callback, or the page would keep fetching the previous contact's values.
test('refresh takes a new identity when the contact id actually changes', async () => {
  const showError = vi.fn();
  const { result, rerender } = renderHook(({ id }) => useContactFieldValues(id, { showError }), {
    initialProps: { id: 1 },
  });

  await waitFor(() => expect(result.current.loading).toBe(false));
  const first = result.current.refresh;

  rerender({ id: 2 });

  expect(result.current.refresh).not.toBe(first);
});

// --- handleMove (issue #1210) ---

test('handleMove swaps adjacent definitions and persists the full order in one call', async () => {
  const first = definition('a', 'Signal', 0);
  const second = definition('b', 'Telegram', 1);
  vi.mocked(getFieldDefinitions).mockResolvedValue({
    field_definitions: [first, second],
    total: 2,
    next_cursor: '',
    limit: 100,
  });
  vi.mocked(reorderFieldDefinitions).mockResolvedValue([
    { ...second, position: 0 },
    { ...first, position: 1 },
  ]);

  const { result } = renderHook(() => useFieldDefinitions());
  await act(async () => {
    await result.current.refresh();
  });
  expect(result.current.definitions.map((d) => d.id)).toEqual(['a', 'b']);

  await act(async () => {
    await result.current.handleMove('a', 1);
  });

  expect(reorderFieldDefinitions).toHaveBeenCalledWith(['b', 'a']);
  expect(result.current.definitions.map((d) => d.id)).toEqual(['b', 'a']);
});

test('handleMove at the list boundary does not call the API', async () => {
  vi.mocked(getFieldDefinitions).mockResolvedValue({
    field_definitions: [definition('a', 'Signal', 0)],
    total: 1,
    next_cursor: '',
    limit: 100,
  });
  vi.mocked(reorderFieldDefinitions).mockClear();

  const { result } = renderHook(() => useFieldDefinitions());
  await act(async () => {
    await result.current.refresh();
  });

  await act(async () => {
    await result.current.handleMove('a', -1);
  });

  expect(reorderFieldDefinitions).not.toHaveBeenCalled();
});

// --- behavior pins (issue #1662) ---

function page(defs: FieldDefinition[]) {
  return { field_definitions: defs, total: defs.length, next_cursor: '', limit: 100 };
}

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

describe('behavior', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.spyOn(console, 'error').mockImplementation(() => {});
    vi.mocked(getFieldDefinitions).mockResolvedValue(page([]));
  });
  afterEach(() => vi.restoreAllMocks());

  test('useFieldDefinitions starts empty and idle, and refresh loads up to 100 definitions', async () => {
    const d = deferred<ReturnType<typeof page>>();
    vi.mocked(getFieldDefinitions).mockReturnValue(d.promise);
    const { result } = renderHook(() => useFieldDefinitions());
    expect(result.current.definitions).toEqual([]);
    expect(result.current.loading).toBe(false);
    expect(result.current.error).toBeNull();
    expect(getFieldDefinitions).not.toHaveBeenCalled();

    act(() => {
      void result.current.refresh();
    });
    await waitFor(() => expect(result.current.loading).toBe(true));
    expect(getFieldDefinitions).toHaveBeenCalledWith(100);
    await act(async () => {
      d.resolve(page([definition('a', 'A', 0)]));
    });
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.definitions.map((x) => x.id)).toEqual(['a']);
  });

  test('refresh falls back to an empty list when the response omits field_definitions', async () => {
    vi.mocked(getFieldDefinitions).mockResolvedValue({} as never);
    const { result } = renderHook(() => useFieldDefinitions());
    await act(async () => {
      await result.current.refresh();
    });
    expect(result.current.definitions).toEqual([]);
  });

  test('refresh surfaces a failure, logs the operation, and a later refresh clears the error', async () => {
    vi.mocked(getFieldDefinitions).mockRejectedValueOnce(new Error('nope'));
    const { result } = renderHook(() => useFieldDefinitions());
    await act(async () => {
      await result.current.refresh();
    });
    expect(result.current.error).toBe('nope');
    expect(result.current.loading).toBe(false);
    expect(console.error).toHaveBeenCalledWith(
      '[fetching custom field definitions] Error:',
      expect.anything(),
    );

    const d = deferred<ReturnType<typeof page>>();
    vi.mocked(getFieldDefinitions).mockReturnValueOnce(d.promise);
    act(() => {
      void result.current.refresh();
    });
    await waitFor(() => expect(result.current.loading).toBe(true));
    expect(result.current.error).toBeNull();
    await act(async () => {
      d.resolve(page([]));
    });
  });

  test.each([
    ['handleCreate', 'creating custom field'],
    ['handleUpdate', 'updating custom field'],
    ['handleDelete', 'deleting custom field'],
  ] as const)('%s calls the API, then reloads the list', async (name, _op) => {
    vi.mocked(createFieldDefinition).mockResolvedValue(definition('n', 'N', 0));
    vi.mocked(updateFieldDefinition).mockResolvedValue(definition('n', 'N', 0));
    vi.mocked(deleteFieldDefinition).mockResolvedValue(undefined);
    vi.mocked(getFieldDefinitions).mockResolvedValue(page([definition('n', 'N', 0)]));
    const input = { label: 'N' } as never;
    const { result } = renderHook(() => useFieldDefinitions());
    await act(async () => {
      if (name === 'handleCreate') await result.current.handleCreate(input);
      else if (name === 'handleUpdate') await result.current.handleUpdate('n', input);
      else await result.current.handleDelete('n');
    });
    if (name === 'handleCreate') expect(createFieldDefinition).toHaveBeenCalledWith(input);
    if (name === 'handleUpdate') expect(updateFieldDefinition).toHaveBeenCalledWith('n', input);
    if (name === 'handleDelete') expect(deleteFieldDefinition).toHaveBeenCalledWith('n');
    expect(getFieldDefinitions).toHaveBeenCalledTimes(1);
    expect(result.current.definitions.map((x) => x.id)).toEqual(['n']);
  });

  test.each([
    ['handleCreate', createFieldDefinition, 'creating custom field'],
    ['handleUpdate', updateFieldDefinition, 'updating custom field'],
    ['handleDelete', deleteFieldDefinition, 'deleting custom field'],
  ] as const)(
    '%s notifies, logs its operation, rethrows, and skips the reload on failure',
    async (name, api, op) => {
      vi.mocked(api).mockRejectedValue(new Error('denied'));
      const showError = vi.fn();
      const { result } = renderHook(() => useFieldDefinitions({ showError }));
      await act(async () => {
        const call =
          name === 'handleCreate'
            ? result.current.handleCreate({} as never)
            : name === 'handleUpdate'
              ? result.current.handleUpdate('x', {} as never)
              : result.current.handleDelete('x');
        await expect(call).rejects.toThrow('denied');
      });
      expect(showError).toHaveBeenCalledWith('denied');
      expect(console.error).toHaveBeenCalledWith(`[${op}] Error:`, expect.anything());
      expect(getFieldDefinitions).not.toHaveBeenCalled();
    },
  );

  async function loaded(ids: string[]) {
    vi.mocked(getFieldDefinitions).mockResolvedValue(
      page(ids.map((id, i) => definition(id, id, i))),
    );
    const showError = vi.fn();
    const hook = renderHook(() => useFieldDefinitions({ showError }));
    await act(async () => {
      await hook.result.current.refresh();
    });
    return { ...hook, showError };
  }

  test('handleMove ignores an unknown id', async () => {
    const { result } = await loaded(['a', 'b']);
    await act(async () => {
      await result.current.handleMove('zzz', 1);
    });
    expect(reorderFieldDefinitions).not.toHaveBeenCalled();
  });

  test('handleMove past the bottom or top edge does not call the API', async () => {
    const { result } = await loaded(['a', 'b']);
    await act(async () => {
      await result.current.handleMove('b', 1);
      await result.current.handleMove('a', -1);
    });
    expect(reorderFieldDefinitions).not.toHaveBeenCalled();
  });

  test('handleMove swaps a middle definition with the neighbor in either direction', async () => {
    const { result } = await loaded(['a', 'b', 'c']);
    vi.mocked(reorderFieldDefinitions).mockResolvedValue([]);
    await act(async () => {
      await result.current.handleMove('b', 1);
    });
    expect(reorderFieldDefinitions).toHaveBeenLastCalledWith(['a', 'c', 'b']);
  });

  test('handleMove up swaps with the previous definition', async () => {
    const { result } = await loaded(['a', 'b', 'c']);
    vi.mocked(reorderFieldDefinitions).mockResolvedValue([]);
    await act(async () => {
      await result.current.handleMove('c', -1);
    });
    expect(reorderFieldDefinitions).toHaveBeenLastCalledWith(['a', 'c', 'b']);
  });

  test('handleMove reports a failed reorder, keeps the list, and rethrows', async () => {
    const { result, showError } = await loaded(['a', 'b']);
    vi.mocked(reorderFieldDefinitions).mockRejectedValue(new Error('conflict'));
    await act(async () => {
      await expect(result.current.handleMove('a', 1)).rejects.toThrow('conflict');
    });
    expect(showError).toHaveBeenCalledWith('conflict');
    expect(console.error).toHaveBeenCalledWith(
      '[reordering custom fields] Error:',
      expect.anything(),
    );
    expect(result.current.definitions.map((x) => x.id)).toEqual(['a', 'b']);
  });

  test('mutating handlers always use the latest notifier', async () => {
    vi.mocked(createFieldDefinition).mockRejectedValue(new Error('c'));
    vi.mocked(updateFieldDefinition).mockRejectedValue(new Error('u'));
    vi.mocked(deleteFieldDefinition).mockRejectedValue(new Error('d'));
    const first = vi.fn();
    const second = vi.fn();
    const { result, rerender } = renderHook(({ n }) => useFieldDefinitions(n), {
      initialProps: { n: { showError: first } },
    });
    rerender({ n: { showError: second } });
    await act(async () => {
      await expect(result.current.handleCreate({} as never)).rejects.toThrow();
      await expect(result.current.handleUpdate('x', {} as never)).rejects.toThrow();
      await expect(result.current.handleDelete('x')).rejects.toThrow();
    });
    expect(first).not.toHaveBeenCalled();
    expect(second).toHaveBeenCalledTimes(3);
  });

  test('handleMove sees definitions loaded after the hook first rendered, and the latest notifier', async () => {
    const first = vi.fn();
    const second = vi.fn();
    vi.mocked(getFieldDefinitions).mockResolvedValue(
      page([definition('a', 'a', 0), definition('b', 'b', 1)]),
    );
    vi.mocked(reorderFieldDefinitions).mockRejectedValue(new Error('x'));
    const { result, rerender } = renderHook(({ n }) => useFieldDefinitions(n), {
      initialProps: { n: { showError: first } },
    });
    await act(async () => {
      await result.current.refresh();
    });
    rerender({ n: { showError: second } });
    await act(async () => {
      await expect(result.current.handleMove('a', 1)).rejects.toThrow('x');
    });
    expect(first).not.toHaveBeenCalled();
    expect(second).toHaveBeenCalledWith('x');
  });

  describe('useContactFieldValues', () => {
    const fv = (defId: string, value: unknown) =>
      ({ id: `v-${defId}`, field_definition_id: defId, value }) as never;

    test('starts empty and idle', () => {
      const { result } = renderHook(() => useContactFieldValues(1));
      expect(result.current.values).toEqual([]);
      expect(result.current.loading).toBe(false);
      expect(result.current.error).toBeNull();
      expect(result.current.valuesByDefinition.size).toBe(0);
    });

    test('refresh with no contact id and no override does nothing', async () => {
      const { result } = renderHook(() => useContactFieldValues(undefined));
      await act(async () => {
        await result.current.refresh();
      });
      expect(getContactFieldValues).not.toHaveBeenCalled();
      expect(result.current.loading).toBe(false);
    });

    test('refresh fetches for the contact id, indexing values by definition, with loading around it', async () => {
      const d = deferred<never[]>();
      vi.mocked(getContactFieldValues).mockReturnValue(d.promise as never);
      const { result } = renderHook(() => useContactFieldValues(7));
      act(() => {
        void result.current.refresh();
      });
      await waitFor(() => expect(result.current.loading).toBe(true));
      expect(getContactFieldValues).toHaveBeenCalledWith(7);
      await act(async () => {
        d.resolve([fv('d1', 'x'), fv('d2', 5)] as never[]);
      });
      await waitFor(() => expect(result.current.loading).toBe(false));
      expect(result.current.values).toHaveLength(2);
      expect(result.current.valuesByDefinition.get('d1')).toBe('x');
      expect(result.current.valuesByDefinition.get('d2')).toBe(5);
    });

    test('an override id wins over the hook contact id, and rescues an undefined one', async () => {
      vi.mocked(getContactFieldValues).mockResolvedValue([]);
      const { result } = renderHook(() => useContactFieldValues(undefined));
      await act(async () => {
        await result.current.refresh(42);
      });
      expect(getContactFieldValues).toHaveBeenCalledWith(42);

      const second = renderHook(() => useContactFieldValues(1));
      await act(async () => {
        await second.result.current.refresh(99);
      });
      expect(getContactFieldValues).toHaveBeenLastCalledWith(99);
    });

    test('a failed refresh sets the error, notifies with the current notifier, and logs', async () => {
      vi.mocked(getContactFieldValues).mockRejectedValue(new Error('gone'));
      const showError = vi.fn();
      const { result } = renderHook(() => useContactFieldValues(1, { showError }));
      await act(async () => {
        await result.current.refresh();
      });
      expect(result.current.error).toBe('gone');
      expect(result.current.loading).toBe(false);
      expect(showError).toHaveBeenCalledWith('gone');
      // Logged once by the error-string formatter and once by the notifier path.
      expect(
        vi
          .mocked(console.error)
          .mock.calls.filter((c) => c[0] === '[fetching custom field values] Error:'),
      ).toHaveLength(2);

      // The next refresh clears the stale error while in flight.
      const d = deferred<never[]>();
      vi.mocked(getContactFieldValues).mockReturnValueOnce(d.promise as never);
      act(() => {
        void result.current.refresh();
      });
      await waitFor(() => expect(result.current.loading).toBe(true));
      expect(result.current.error).toBeNull();
      await act(async () => {
        d.resolve([]);
      });
    });

    test('refresh and save follow the contact id when it changes', async () => {
      vi.mocked(getContactFieldValues).mockResolvedValue([]);
      vi.mocked(replaceContactFieldValues).mockResolvedValue([]);
      const { result, rerender } = renderHook(({ id }) => useContactFieldValues(id), {
        initialProps: { id: 1 },
      });
      rerender({ id: 2 });
      await act(async () => {
        await result.current.refresh();
        await result.current.save([]);
      });
      expect(getContactFieldValues).toHaveBeenLastCalledWith(2);
      expect(replaceContactFieldValues).toHaveBeenLastCalledWith(2, []);
    });

    test('valuesByDefinition tracks later changes to the values', async () => {
      vi.mocked(replaceContactFieldValues)
        .mockResolvedValueOnce([fv('d1', 'one')] as never)
        .mockResolvedValueOnce([fv('d1', 'two')] as never);
      const { result } = renderHook(() => useContactFieldValues(1));
      await act(async () => {
        await result.current.save([]);
      });
      expect(result.current.valuesByDefinition.get('d1')).toBe('one');
      await act(async () => {
        await result.current.save([]);
      });
      expect(result.current.valuesByDefinition.get('d1')).toBe('two');
    });

    test('save without a contact id is a no-op', async () => {
      const { result } = renderHook(() => useContactFieldValues(undefined));
      await act(async () => {
        await result.current.save([]);
      });
      expect(replaceContactFieldValues).not.toHaveBeenCalled();
    });

    test('save replaces the values and stores the server response', async () => {
      vi.mocked(replaceContactFieldValues).mockResolvedValue([fv('d1', 'saved')] as never);
      const { result } = renderHook(() => useContactFieldValues(3));
      const input = [{ field_definition_id: 'd1', value: 'saved' }] as never;
      await act(async () => {
        await result.current.save(input);
      });
      expect(replaceContactFieldValues).toHaveBeenCalledWith(3, input);
      expect(result.current.valuesByDefinition.get('d1')).toBe('saved');
    });

    test('a failed save notifies, logs, and rethrows without touching the values', async () => {
      vi.mocked(replaceContactFieldValues).mockRejectedValue(new Error('rejected'));
      const showError = vi.fn();
      const { result } = renderHook(() => useContactFieldValues(3, { showError }));
      await act(async () => {
        await expect(result.current.save([])).rejects.toThrow('rejected');
      });
      expect(showError).toHaveBeenCalledWith('rejected');
      expect(console.error).toHaveBeenCalledWith(
        '[saving custom field values] Error:',
        expect.anything(),
      );
      expect(result.current.values).toEqual([]);
    });
  });
});
