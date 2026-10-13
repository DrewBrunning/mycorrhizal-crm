import { act, cleanup, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import {
  cancelSourceImport,
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

test('a failing status poll surfaces the error and stops polling', async () => {
  vi.useFakeTimers();
  statusMock.mockRejectedValue(new Error('status unavailable'));

  const { result } = renderHook(() =>
    useSourceImportWizard({ basePath: '/x', startFetch: vi.fn().mockResolvedValue(undefined) }),
  );
  await act(async () => {
    await result.current.beginFetch('s1');
    await vi.advanceTimersByTimeAsync(0);
  });
  expect(result.current.error).toBe('status unavailable');
  expect(result.current.step).toBe('fetching');
  expect(statusMock).toHaveBeenCalledTimes(1);

  // The interval was cleared: later ticks never poll again.
  await act(async () => {
    await vi.advanceTimersByTimeAsync(4500);
  });
  expect(statusMock).toHaveBeenCalledTimes(1);
});

test('resolveRowAction prefers an explicit choice, then the suggestion, then add', () => {
  const chosen = new Map<number, 'add' | 'skip' | 'update'>([[1, 'skip']]);
  expect(resolveRowAction(chosen, { row_index: 1, suggested_action: 'update' })).toBe('skip');
  expect(resolveRowAction(chosen, { row_index: 2, suggested_action: 'update' })).toBe('update');
  expect(resolveRowAction(chosen, { row_index: 3, suggested_action: 'add' })).toBe('add');
  expect(resolveRowAction(chosen, { row_index: 4 })).toBe('add');
});

// --- behavior pins (issue #1662) ---

const cancelMock = vi.mocked(cancelSourceImport);

function st(phase: string, extra: Record<string, unknown> = {}) {
  return { session_id: 's1', phase, phase_done: 0, phase_total: 0, ...extra } as never;
}

function mount(startFetch = vi.fn().mockResolvedValue(undefined), basePath = '/x') {
  return renderHook(() => useSourceImportWizard({ basePath, startFetch }));
}

async function tick(ms = 1500) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
    await vi.advanceTimersByTimeAsync(0);
  });
}

test('starts at connect with nothing loaded', () => {
  const { result } = mount();
  expect(result.current.step).toBe('connect');
  expect(result.current.sessionId).toBeNull();
  expect(result.current.status).toBeNull();
  expect(result.current.preview).toBeNull();
  expect(result.current.result).toBeNull();
  expect(result.current.rowActions.size).toBe(0);
  expect(result.current.error).toBeNull();
  expect(result.current.busy).toBe(false);
});

test('beginFetch records the session, polls the given base path, and polls every 1500ms', async () => {
  vi.useFakeTimers();
  statusMock.mockResolvedValue(st('fetching_contacts'));
  const { result } = mount(undefined, '/contacts/import/monica');
  await act(async () => {
    await result.current.beginFetch('sess-9');
  });
  expect(result.current.sessionId).toBe('sess-9');
  expect(statusMock).toHaveBeenCalledWith('/contacts/import/monica', 'sess-9');
  expect(statusMock).toHaveBeenCalledTimes(1);

  await tick(1499);
  expect(statusMock).toHaveBeenCalledTimes(1);
  await tick(1);
  expect(statusMock).toHaveBeenCalledTimes(2);
  await tick(1500);
  expect(statusMock).toHaveBeenCalledTimes(3);
});

test('beginFetch clears an earlier error and holds busy only while the source starts', async () => {
  const gate: { release?: () => void } = {};
  const startFetch = vi
    .fn()
    .mockRejectedValueOnce(new Error('first'))
    .mockImplementationOnce(
      () =>
        new Promise<void>((res) => {
          gate.release = res;
        }),
    );
  statusMock.mockResolvedValue(st('fetching_contacts'));
  vi.useFakeTimers();
  const { result } = mount(startFetch);
  await act(async () => {
    await result.current.beginFetch('s1');
  });
  expect(result.current.error).toBe('first');
  expect(result.current.busy).toBe(false);
  expect(statusMock).not.toHaveBeenCalled();

  act(() => {
    void result.current.beginFetch('s1');
  });
  expect(result.current.busy).toBe(true);
  expect(result.current.error).toBeNull();
  await act(async () => {
    gate.release?.();
  });
  expect(result.current.busy).toBe(false);
  expect(result.current.step).toBe('fetching');
});

