import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import {
  addOccasionEventAttendee,
  createOccasionEvent,
  deleteOccasionEvent,
  getInviteeSuggestions,
  getOccasionEvent,
  getOccasionEvents,
  type OccasionEvent,
  type OccasionEventInput,
  removeOccasionEventAttendee,
  updateOccasionEvent,
  updateOccasionEventAttendee,
} from '../api/occasionEvents';
import { useOccasionEventAttendees, useOccasionEvents } from './useOccasionEvents';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock('../api/occasionEvents', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/occasionEvents')>();
  return {
    ...actual,
    getOccasionEvents: vi.fn(),
    getOccasionEvent: vi.fn(),
    createOccasionEvent: vi.fn(),
    updateOccasionEvent: vi.fn(),
    deleteOccasionEvent: vi.fn(),
    addOccasionEventAttendee: vi.fn(),
    updateOccasionEventAttendee: vi.fn(),
    removeOccasionEventAttendee: vi.fn(),
    getInviteeSuggestions: vi.fn(),
  };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  for (const fn of [
    getOccasionEvents,
    getOccasionEvent,
    createOccasionEvent,
    updateOccasionEvent,
    deleteOccasionEvent,
    addOccasionEventAttendee,
    updateOccasionEventAttendee,
    removeOccasionEventAttendee,
    getInviteeSuggestions,
  ]) {
    vi.mocked(fn).mockReset();
  }
});

const event: OccasionEvent = {
  id: 'ev-1',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  title: 'Summer BBQ',
  starts_at: '2026-07-04T15:00:00Z',
  sensitivity: 'normal',
};

test('useOccasionEvents loads on mount and clears loading', async () => {
  vi.mocked(getOccasionEvents).mockResolvedValue({
    occasion_events: [event],
    total: 1,
    next_cursor: '',
    limit: 100,
  });

  const { result } = renderHook(() => useOccasionEvents());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.events).toHaveLength(1);
  expect(result.current.error).toBeNull();
});

test('useOccasionEvents handleSave updates an existing event', async () => {
  vi.mocked(getOccasionEvents).mockResolvedValue({
    occasion_events: [event],
    total: 1,
    next_cursor: '',
    limit: 100,
  });

  const { result } = renderHook(() => useOccasionEvents());
  await waitFor(() => expect(result.current.loading).toBe(false));

  const input: OccasionEventInput = { title: 'Renamed', starts_at: '2026-07-04T15:00:00Z' };
  await act(async () => {
    await result.current.handleSave(event, input);
  });

  expect(updateOccasionEvent).toHaveBeenCalledWith('ev-1', input);
  expect(createOccasionEvent).not.toHaveBeenCalled();
});

test('useOccasionEvents handleSave creates when there is no event', async () => {
  vi.mocked(getOccasionEvents).mockResolvedValue({
    occasion_events: [],
    total: 0,
    next_cursor: '',
    limit: 100,
  });

  const { result } = renderHook(() => useOccasionEvents());
  await waitFor(() => expect(result.current.loading).toBe(false));

  const input: OccasionEventInput = { title: 'New', starts_at: '2026-07-04T15:00:00Z' };
  await act(async () => {
    await result.current.handleSave(null, input);
  });

  expect(createOccasionEvent).toHaveBeenCalledWith(input);
});

test('useOccasionEvents handleDelete deletes and notifies on failure', async () => {
  vi.mocked(getOccasionEvents).mockResolvedValue({
    occasion_events: [event],
    total: 1,
    next_cursor: '',
    limit: 100,
  });
  vi.mocked(deleteOccasionEvent).mockRejectedValue(new Error('boom'));
  const showError = vi.fn();

  const { result } = renderHook(() => useOccasionEvents({ showError }));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await expect(result.current.handleDelete('ev-1')).rejects.toThrow('boom');
  expect(showError).toHaveBeenCalledWith('boom');
});

test('useOccasionEvents sets error when the fetch fails', async () => {
  vi.mocked(getOccasionEvents).mockRejectedValue(new Error('boom'));

  const { result } = renderHook(() => useOccasionEvents());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.error).toBe('boom');
  expect(result.current.events).toEqual([]);
});

// ---- attendees hook -------------------------------------------------------

