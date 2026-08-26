import { act, cleanup, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { type Activity, deleteActivity, updateActivity } from '../api/activities';
import { getAllContacts } from '../api/contacts';
import { deleteNote, type Note, updateNote } from '../api/notes';
import { useTimelineEditing } from './useTimelineEditing';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock('../api/contacts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/contacts')>();
  return { ...actual, getAllContacts: vi.fn() };
});

vi.mock('../api/notes', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/notes')>();
  return { ...actual, updateNote: vi.fn(), deleteNote: vi.fn() };
});

vi.mock('../api/activities', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/activities')>();
  return { ...actual, updateActivity: vi.fn(), deleteActivity: vi.fn() };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(getAllContacts).mockReset();
  vi.mocked(updateNote).mockReset();
  vi.mocked(deleteNote).mockReset();
  vi.mocked(updateActivity).mockReset();
  vi.mocked(deleteActivity).mockReset();
});

const note: Note = {
  ID: 1,
  content: 'hello',
  date: '2026-08-12T10:00:00Z',
  CreatedAt: '2026-08-12T10:00:00Z',
  UpdatedAt: '2026-08-12T10:00:00Z',
};

const contact = { ID: 5, firstname: 'Alice', lastname: 'A' };

const activity: Activity = {
  ID: 2,
  title: 'coffee',
  description: 'debrief',
  location: 'cafe',
  date: '2026-08-12T10:00:00Z',
  CreatedAt: '2026-08-12T10:00:00Z',
  UpdatedAt: '2026-08-12T10:00:00Z',
  contacts: [contact],
};

test('starts editing a note with its content and date', async () => {
  const { result } = renderHook(() => useTimelineEditing(42, vi.fn()));

  await act(async () => {
    await result.current.handleStartEditTimelineItem('note', note);
  });

  expect(result.current.editingTimelineItem).toEqual({ type: 'note', id: 1 });
  expect(result.current.editTimelineValues).toEqual({
    noteContent: 'hello',
    noteDate: '2026-08-12',
  });
  expect(getAllContacts).not.toHaveBeenCalled();
});

test('starts editing an activity and fetches contacts for the autocomplete', async () => {
  vi.mocked(getAllContacts).mockResolvedValue([contact]);
  const { result } = renderHook(() => useTimelineEditing(42, vi.fn()));

  await act(async () => {
    await result.current.handleStartEditTimelineItem('activity', activity);
  });

  expect(getAllContacts).toHaveBeenCalledWith({ limit: 1000 });
  expect(result.current.allContacts).toEqual([contact]);
  expect(result.current.editingTimelineItem).toEqual({ type: 'activity', id: 2 });
  expect(result.current.editTimelineValues).toEqual({
    activityTitle: 'coffee',
    activityDescription: 'debrief',
    activityLocation: 'cafe',
    activityDate: '2026-08-12',
    activityContacts: [contact],
  });
});

test('does not re-fetch contacts once loaded', async () => {
  vi.mocked(getAllContacts).mockResolvedValue([contact]);
  const { result } = renderHook(() => useTimelineEditing(42, vi.fn()));

  await act(async () => {
    await result.current.handleStartEditTimelineItem('activity', activity);
  });
  act(() => result.current.handleCancelEditTimelineItem());
  await act(async () => {
    await result.current.handleStartEditTimelineItem('activity', activity);
  });

  expect(getAllContacts).toHaveBeenCalledTimes(1);
});

test('handleCancelEditTimelineItem resets the editor', async () => {
  const { result } = renderHook(() => useTimelineEditing(42, vi.fn()));

  await act(async () => {
    await result.current.handleStartEditTimelineItem('note', note);
  });
  act(() => result.current.handleCancelEditTimelineItem());

  expect(result.current.editingTimelineItem).toBeNull();
  expect(result.current.editTimelineValues).toEqual({});
});

test('handleUpdateNote saves, refreshes and resets', async () => {
  vi.mocked(updateNote).mockResolvedValue({ ...note, content: 'updated' });
  const onRefresh = vi.fn().mockResolvedValue(undefined);
  const { result } = renderHook(() => useTimelineEditing(42, onRefresh));

  await act(async () => {
    await result.current.handleStartEditTimelineItem('note', note);
  });
  await act(async () => {
    await result.current.handleUpdateNote(1);
  });

  expect(updateNote).toHaveBeenCalledWith(1, {
    content: 'hello',
    date: new Date('2026-08-12').toISOString(),
    contact_id: 42,
  });
  expect(onRefresh).toHaveBeenCalledTimes(1);
  expect(result.current.editingTimelineItem).toBeNull();
  expect(result.current.editTimelineValues).toEqual({});
});