test('a second beginFetch replaces the poll loop rather than stacking intervals', async () => {
  vi.useFakeTimers();
  statusMock.mockResolvedValue(st('fetching_contacts'));
  const { result } = mount();
  await act(async () => {
    await result.current.beginFetch('s1');
  });
  await act(async () => {
    await result.current.beginFetch('s1');
  });
  statusMock.mockClear();
  await tick(1500);
  expect(statusMock).toHaveBeenCalledTimes(1);
});

test('reaching ready stops the poll loop; a failed preview fetch surfaces the error', async () => {
  vi.useFakeTimers();
  statusMock.mockResolvedValue(st('ready'));
  previewMock.mockRejectedValue(new Error('preview broke'));
  const { result } = mount();
  await act(async () => {
    await result.current.beginFetch('s1');
    await vi.advanceTimersByTimeAsync(0);
  });
  expect(result.current.error).toBe('preview broke');
  expect(result.current.step).toBe('fetching');
  expect(result.current.preview).toBeNull();
  expect(statusMock).toHaveBeenCalledTimes(1);
  await tick(4500);
  expect(statusMock).toHaveBeenCalledTimes(1);
});

test('after review is reached no further status polls happen', async () => {
  vi.useFakeTimers();
  statusMock.mockResolvedValue(st('ready'));
  previewMock.mockResolvedValue(preview);
  const { result } = mount();
  await toReview({ current: result.current });
  expect(result.current.step).toBe('review');
  const calls = statusMock.mock.calls.length;
  await tick(6000);
  expect(statusMock.mock.calls.length).toBe(calls);
});

test('a failed phase stops polling and falls back to a generic message when none is given', async () => {
  vi.useFakeTimers();
  statusMock.mockResolvedValue(st('failed'));
  const { result } = mount();
  await act(async () => {
    await result.current.beginFetch('s1');
    await vi.advanceTimersByTimeAsync(0);
  });
  expect(result.current.error).toBe('The import failed.');
  expect(result.current.status?.phase).toBe('failed');
  await tick(4500);
  expect(statusMock).toHaveBeenCalledTimes(1);
});

test('phases only act on the step that expects them', async () => {
  vi.useFakeTimers();
  // While fetching, done/cancelled are ignored (and polling continues).
  statusMock.mockResolvedValue(st('done', { result: doneResult }));
  const a = mount();
  await act(async () => {
    await a.result.current.beginFetch('s1');
    await vi.advanceTimersByTimeAsync(0);
  });
  expect(a.result.current.step).toBe('fetching');
  expect(a.result.current.result).toBeNull();
  await tick(1500);
  expect(statusMock.mock.calls.length).toBeGreaterThan(1);
  cleanup();

  statusMock.mockReset();
  statusMock.mockResolvedValue(st('cancelled'));
  const b = mount();
  await act(async () => {
    await b.result.current.beginFetch('s1');
    await vi.advanceTimersByTimeAsync(0);
  });
  expect(b.result.current.step).toBe('fetching');
  cleanup();

  // While importing, ready is ignored: no preview fetch, polling continues.
  statusMock.mockReset();
  previewMock.mockReset();
  statusMock.mockResolvedValueOnce(st('ready')).mockResolvedValue(st('ready'));
  previewMock.mockResolvedValue(preview);
  confirmMock.mockResolvedValue(undefined);
  const c = mount();
  await toReview({ current: c.result.current });
  previewMock.mockClear();
  await act(async () => {
    await c.result.current.confirm();
    await vi.advanceTimersByTimeAsync(3000);
  });
  expect(c.result.current.step).toBe('importing');
  expect(previewMock).not.toHaveBeenCalled();
});

test('done stores the result and stops polling; done without a result leaves it null', async () => {
  vi.useFakeTimers();
  statusMock
    .mockResolvedValueOnce(st('ready'))
    .mockResolvedValue(st('done', { result: doneResult }));
  previewMock.mockResolvedValue(preview);
  confirmMock.mockResolvedValue(undefined);
  const { result } = mount();
  await toReview({ current: result.current });
  await act(async () => {
    await result.current.confirm();
    await vi.advanceTimersByTimeAsync(0);
  });
  expect(result.current.step).toBe('result');
  expect(result.current.result).toEqual(doneResult);
  const calls = statusMock.mock.calls.length;
  await tick(4500);
  expect(statusMock.mock.calls.length).toBe(calls);
  cleanup();

  statusMock.mockReset();
  statusMock.mockResolvedValueOnce(st('ready')).mockResolvedValue(st('done'));
  const bare = mount();
  await toReview({ current: bare.result.current });
  await act(async () => {
    await bare.result.current.confirm();
    await vi.advanceTimersByTimeAsync(0);
  });
  expect(bare.result.current.step).toBe('result');
  expect(bare.result.current.result).toBeNull();
});