const attendeeView = {
  id: 'a-1',
  event_id: 'ev-1',
  entity_id: 'uid-1',
  contact_id: 3,
  contact_name: 'Alice',
  rsvp: 'pending' as const,
};

test('useOccasionEventAttendees loads the event detail', async () => {
  vi.mocked(getOccasionEvent).mockResolvedValue({
    occasion_event: event,
    attendees: [attendeeView],
  });

  const { result } = renderHook(() => useOccasionEventAttendees('ev-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.attendees).toHaveLength(1);
  expect(getOccasionEvent).toHaveBeenCalledWith('ev-1');
});

test('useOccasionEventAttendees does nothing without an event id', async () => {
  const { result } = renderHook(() => useOccasionEventAttendees(undefined));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getOccasionEvent).not.toHaveBeenCalled();
  expect(result.current.attendees).toEqual([]);
});

test('attendee add / update / remove refresh the list', async () => {
  vi.mocked(getOccasionEvent).mockResolvedValue({ occasion_event: event, attendees: [] });
  vi.mocked(addOccasionEventAttendee).mockResolvedValue({
    id: 'a-1',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    event_id: 'ev-1',
    entity_id: 'uid-1',
    rsvp: 'pending',
  });
  vi.mocked(updateOccasionEventAttendee).mockResolvedValue({
    id: 'a-1',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    event_id: 'ev-1',
    entity_id: 'uid-1',
    rsvp: 'accepted',
  });
  vi.mocked(removeOccasionEventAttendee).mockResolvedValue(undefined);

  const { result } = renderHook(() => useOccasionEventAttendees('ev-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.handleAdd('uid-1');
  });
  await act(async () => {
    await result.current.handleUpdateRsvp('uid-1', 'accepted');
  });
  await act(async () => {
    await result.current.handleRemove('uid-1');
  });

  expect(addOccasionEventAttendee).toHaveBeenCalledWith('ev-1', { entity_id: 'uid-1' });
  expect(updateOccasionEventAttendee).toHaveBeenCalledWith('ev-1', 'uid-1', 'accepted');
  expect(removeOccasionEventAttendee).toHaveBeenCalledWith('ev-1', 'uid-1');
  // one initial load + one refresh per mutation
  expect(getOccasionEvent).toHaveBeenCalledTimes(4);
});

test('loadSuggestions fetches and clearSuggestions empties them', async () => {
  vi.mocked(getOccasionEvent).mockResolvedValue({ occasion_event: event, attendees: [] });
  vi.mocked(getInviteeSuggestions).mockResolvedValue([
    { contact_id: 1, contact_name: 'Bob', entity_id: 'uid-2' },
  ]);

  const { result } = renderHook(() => useOccasionEventAttendees('ev-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.loadSuggestions(['c1']);
  });
  expect(result.current.suggestions).toHaveLength(1);

  act(() => result.current.clearSuggestions());
  expect(result.current.suggestions).toEqual([]);
});

test('loadSuggestions with no circles and a failed fetch both clear', async () => {
  vi.mocked(getOccasionEvent).mockResolvedValue({ occasion_event: event, attendees: [] });
  vi.mocked(getInviteeSuggestions).mockRejectedValue(new Error('boom'));
  const showError = vi.fn();

  const { result } = renderHook(() => useOccasionEventAttendees('ev-1', { showError }));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.loadSuggestions([]);
  });
  expect(getInviteeSuggestions).not.toHaveBeenCalled();

  await act(async () => {
    await result.current.loadSuggestions(['c1']);
  });
  expect(showError).toHaveBeenCalledWith('boom');
  expect(result.current.suggestions).toEqual([]);
});

// --- behavior pins (issue #1662) ---

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

const asInput = {} as OccasionEventInput;

test('useOccasionEvents starts empty and idle, fetches up to 100, and reports loading while in flight', async () => {
  const d = deferred<never>();
  vi.mocked(getOccasionEvents).mockReturnValue(d.promise);
  const { result } = renderHook(() => useOccasionEvents());
  expect(result.current.events).toEqual([]);
  expect(result.current.error).toBeNull();
  await waitFor(() => expect(result.current.loading).toBe(true));
  expect(getOccasionEvents).toHaveBeenCalledWith({ limit: 100 });
  await act(async () => {
    d.resolve({ occasion_events: [event] } as never);
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.events).toEqual([event]);
});

test('useOccasionEvents tolerates a response without occasion_events', async () => {
  vi.mocked(getOccasionEvents).mockResolvedValue({} as never);
  const { result } = renderHook(() => useOccasionEvents());
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.events).toEqual([]);
});