test('handleUpdateNote is a no-op with empty content', async () => {
  const onRefresh = vi.fn().mockResolvedValue(undefined);
  const { result } = renderHook(() => useTimelineEditing(42, onRefresh));

  await act(async () => {
    await result.current.handleUpdateNote(1);
  });

  expect(updateNote).not.toHaveBeenCalled();
  expect(onRefresh).not.toHaveBeenCalled();
});

test('handleUpdateActivity saves, refreshes and resets', async () => {
  vi.mocked(updateActivity).mockResolvedValue(activity);
  const onRefresh = vi.fn().mockResolvedValue(undefined);
  const { result } = renderHook(() => useTimelineEditing(42, onRefresh));

  await act(async () => {
    await result.current.handleStartEditTimelineItem('activity', activity);
  });
  await act(async () => {
    await result.current.handleUpdateActivity(2);
  });

  expect(updateActivity).toHaveBeenCalledWith(2, {
    title: 'coffee',
    description: 'debrief',
    location: 'cafe',
    date: new Date('2026-08-12').toISOString(),
    contact_ids: [5],
  });
  expect(onRefresh).toHaveBeenCalledTimes(1);
  expect(result.current.editingTimelineItem).toBeNull();
  expect(result.current.editTimelineValues).toEqual({});
});

test('handleUpdateActivity is a no-op with an empty title', async () => {
  const onRefresh = vi.fn().mockResolvedValue(undefined);
  const { result } = renderHook(() => useTimelineEditing(42, onRefresh));

  await act(async () => {
    await result.current.handleUpdateActivity(2);
  });

  expect(updateActivity).not.toHaveBeenCalled();
  expect(onRefresh).not.toHaveBeenCalled();
});

test('handleDeleteNote deletes, refreshes and resets', async () => {
  vi.mocked(deleteNote).mockResolvedValue(undefined);
  const onRefresh = vi.fn().mockResolvedValue(undefined);
  const { result } = renderHook(() => useTimelineEditing(42, onRefresh));

  await act(async () => {
    await result.current.handleStartEditTimelineItem('note', note);
  });
  await act(async () => {
    await result.current.handleDeleteNote(1);
  });

  expect(deleteNote).toHaveBeenCalledWith(1);
  expect(onRefresh).toHaveBeenCalledTimes(1);
  expect(result.current.editingTimelineItem).toBeNull();
});

test('handleDeleteActivity deletes, refreshes and resets', async () => {
  vi.mocked(deleteActivity).mockResolvedValue(undefined);
  const onRefresh = vi.fn().mockResolvedValue(undefined);
  const { result } = renderHook(() => useTimelineEditing(42, onRefresh));

  await act(async () => {
    await result.current.handleStartEditTimelineItem('activity', activity);
  });
  await act(async () => {
    await result.current.handleDeleteActivity(2);
  });

  expect(deleteActivity).toHaveBeenCalledWith(2);
  expect(onRefresh).toHaveBeenCalledTimes(1);
  expect(result.current.editingTimelineItem).toBeNull();
});

test('update errors notify through the notifier but keep the editor open', async () => {
  vi.mocked(updateNote).mockRejectedValue(new Error('boom'));
  const showError = vi.fn();
  const onRefresh = vi.fn().mockResolvedValue(undefined);
  const { result } = renderHook(() => useTimelineEditing(42, onRefresh, { showError }));

  await act(async () => {
    await result.current.handleStartEditTimelineItem('note', note);
  });
  await act(async () => {
    await result.current.handleUpdateNote(1);
  });

  expect(showError).toHaveBeenCalledWith('boom');
  expect(onRefresh).not.toHaveBeenCalled();
  expect(result.current.editingTimelineItem).toEqual({ type: 'note', id: 1 });
});

test('contact autocomplete fetch failures are swallowed', async () => {
  vi.mocked(getAllContacts).mockRejectedValue(new Error('boom'));
  const { result } = renderHook(() => useTimelineEditing(42, vi.fn()));

  await act(async () => {
    await result.current.handleStartEditTimelineItem('activity', activity);
  });

  expect(result.current.allContacts).toEqual([]);
  expect(result.current.editTimelineValues.activityTitle).toBe('coffee');
});
