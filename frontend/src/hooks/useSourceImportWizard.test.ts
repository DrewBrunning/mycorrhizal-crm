import { act, cleanup, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import {
  confirmSourceImport,
  getSourceImportPreview,
  getSourceImportStatus,
  type SourceImportPreviewResponse,
} from '../api/sourceImport';
import { resolveRowAction, useSourceImportWizard } from './useSourceImportWizard';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.useRealTimers();
});

vi.mock('../api/sourceImport', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/sourceImport')>();
  return {
    ...actual,
    getSourceImportStatus: vi.fn(),
    getSourceImportPreview: vi.fn(),
    confirmSourceImport: vi.fn(),
    cancelSourceImport: vi.fn().mockResolvedValue(undefined),
  };
});

const statusMock = vi.mocked(getSourceImportStatus);
const previewMock = vi.mocked(getSourceImportPreview);
const confirmMock = vi.mocked(confirmSourceImport);

const preview: SourceImportPreviewResponse = {
  session_id: 's1',
  total_rows: 2,
  valid_rows: 2,
  duplicate_count: 1,
  error_count: 0,
  totals: { activities: 0, notes: 1, reminders: 0, relationships: 0, gifts: 0 },
  loss_report: [
    { record: 'monica contact/1', field: 'is_dead', category: 'transformed', message: 'no date' },
  ],
  rows: [
    {
      row_index: 0,
      parsed_contact: { firstname: 'Ada' },
      validation_errors: [],
      duplicate_match: null,
      suggested_action: 'add',
      merge_diff: null,
      batch_duplicate_of: null,
      related: { activities: 0, notes: 1, reminders: 0, relationships: 0, gifts: 0 },
      has_photo: false,
    },
    {
      row_index: 1,
      parsed_contact: { firstname: 'Grace' },
      validation_errors: [],
      duplicate_match: {
        existing_contact_id: 9,
        existing_firstname: 'Grace',
        existing_lastname: 'Hopper',
        existing_email: '',
        existing_phone: '',
        match_reason: 'name',
      },
      merge_diff: null,
      batch_duplicate_of: null,
      suggested_action: 'update',
      related: { activities: 0, notes: 0, reminders: 0, relationships: 0, gifts: 0 },
      has_photo: false,
    },
  ],
};

beforeEach(() => {
  statusMock.mockReset();
  previewMock.mockReset();
  confirmMock.mockReset();
});

test('polls the fetch to ready, then loads the preview and moves to review', async () => {
  vi.useFakeTimers();
  statusMock
    .mockResolvedValueOnce({
      session_id: 's1',
      phase: 'fetching_contacts',
      phase_done: 1,
      phase_total: 3,
    })
    .mockResolvedValue({ session_id: 's1', phase: 'ready', phase_done: 3, phase_total: 3 });
  previewMock.mockResolvedValue(preview);
  const startFetch = vi.fn().mockResolvedValue(undefined);

  const { result } = renderHook(() => useSourceImportWizard({ basePath: '/x', startFetch }));

  await act(async () => {
    await result.current.beginFetch('s1');
  });
  expect(startFetch).toHaveBeenCalledWith('s1');
  // The immediate first poll saw "fetching_contacts".
  expect(result.current.step).toBe('fetching');
  expect(result.current.status?.phase).toBe('fetching_contacts');

  // Next poll tick: "ready" -> preview -> review.
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1500);
    await vi.advanceTimersByTimeAsync(0);
  });
  expect(result.current.step).toBe('review');
  expect(result.current.preview?.loss_report).toHaveLength(1);
});

test('setAllActions("update") only assigns update where a duplicate exists', async () => {
  vi.useFakeTimers();
  statusMock.mockResolvedValue({ session_id: 's1', phase: 'ready', phase_done: 3, phase_total: 3 });
  previewMock.mockResolvedValue(preview);

  const { result } = renderHook(() =>
    useSourceImportWizard({ basePath: '/x', startFetch: vi.fn().mockResolvedValue(undefined) }),
  );
  await act(async () => {
    await result.current.beginFetch('s1');
    await vi.advanceTimersByTimeAsync(1500);
    await vi.advanceTimersByTimeAsync(0);
  });
  expect(result.current.step).toBe('review');

  act(() => result.current.setAllActions('update'));
  expect(result.current.rowActions.get(0)).toBe('add'); // no duplicate -> add
  expect(result.current.rowActions.get(1)).toBe('update'); // duplicate -> update
});

const doneResult = {
  total_processed: 2,
  created: 1,
  updated: 1,
  skipped: 0,
  errors: [] as string[],
  relationships_created: 0,
  notes_created: 1,
  activities_created: 0,
  reminders_created: 0,
  gifts_created: 0,
  custom_fields_created: 0,
  photos_queued: 0,
  photos_saved: 0,
  photos_failed: 0,
};

// Drives beginFetch + poll to the review step. Leaves fake timers on.
async function toReview(result: { current: ReturnType<typeof useSourceImportWizard> }) {
  await act(async () => {
    await result.current.beginFetch('s1');
    await vi.advanceTimersByTimeAsync(1500);
    await vi.advanceTimersByTimeAsync(0);
  });
}