test('useOccasionEvents logs the failed operation and a later refresh clears the error', async () => {
  vi.mocked(getOccasionEvents).mockRejectedValueOnce(new Error('offline'));
  const { result } = renderHook(() => useOccasionEvents());
  await waitFor(() => expect(result.current.error).toBe('offline'));
  expect(console.error).toHaveBeenCalledWith(
    '[fetching occasion events] Error:',
    expect.anything(),
  );

  const d = deferred<never>();
  vi.mocked(getOccasionEvents).mockReturnValueOnce(d.promise);
  act(() => {
    void result.current.refresh();
  });
  await waitFor(() => expect(result.current.loading).toBe(true));
  expect(result.current.error).toBeNull();
  await act(async () => {
    d.resolve({ occasion_events: [] } as never);
  });
});

test('useOccasionEvents mutations notify, log their operation, rethrow, and skip the reload on failure', async () => {
  vi.mocked(getOccasionEvents).mockResolvedValue({ occasion_events: [] } as never);
  vi.mocked(updateOccasionEvent).mockRejectedValue(new Error('u'));
  vi.mocked(createOccasionEvent).mockRejectedValue(new Error('c'));
  vi.mocked(deleteOccasionEvent).mockRejectedValue(new Error('d'));
  const showError = vi.fn();
  const { result } = renderHook(() => useOccasionEvents({ showError }));
  await waitFor(() => expect(result.current.loading).toBe(false));
  vi.mocked(getOccasionEvents).mockClear();

  await act(async () => {
    await expect(result.current.handleSave(event, asInput)).rejects.toThrow('u');
    await expect(result.current.handleSave(null, asInput)).rejects.toThrow('c');
    await expect(result.current.handleDelete('ev-1')).rejects.toThrow('d');
  });
  expect(showError.mock.calls.map((c) => c[0])).toEqual(['u', 'c', 'd']);
  expect(console.error).toHaveBeenCalledWith('[saving occasion event] Error:', expect.anything());
  expect(console.error).toHaveBeenCalledWith('[deleting occasion event] Error:', expect.anything());
  expect(getOccasionEvents).not.toHaveBeenCalled();
});

