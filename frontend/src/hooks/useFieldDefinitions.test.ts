import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import type { FieldDefinition } from '../api/fieldDefinitions';
import { getFieldDefinitions, reorderFieldDefinitions } from '../api/fieldDefinitions';
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