test('confirm returns 202, moves to importing, then polls the background import to done', async () => {
  vi.useFakeTimers();
  statusMock
    .mockResolvedValueOnce({ session_id: 's1', phase: 'ready', phase_done: 3, phase_total: 3 })
    // during confirm: one "importing" tick then "done" with the result
    .mockResolvedValueOnce({ session_id: 's1', phase: 'importing', phase_done: 1, phase_total: 12 })
    .mockResolvedValue({
      session_id: 's1',
      phase: 'done',
      phase_done: 12,
      phase_total: 12,
      result: doneResult,
    });
  previewMock.mockResolvedValue(preview);
  confirmMock.mockResolvedValue(undefined);

  const { result } = renderHook(() =>
    useSourceImportWizard({ basePath: '/x', startFetch: vi.fn().mockResolvedValue(undefined) }),
  );
  await toReview({ current: result.current });

  await act(async () => {
    await result.current.confirm();
  });
  const [, sid, actions] = confirmMock.mock.calls[0];
  expect(sid).toBe('s1');
  expect(actions).toEqual([
    { row_index: 0, action: 'add' },
    { row_index: 1, action: 'update' },
  ]);
  expect(result.current.step).toBe('importing');

  await act(async () => {
    await vi.advanceTimersByTimeAsync(1500);
    await vi.advanceTimersByTimeAsync(0);
  });
  expect(result.current.step).toBe('result');
  expect(result.current.result?.updated).toBe(1);
});

test('a cancelled in-flight import returns the wizard to the review step', async () => {
  vi.useFakeTimers();
  statusMock
    .mockResolvedValueOnce({ session_id: 's1', phase: 'ready', phase_done: 3, phase_total: 3 })
    .mockResolvedValue({ session_id: 's1', phase: 'cancelled', phase_done: 0, phase_total: 12 });
  previewMock.mockResolvedValue(preview);
  confirmMock.mockResolvedValue(undefined);

  const { result } = renderHook(() =>
    useSourceImportWizard({ basePath: '/x', startFetch: vi.fn().mockResolvedValue(undefined) }),
  );
  await toReview({ current: result.current });
  await act(async () => {
    await result.current.confirm();
  });
  await act(async () => {
    await result.current.cancelImport();
    await vi.advanceTimersByTimeAsync(1500);
    await vi.advanceTimersByTimeAsync(0);
  });
  expect(result.current.step).toBe('review');
  expect(result.current.error).toBeNull();
});

test('a failed background import surfaces the error', async () => {
  vi.useFakeTimers();
  statusMock
    .mockResolvedValueOnce({ session_id: 's1', phase: 'ready', phase_done: 3, phase_total: 3 })
    .mockResolvedValue({
      session_id: 's1',
      phase: 'failed',
      phase_done: 0,
      phase_total: 0,
      error: 'disk full',
    });
  previewMock.mockResolvedValue(preview);
  confirmMock.mockResolvedValue(undefined);

  const { result } = renderHook(() =>
    useSourceImportWizard({ basePath: '/x', startFetch: vi.fn().mockResolvedValue(undefined) }),
  );
  await toReview({ current: result.current });
  await act(async () => {
    await result.current.confirm();
    await vi.advanceTimersByTimeAsync(1500);
    await vi.advanceTimersByTimeAsync(0);
  });
  expect(result.current.error).toBe('disk full');
});

test('reset clears every field back to the connect step', async () => {
  vi.useFakeTimers();
  statusMock.mockResolvedValue({ session_id: 's1', phase: 'ready', phase_done: 3, phase_total: 3 });
  previewMock.mockResolvedValue(preview);

  const { result } = renderHook(() =>
    useSourceImportWizard({ basePath: '/x', startFetch: vi.fn().mockResolvedValue(undefined) }),
  );
  await toReview({ current: result.current });
  act(() => result.current.setRowAction(0, 'skip'));
  act(() => result.current.reset());
  expect(result.current.step).toBe('connect');
  expect(result.current.preview).toBeNull();
  expect(result.current.rowActions.size).toBe(0);
});

test('a fetch failure during beginFetch surfaces the error', async () => {
  const startFetch = vi.fn().mockRejectedValue(new Error('cannot start'));
  const { result } = renderHook(() => useSourceImportWizard({ basePath: '/x', startFetch }));
  await act(async () => {
    await result.current.beginFetch('s1');
  });
  expect(result.current.error).toBe('cannot start');
  expect(result.current.step).toBe('connect');
});

test('resolveRowAction prefers an explicit choice, then the suggestion, then add', () => {
  const chosen = new Map<number, 'add' | 'skip' | 'update'>([[1, 'skip']]);
  expect(resolveRowAction(chosen, { row_index: 1, suggested_action: 'update' })).toBe('skip');
  expect(resolveRowAction(chosen, { row_index: 2, suggested_action: 'update' })).toBe('update');
  expect(resolveRowAction(chosen, { row_index: 3, suggested_action: 'add' })).toBe('add');
  expect(resolveRowAction(chosen, { row_index: 4 })).toBe('add');
});
