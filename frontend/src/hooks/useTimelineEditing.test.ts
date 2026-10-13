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

// --- behavior pins (issue #1662) ---

test('starts with no editor open and nothing preloaded', () => {
  const { result } = renderHook(() => useTimelineEditing(42, vi.fn()));
  expect(result.current.editingTimelineItem).toBeNull();
  expect(result.current.editTimelineValues).toEqual({});
  expect(result.current.allContacts).toEqual([]);
});

test('a note with no content or date opens with empty strings', async () => {
  const { result } = renderHook(() => useTimelineEditing(42, vi.fn()));
  await act(async () => {
    await result.current.handleStartEditTimelineItem('note', {
      ...note,
      content: undefined,
      date: undefined,
    } as unknown as Note);
  });
  expect(result.current.editTimelineValues).toEqual({ noteContent: '', noteDate: '' });
});

test('an activity with only a title opens with empty optional fields and no contacts', async () => {
  vi.mocked(getAllContacts).mockResolvedValue([]);
  const { result } = renderHook(() => useTimelineEditing(42, vi.fn()));
  await act(async () => {
    await result.current.handleStartEditTimelineItem('activity', {
      ID: 9,
      title: undefined,
    } as unknown as Activity);
  });
  expect(result.current.editTimelineValues).toEqual({
    activityTitle: '',
    activityDescription: '',
    activityLocation: '',
    activityDate: '',
    activityContacts: [],
  });
});

test('the autocomplete fetch asks for up to 1000 contacts, tolerates an empty answer, and logs a failure', async () => {
  vi.mocked(getAllContacts).mockResolvedValueOnce(null as never);
  const { result } = renderHook(() => useTimelineEditing(42, vi.fn()));
  await act(async () => {
    await result.current.handleStartEditTimelineItem('activity', activity);
  });
  expect(getAllContacts).toHaveBeenCalledWith({ limit: 1000 });
  expect(result.current.allContacts).toEqual([]);

  vi.mocked(getAllContacts).mockRejectedValueOnce(new Error('offline'));
  await act(async () => {
    await result.current.handleStartEditTimelineItem('activity', activity);
  });
  expect(console.error).toHaveBeenCalledWith(
    '[fetching contacts for autocomplete] Error:',
    expect.anything(),
  );
});

test('blank note content or activity title (whitespace only) is not saved', async () => {
  const { result } = renderHook(() => useTimelineEditing(42, vi.fn()));
  await act(async () => {
    result.current.setEditTimelineValues({ noteContent: '   ', activityTitle: '  \t' });
  });
  await act(async () => {
    await result.current.handleUpdateNote(1);
    await result.current.handleUpdateActivity(2);
  });
  expect(updateNote).not.toHaveBeenCalled();
  expect(updateActivity).not.toHaveBeenCalled();
});

test('a note saved without a date is stamped with the current time', async () => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date('2026-09-01T12:00:00Z'));
  vi.mocked(updateNote).mockResolvedValue(note);
  const { result } = renderHook(() => useTimelineEditing(42, vi.fn().mockResolvedValue(undefined)));
  await act(async () => {
    result.current.setEditTimelineValues({ noteContent: 'hi' });
  });
  await act(async () => {
    await result.current.handleUpdateNote(1);
  });
  vi.useRealTimers();
  expect(updateNote).toHaveBeenCalledWith(1, {
    content: 'hi',
    date: '2026-09-01T12:00:00.000Z',
    contact_id: 42,
  });
});

test('an activity save sends every field and the contact ids', async () => {
  vi.mocked(updateActivity).mockResolvedValue(activity);
  const { result } = renderHook(() => useTimelineEditing(42, vi.fn().mockResolvedValue(undefined)));
  await act(async () => {
    result.current.setEditTimelineValues({
      activityTitle: 'lunch',
      activityDescription: 'tacos',
      activityLocation: 'truck',
      activityDate: '2026-08-20',
      activityContacts: [
        { ID: 5, firstname: 'A', lastname: 'A' },
        { ID: 6, firstname: 'B', lastname: 'B' },
      ],
    });
  });
  await act(async () => {
    await result.current.handleUpdateActivity(2);
  });
  expect(updateActivity).toHaveBeenCalledWith(2, {
    title: 'lunch',
    description: 'tacos',
    location: 'truck',
    date: new Date('2026-08-20').toISOString(),
    contact_ids: [5, 6],
  });
});

test('an activity saved with only a title defaults description, location, date and contacts', async () => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date('2026-09-01T12:00:00Z'));
  vi.mocked(updateActivity).mockResolvedValue(activity);
  const { result } = renderHook(() => useTimelineEditing(42, vi.fn().mockResolvedValue(undefined)));
  await act(async () => {
    result.current.setEditTimelineValues({ activityTitle: 'solo' });
  });
  await act(async () => {
    await result.current.handleUpdateActivity(2);
  });
  vi.useRealTimers();
  expect(updateActivity).toHaveBeenCalledWith(2, {
    title: 'solo',
    description: '',
    location: '',
    date: '2026-09-01T12:00:00.000Z',
    contact_ids: [],
  });
});

test('each failing operation notifies, logs its own label, skips the refresh and keeps the editor open', async () => {
  vi.mocked(updateNote).mockRejectedValue(new Error('un'));
  vi.mocked(updateActivity).mockRejectedValue(new Error('ua'));
  vi.mocked(deleteNote).mockRejectedValue(new Error('dn'));
  vi.mocked(deleteActivity).mockRejectedValue(new Error('da'));
  const showError = vi.fn();
  const onRefresh = vi.fn().mockResolvedValue(undefined);
  const { result } = renderHook(() => useTimelineEditing(42, onRefresh, { showError }));
  await act(async () => {
    await result.current.handleStartEditTimelineItem('note', note);
  });
  await act(async () => {
    result.current.setEditTimelineValues({ noteContent: 'x', activityTitle: 'y' });
  });
  await act(async () => {
    await result.current.handleUpdateNote(1);
    await result.current.handleUpdateActivity(2);
    await result.current.handleDeleteNote(1);
    await result.current.handleDeleteActivity(2);
  });
  expect(showError.mock.calls.map((c) => c[0])).toEqual(['un', 'ua', 'dn', 'da']);
  for (const op of ['updating note', 'updating activity', 'deleting note', 'deleting activity']) {
    expect(console.error).toHaveBeenCalledWith(`[${op}] Error:`, expect.anything());
  }
  expect(onRefresh).not.toHaveBeenCalled();
  expect(result.current.editingTimelineItem).toEqual({ type: 'note', id: 1 });
});
