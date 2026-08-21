import { test, expect, vi, afterEach, beforeEach } from 'vitest';
import { renderHook, cleanup, act } from '@testing-library/react';
import { useContactDialogs } from './useContactDialogs';
import { createNote } from '../api/notes';
import { createActivity } from '../api/activities';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock('../api/notes', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/notes')>();
  return { ...actual, createNote: vi.fn() };
});

vi.mock('../api/activities', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/activities')>();
  return { ...actual, createActivity: vi.fn() };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(createNote).mockReset();
  vi.mocked(createActivity).mockReset();
});

test('dialogs start closed', () => {
  const { result } = renderHook(() => useContactDialogs('42', vi.fn()));
  expect(result.current.noteDialogOpen).toBe(false);
  expect(result.current.activityDialogOpen).toBe(false);
});

test('setters toggle the dialogs', () => {
  const { result } = renderHook(() => useContactDialogs('42', vi.fn()));
  act(() => result.current.setNoteDialogOpen(true));
  expect(result.current.noteDialogOpen).toBe(true);
  act(() => result.current.setActivityDialogOpen(true));
  expect(result.current.activityDialogOpen).toBe(true);
});

test('handleSaveNote creates the note with an ISO date and refreshes', async () => {
  vi.mocked(createNote).mockResolvedValue({
    ID: 1,
    content: 'hello',
    date: '2026-08-12T10:00:00Z',
    CreatedAt: '2026-08-12T10:00:00Z',
    UpdatedAt: '2026-08-12T10:00:00Z',
  });
  const onRefresh = vi.fn().mockResolvedValue(undefined);

  const { result } = renderHook(() => useContactDialogs('42', onRefresh));
  await act(async () => {
    await result.current.handleSaveNote('hello', '2026-08-12T10:00:00Z');
  });

  expect(createNote).toHaveBeenCalledWith('42', {
    content: 'hello',
    date: new Date('2026-08-12T10:00:00Z').toISOString(),
    contact_id: 42,
  });
  expect(onRefresh).toHaveBeenCalledTimes(1);
});

test('handleSaveNote does nothing without a contact id', async () => {
  const onRefresh = vi.fn().mockResolvedValue(undefined);

  const { result } = renderHook(() => useContactDialogs(undefined, onRefresh));
  await act(async () => {
    await result.current.handleSaveNote('hello', '2026-08-12T10:00:00Z');
  });

  expect(createNote).not.toHaveBeenCalled();
  expect(onRefresh).not.toHaveBeenCalled();
});

test('handleSaveNote errors notify through the notifier and rethrow', async () => {
  vi.mocked(createNote).mockRejectedValue(new Error('boom'));
  const showError = vi.fn();

  const { result } = renderHook(() => useContactDialogs('42', vi.fn(), { showError }));
  await expect(
    result.current.handleSaveNote('hello', '2026-08-12T10:00:00Z')
  ).rejects.toThrow('boom');
  expect(showError).toHaveBeenCalledWith('boom');
});

test('handleSaveActivity creates the activity with its contacts and refreshes', async () => {
  vi.mocked(createActivity).mockResolvedValue({
    ID: 1,
    title: 'coffee',
    date: '2026-08-12T10:00:00Z',
    CreatedAt: '2026-08-12T10:00:00Z',
    UpdatedAt: '2026-08-12T10:00:00Z',
  });
  const onRefresh = vi.fn().mockResolvedValue(undefined);

  const { result } = renderHook(() => useContactDialogs('42', onRefresh));
  await act(async () => {
    await result.current.handleSaveActivity({
      title: 'coffee',
      description: 'debrief',
      location: 'cafe',
      date: '2026-08-12T10:00:00Z',
      contact_ids: [42],
    });
  });

  expect(createActivity).toHaveBeenCalledWith({
    title: 'coffee',
    description: 'debrief',
    location: 'cafe',
    date: new Date('2026-08-12T10:00:00Z').toISOString(),
    contact_ids: [42],
  });
  expect(onRefresh).toHaveBeenCalledTimes(1);
});

test('handleSaveActivity errors notify through the notifier and rethrow', async () => {
  vi.mocked(createActivity).mockRejectedValue(new Error('boom'));
  const showError = vi.fn();

  const { result } = renderHook(() => useContactDialogs('42', vi.fn(), { showError }));
  await expect(
    result.current.handleSaveActivity({
      title: 'coffee',
      description: '',
      location: '',
      date: '2026-08-12T10:00:00Z',
      contact_ids: [42],
    })
  ).rejects.toThrow('boom');
  expect(showError).toHaveBeenCalledWith('boom');
});
