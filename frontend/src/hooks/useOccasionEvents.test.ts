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