test('a cancelled phase stops polling and clears the error', async () => {
  vi.useFakeTimers();
  statusMock.mockResolvedValueOnce(st('ready')).mockResolvedValue(st('cancelled'));
  previewMock.mockResolvedValue(preview);
  confirmMock.mockRejectedValueOnce(new Error('first try'));
  confirmMock.mockResolvedValue(undefined);
  const { result } = mount();
  await toReview({ current: result.current });
  await act(async () => {
    await result.current.confirm();
  });
  expect(result.current.error).toBe('first try');
  await act(async () => {
    await result.current.confirm();
    await vi.advanceTimersByTimeAsync(0);
  });
  expect(result.current.step).toBe('review');
  expect(result.current.error).toBeNull();
  const calls = statusMock.mock.calls.length;
  await tick(4500);
  expect(statusMock.mock.calls.length).toBe(calls);
});

test('setRowAction records choices without mutating the previous map', async () => {
  const { result } = mount();
  const before = result.current.rowActions;
  act(() => result.current.setRowAction(3, 'skip'));
  expect(result.current.rowActions.get(3)).toBe('skip');
  expect(result.current.rowActions).not.toBe(before);
  expect(before.size).toBe(0);
  act(() => result.current.setRowAction(4, 'update'));
  expect(result.current.rowActions.get(3)).toBe('skip');
  expect(result.current.rowActions.get(4)).toBe('update');
});

test('setAllActions applies skip/add to every row and is empty before a preview exists', async () => {
  const early = mount();
  act(() => early.result.current.setAllActions('skip'));
  expect(early.result.current.rowActions.size).toBe(0);
  cleanup();

  vi.useFakeTimers();
  statusMock.mockResolvedValue(st('ready'));
  previewMock.mockResolvedValue(preview);
  const { result } = mount();
  await toReview({ current: result.current });
  act(() => result.current.setAllActions('skip'));
  expect([...result.current.rowActions.entries()]).toEqual([
    [0, 'skip'],
    [1, 'skip'],
  ]);
  act(() => result.current.setAllActions('add'));
  expect(result.current.rowActions.get(1)).toBe('add');
});

test('confirm without a session or preview does nothing', async () => {
  const { result } = mount();
  await act(async () => {
    await result.current.confirm();
  });
  expect(confirmMock).not.toHaveBeenCalled();
  expect(result.current.busy).toBe(false);
});

test('confirm sends the resolved action for every row, and busy only spans the request', async () => {
  vi.useFakeTimers();
  statusMock.mockResolvedValue(st('ready'));
  previewMock.mockResolvedValue(preview);
  const gate: { release?: () => void } = {};
  confirmMock.mockImplementation(
    () =>
      new Promise<undefined>((res) => {
        gate.release = () => res(undefined);
      }) as never,
  );
  const { result } = mount(undefined, '/imp');
  await toReview({ current: result.current });
  act(() => result.current.setRowAction(0, 'skip'));
  act(() => {
    void result.current.confirm();
  });
  expect(result.current.busy).toBe(true);
  expect(confirmMock).toHaveBeenCalledWith('/imp', 's1', [
    { row_index: 0, action: 'skip' },
    { row_index: 1, action: 'update' },
  ]);
  await act(async () => {
    gate.release?.();
  });
  expect(result.current.busy).toBe(false);
  expect(result.current.step).toBe('importing');
});

test('a failed confirm reports the error, stays on review, and releases busy', async () => {
  vi.useFakeTimers();
  statusMock.mockResolvedValue(st('ready'));
  previewMock.mockResolvedValue(preview);
  confirmMock.mockRejectedValue(new Error('rejected'));
  const { result } = mount();
  await toReview({ current: result.current });
  await act(async () => {
    await result.current.confirm();
  });
  expect(result.current.error).toBe('rejected');
  expect(result.current.step).toBe('review');
  expect(result.current.busy).toBe(false);
});