test('useOccasionEvents mutations use the latest notifier', async () => {
  vi.mocked(getOccasionEvents).mockResolvedValue({ occasion_events: [] } as never);
  vi.mocked(createOccasionEvent).mockRejectedValue(new Error('c'));
  vi.mocked(deleteOccasionEvent).mockRejectedValue(new Error('d'));
  const first = vi.fn();
  const second = vi.fn();
  const { result, rerender } = renderHook(({ n }) => useOccasionEvents(n), {
    initialProps: { n: { showError: first } },
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
  rerender({ n: { showError: second } });
  await act(async () => {
    await expect(result.current.handleSave(null, asInput)).rejects.toThrow();
    await expect(result.current.handleDelete('x')).rejects.toThrow();
  });
  expect(first).not.toHaveBeenCalled();
  expect(second).toHaveBeenCalledTimes(2);
});

// --- useOccasionEventAttendees ---

test('attendees start empty and idle; loading is true while the detail loads', async () => {
  const d = deferred<never>();
  vi.mocked(getOccasionEvent).mockReturnValue(d.promise);
  const { result } = renderHook(() => useOccasionEventAttendees('ev-1'));
  expect(result.current.attendees).toEqual([]);
  expect(result.current.suggestions).toEqual([]);
  expect(result.current.suggestionsLoading).toBe(false);
  expect(result.current.error).toBeNull();
  await waitFor(() => expect(result.current.loading).toBe(true));
  expect(getOccasionEvent).toHaveBeenCalledWith('ev-1');
  await act(async () => {
    d.resolve({ attendees: [{ vcard_uid: 'u1' }] } as never);
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.attendees).toEqual([{ vcard_uid: 'u1' }]);
});

test('attendees tolerate a detail without attendees and clear when the event id goes away', async () => {
  vi.mocked(getOccasionEvent).mockResolvedValueOnce({ attendees: [{ vcard_uid: 'u1' }] } as never);
  const { result, rerender } = renderHook(({ id }) => useOccasionEventAttendees(id), {
    initialProps: { id: 'ev-1' as string | undefined },
  });
  await waitFor(() => expect(result.current.attendees).toHaveLength(1));

  rerender({ id: undefined });
  await waitFor(() => expect(result.current.attendees).toEqual([]));
  expect(result.current.loading).toBe(false);

  vi.mocked(getOccasionEvent).mockResolvedValueOnce({} as never);
  rerender({ id: 'ev-2' });
  await waitFor(() => expect(getOccasionEvent).toHaveBeenLastCalledWith('ev-2'));
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.attendees).toEqual([]);
});

test('attendees report a failed load with its operation, and a refresh clears the error', async () => {
  vi.mocked(getOccasionEvent).mockRejectedValueOnce(new Error('bad'));
  const { result } = renderHook(() => useOccasionEventAttendees('ev-1'));
  await waitFor(() => expect(result.current.error).toBe('bad'));
  expect(console.error).toHaveBeenCalledWith(
    '[fetching event attendees] Error:',
    expect.anything(),
  );
  const d = deferred<never>();
  vi.mocked(getOccasionEvent).mockReturnValueOnce(d.promise);
  act(() => {
    void result.current.refresh();
  });
  await waitFor(() => expect(result.current.loading).toBe(true));
  expect(result.current.error).toBeNull();
  await act(async () => {
    d.resolve({ attendees: [] } as never);
  });
});

test('attendee mutations pass their arguments, and are no-ops without an event id', async () => {
  vi.mocked(getOccasionEvent).mockResolvedValue({ attendees: [] } as never);
  const none = renderHook(() => useOccasionEventAttendees(undefined));
  await act(async () => {
    await none.result.current.handleAdd('e1');
    await none.result.current.handleUpdateRsvp('u1', 'yes' as never);
    await none.result.current.handleRemove('u1');
  });
  expect(addOccasionEventAttendee).not.toHaveBeenCalled();
  expect(updateOccasionEventAttendee).not.toHaveBeenCalled();
  expect(removeOccasionEventAttendee).not.toHaveBeenCalled();

  const { result } = renderHook(() => useOccasionEventAttendees('ev-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));
  await act(async () => {
    await result.current.handleAdd('e1');
    await result.current.handleUpdateRsvp('u1', 'yes' as never);
    await result.current.handleRemove('u2');
  });
  expect(addOccasionEventAttendee).toHaveBeenCalledWith('ev-1', { entity_id: 'e1' });
  expect(updateOccasionEventAttendee).toHaveBeenCalledWith('ev-1', 'u1', 'yes');
  expect(removeOccasionEventAttendee).toHaveBeenCalledWith('ev-1', 'u2');
});

test('attendee mutations notify, log their operation, and rethrow; they use the latest event and notifier', async () => {
  vi.mocked(getOccasionEvent).mockResolvedValue({ attendees: [] } as never);
  vi.mocked(addOccasionEventAttendee).mockRejectedValue(new Error('a'));
  vi.mocked(updateOccasionEventAttendee).mockRejectedValue(new Error('r'));
  vi.mocked(removeOccasionEventAttendee).mockRejectedValue(new Error('x'));
  const first = vi.fn();
  const second = vi.fn();
  const { result, rerender } = renderHook(({ id, n }) => useOccasionEventAttendees(id, n), {
    initialProps: { id: 'ev-1', n: { showError: first } },
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
  rerender({ id: 'ev-2', n: { showError: second } });
  await act(async () => {
    await expect(result.current.handleAdd('e')).rejects.toThrow('a');
    await expect(result.current.handleUpdateRsvp('u', 'no' as never)).rejects.toThrow('r');
    await expect(result.current.handleRemove('u')).rejects.toThrow('x');
  });
  expect(first).not.toHaveBeenCalled();
  expect(second.mock.calls.map((c) => c[0])).toEqual(['a', 'r', 'x']);
  expect(addOccasionEventAttendee).toHaveBeenCalledWith('ev-2', { entity_id: 'e' });
  expect(console.error).toHaveBeenCalledWith('[adding event attendee] Error:', expect.anything());
  expect(console.error).toHaveBeenCalledWith('[updating attendee RSVP] Error:', expect.anything());
  expect(console.error).toHaveBeenCalledWith('[removing event attendee] Error:', expect.anything());
});

test('loadSuggestions fetches for the circles, toggles its spinner, and replaces earlier suggestions', async () => {
  vi.mocked(getOccasionEvent).mockResolvedValue({ attendees: [] } as never);
  const d = deferred<never>();
  vi.mocked(getInviteeSuggestions).mockReturnValue(d.promise);
  const { result } = renderHook(() => useOccasionEventAttendees('ev-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));
  act(() => {
    void result.current.loadSuggestions(['c1', 'c2']);
  });
  await waitFor(() => expect(result.current.suggestionsLoading).toBe(true));
  expect(getInviteeSuggestions).toHaveBeenCalledWith({ circleIds: ['c1', 'c2'], eventId: 'ev-1' });
  await act(async () => {
    d.resolve([{ entity_id: 's1' }] as never);
  });
  await waitFor(() => expect(result.current.suggestionsLoading).toBe(false));
  expect(result.current.suggestions).toEqual([{ entity_id: 's1' }]);

  // Empty circle set clears without calling the API again.
  vi.mocked(getInviteeSuggestions).mockClear();
  await act(async () => {
    await result.current.loadSuggestions([]);
  });
  expect(result.current.suggestions).toEqual([]);
  expect(getInviteeSuggestions).not.toHaveBeenCalled();
});

test('loadSuggestions without an event id clears and does not call the API', async () => {
  const { result } = renderHook(() => useOccasionEventAttendees(undefined));
  await act(async () => {
    await result.current.loadSuggestions(['c1']);
  });
  expect(getInviteeSuggestions).not.toHaveBeenCalled();
  expect(result.current.suggestions).toEqual([]);
});

test('a failed loadSuggestions notifies, logs, clears stale suggestions and stops the spinner', async () => {
  vi.mocked(getOccasionEvent).mockResolvedValue({ attendees: [] } as never);
  vi.mocked(getInviteeSuggestions)
    .mockResolvedValueOnce([{ entity_id: 's1' }] as never)
    .mockRejectedValueOnce(new Error('nope'));
  const showError = vi.fn();
  const { result } = renderHook(() => useOccasionEventAttendees('ev-1', { showError }));
  await waitFor(() => expect(result.current.loading).toBe(false));
  await act(async () => {
    await result.current.loadSuggestions(['c1']);
  });
  expect(result.current.suggestions).toHaveLength(1);
  await act(async () => {
    await result.current.loadSuggestions(['c1']);
  });
  expect(result.current.suggestions).toEqual([]);
  expect(result.current.suggestionsLoading).toBe(false);
  expect(showError).toHaveBeenCalledWith('nope');
  expect(console.error).toHaveBeenCalledWith(
    '[loading invitee suggestions] Error:',
    expect.anything(),
  );
});

test('loadSuggestions follows the current event id and notifier; clearSuggestions empties and is stable', async () => {
  vi.mocked(getOccasionEvent).mockResolvedValue({ attendees: [] } as never);
  vi.mocked(getInviteeSuggestions).mockResolvedValue([{ entity_id: 's' }] as never);
  const { result, rerender } = renderHook(({ id }) => useOccasionEventAttendees(id), {
    initialProps: { id: 'ev-1' },
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
  const clear = result.current.clearSuggestions;
  rerender({ id: 'ev-2' });
  await act(async () => {
    await result.current.loadSuggestions(['c']);
  });
  expect(getInviteeSuggestions).toHaveBeenLastCalledWith({ circleIds: ['c'], eventId: 'ev-2' });
  expect(result.current.suggestions).toHaveLength(1);
  act(() => result.current.clearSuggestions());
  expect(result.current.suggestions).toEqual([]);
  expect(result.current.clearSuggestions).toBe(clear);
});
