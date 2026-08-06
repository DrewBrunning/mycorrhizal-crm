import { test, expect, vi, afterEach } from 'vitest';
import { renderHook, cleanup, waitFor } from '@testing-library/react';
import { useContactFieldValues } from './useFieldDefinitions';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(cleanup);

vi.mock('../api/fieldDefinitions', () => ({
  getContactFieldValues: vi.fn().mockResolvedValue([]),
  replaceContactFieldValues: vi.fn().mockResolvedValue([]),
  getFieldDefinitions: vi.fn().mockResolvedValue({ field_definitions: [] }),
  createFieldDefinition: vi.fn(),
  updateFieldDefinition: vi.fn(),
  deleteFieldDefinition: vi.fn(),
}));

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
  const { result, rerender } = renderHook(
    ({ notifier }) => useContactFieldValues(1, notifier),
    { initialProps: { notifier: { showError } } }
  );

  await waitFor(() => expect(result.current.loading).toBe(false));
  const first = result.current.refresh;

  rerender({ notifier: { showError } });
  rerender({ notifier: { showError } });

  expect(result.current.refresh).toBe(first);
});

test('save keeps a stable identity when the caller passes a fresh notifier literal', async () => {
  const showError = vi.fn();
  const { result, rerender } = renderHook(
    ({ notifier }) => useContactFieldValues(1, notifier),
    { initialProps: { notifier: { showError } } }
  );

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
  const { result, rerender } = renderHook(
    ({ id }) => useContactFieldValues(id, { showError }),
    { initialProps: { id: 1 } }
  );

  await waitFor(() => expect(result.current.loading).toBe(false));
  const first = result.current.refresh;

  rerender({ id: 2 });

  expect(result.current.refresh).not.toBe(first);
});