test('cancelImport needs a session, calls the cancel endpoint, and always releases busy', async () => {
  cancelMock.mockClear();
  const none = mount();
  await act(async () => {
    await none.result.current.cancelImport();
  });
  expect(cancelMock).not.toHaveBeenCalled();
  cleanup();

  vi.useFakeTimers();
  statusMock.mockResolvedValue(st('fetching_contacts'));
  const { result } = mount(undefined, '/imp');
  await act(async () => {
    await result.current.beginFetch('s7');
  });
  cancelMock.mockClear();
  const gate: { release?: () => void } = {};
  cancelMock.mockImplementationOnce(
    () =>
      new Promise<void>((res) => {
        gate.release = res;
      }),
  );
  act(() => {
    void result.current.cancelImport();
  });
  expect(result.current.busy).toBe(true);
  expect(cancelMock).toHaveBeenCalledWith('/imp', 's7');
  await act(async () => {
    gate.release?.();
  });
  expect(result.current.busy).toBe(false);

  cancelMock.mockRejectedValueOnce(new Error('cancel failed'));
  await act(async () => {
    await expect(result.current.cancelImport()).rejects.toThrow('cancel failed');
  });
  expect(result.current.busy).toBe(false);
});

test('cancel stops polling and cancels the session only when one exists', async () => {
  vi.useFakeTimers();
  const idle = mount();
  cancelMock.mockClear();
  await act(async () => {
    await idle.result.current.cancel();
  });
  expect(cancelMock).not.toHaveBeenCalled();
  cleanup();

  statusMock.mockResolvedValue(st('fetching_contacts'));
  const { result } = mount(undefined, '/imp');
  await act(async () => {
    await result.current.beginFetch('s5');
  });
  await act(async () => {
    await result.current.cancel();
  });
  expect(cancelMock).toHaveBeenCalledWith('/imp', 's5');
  const calls = statusMock.mock.calls.length;
  await tick(4500);
  expect(statusMock.mock.calls.length).toBe(calls);
});

test('reset stops polling and clears status, result, error, busy and session', async () => {
  vi.useFakeTimers();
  statusMock
    .mockResolvedValueOnce(st('ready'))
    .mockResolvedValue(st('done', { result: doneResult }));
  previewMock.mockResolvedValue(preview);
  confirmMock.mockResolvedValue(undefined);
  const { result } = mount();
  await toReview({ current: result.current });
  await act(async () => {
    await result.current.confirm();
    await vi.advanceTimersByTimeAsync(0);
  });
  expect(result.current.result).not.toBeNull();

  act(() => result.current.reset());
  expect(result.current.step).toBe('connect');
  expect(result.current.sessionId).toBeNull();
  expect(result.current.status).toBeNull();
  expect(result.current.preview).toBeNull();
  expect(result.current.result).toBeNull();
  expect(result.current.error).toBeNull();
  expect(result.current.busy).toBe(false);
});

test('reset also halts a live poll loop, and unmounting does too', async () => {
  vi.useFakeTimers();
  statusMock.mockResolvedValue(st('fetching_contacts'));
  const a = mount();
  await act(async () => {
    await a.result.current.beginFetch('s1');
  });
  act(() => a.result.current.reset());
  statusMock.mockClear();
  await tick(4500);
  expect(statusMock).not.toHaveBeenCalled();
  cleanup();

  const b = mount();
  await act(async () => {
    await b.result.current.beginFetch('s1');
  });
  statusMock.mockClear();
  b.unmount();
  await tick(4500);
  expect(statusMock).not.toHaveBeenCalled();
});

test('a failed reset-time error is cleared by the next beginFetch', async () => {
  const startFetch = vi.fn().mockRejectedValueOnce(new Error('x')).mockResolvedValue(undefined);
  vi.useFakeTimers();
  statusMock.mockResolvedValue(st('fetching_contacts'));
  const { result } = mount(startFetch);
  await act(async () => {
    await result.current.beginFetch('s1');
  });
  expect(result.current.error).toBe('x');
  await act(async () => {
    await result.current.beginFetch('s1');
  });
  expect(result.current.error).toBeNull();
});

test('resolveRowAction honors skip, ignores unknown suggestions', () => {
  const none = new Map<number, 'add' | 'skip' | 'update'>();
  expect(resolveRowAction(none, { row_index: 1, suggested_action: 'skip' })).toBe('skip');
  expect(resolveRowAction(none, { row_index: 1, suggested_action: 'merge' })).toBe('add');
  expect(resolveRowAction(none, { row_index: 1, suggested_action: '' })).toBe('add');
});

test('a changed base path and start function are picked up by later actions', async () => {
  vi.useFakeTimers();
  statusMock.mockResolvedValue(st('fetching_contacts'));
  const first = vi.fn().mockResolvedValue(undefined);
  const second = vi.fn().mockResolvedValue(undefined);
  const { result, rerender } = renderHook(
    ({ basePath, startFetch }) => useSourceImportWizard({ basePath, startFetch }),
    { initialProps: { basePath: '/one', startFetch: first } },
  );
  rerender({ basePath: '/two', startFetch: second });
  await act(async () => {
    await result.current.beginFetch('s1');
  });
  expect(first).not.toHaveBeenCalled();
  expect(second).toHaveBeenCalledWith('s1');
  expect(statusMock).toHaveBeenLastCalledWith('/two', 's1');
  await tick(1500);
  expect(statusMock).toHaveBeenLastCalledWith('/two', 's1');
});

test('confirm and cancelImport use the latest base path', async () => {
  vi.useFakeTimers();
  statusMock.mockResolvedValue(st('ready'));
  previewMock.mockResolvedValue(preview);
  confirmMock.mockResolvedValue(undefined);
  cancelMock.mockClear();
  const { result, rerender } = renderHook(
    ({ basePath }) =>
      useSourceImportWizard({ basePath, startFetch: vi.fn().mockResolvedValue(undefined) }),
    { initialProps: { basePath: '/one' } },
  );
  await toReview({ current: result.current });
  rerender({ basePath: '/two' });
  await act(async () => {
    await result.current.confirm();
  });
  expect(confirmMock).toHaveBeenLastCalledWith('/two', 's1', expect.anything());
  await act(async () => {
    await result.current.cancelImport();
    await result.current.cancel();
  });
  expect(cancelMock).toHaveBeenCalledWith('/two', 's1');
});

test('a late cancelled answer clears an error left by a concurrently failed poll', async () => {
  vi.useFakeTimers();
  const slowFail = (() => {
    let reject!: (e: unknown) => void;
    const promise = new Promise<never>((_, rej) => {
      reject = rej;
    });
    return { promise, reject };
  })();
  statusMock.mockResolvedValueOnce(st('ready'));
  previewMock.mockResolvedValue(preview);
  confirmMock.mockResolvedValue(undefined);
  const { result } = mount();
  await toReview({ current: result.current });

  // importing: the immediate poll hangs and will fail; the next tick's poll says cancelled.
  statusMock.mockReturnValueOnce(slowFail.promise as never);
  statusMock.mockResolvedValueOnce(st('importing'));
  await act(async () => {
    await result.current.confirm();
  });
  statusMock.mockReset();
  let releaseCancelled!: (v: unknown) => void;
  statusMock.mockReturnValueOnce(
    new Promise((res) => {
      releaseCancelled = res;
    }) as never,
  );
  await tick(1500);
  await act(async () => {
    slowFail.reject(new Error('transient'));
  });
  expect(result.current.error).toBe('transient');
  await act(async () => {
    releaseCancelled(st('cancelled'));
  });
  expect(result.current.error).toBeNull();
  expect(result.current.step).toBe('review');
});

test('confirm clears an earlier error as soon as it starts, and does nothing with a session but no preview', async () => {
  vi.useFakeTimers();
  statusMock.mockResolvedValue(st('fetching_contacts'));
  const idle = mount();
  await act(async () => {
    await idle.result.current.beginFetch('s1');
  });
  confirmMock.mockClear();
  await act(async () => {
    await idle.result.current.confirm();
  });
  expect(confirmMock).not.toHaveBeenCalled();
  cleanup();

  statusMock.mockReset();
  statusMock.mockResolvedValue(st('ready'));
  previewMock.mockResolvedValue(preview);
  confirmMock.mockRejectedValueOnce(new Error('first'));
  const { result } = mount();
  await toReview({ current: result.current });
  await act(async () => {
    await result.current.confirm();
  });
  expect(result.current.error).toBe('first');
  const gate: { release?: () => void } = {};
  confirmMock.mockImplementationOnce(
    () =>
      new Promise<undefined>((res) => {
        gate.release = () => res(undefined);
      }) as never,
  );
  act(() => {
    void result.current.confirm();
  });
  expect(result.current.error).toBeNull();
  await act(async () => {
    gate.release?.();
  });
});

test('reset clears an error left by a failed fetch', async () => {
  const { result } = mount(vi.fn().mockRejectedValue(new Error('cannot start')));
  await act(async () => {
    await result.current.beginFetch('s1');
  });
  expect(result.current.error).toBe('cannot start');
  act(() => result.current.reset());
  expect(result.current.error).toBeNull();
});
