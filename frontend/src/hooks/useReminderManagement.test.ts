import { act, cleanup, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import {
  completeReminder,
  createReminder,
  deleteReminder,
  getRemindersForContact,
  type Reminder,
  type ReminderFormData,
  updateReminder,
} from '../api/reminders';
import { useReminderManagement } from './useReminderManagement';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock('../api/reminders', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/reminders')>();
  return {
    ...actual,
    getRemindersForContact: vi.fn(),
    createReminder: vi.fn(),
    updateReminder: vi.fn(),
    deleteReminder: vi.fn(),
    completeReminder: vi.fn(),
  };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(getRemindersForContact).mockReset();
  vi.mocked(createReminder).mockReset();
  vi.mocked(updateReminder).mockReset();
  vi.mocked(deleteReminder).mockReset();
  vi.mocked(completeReminder).mockReset();
});

function reminder(id: number, message: string): Reminder {
  return {
    ID: id,
    message,
    by_mail: false,
    remind_at: '2026-08-20T09:00:00Z',
    recurrence: 'once',
    reoccur_from_completion: false,
    completed: false,
    email_sent: false,
    contact_id: 1,
  };
}

function formData(overrides: Partial<ReminderFormData> = {}): ReminderFormData {
  return {
    message: 'call Ada',
    by_mail: false,
    remind_at: '2026-08-20T09:00:00Z',
    recurrence: 'once',
    reoccur_from_completion: false,
    contact_id: 1,
    ...overrides,
  };
}

test('refreshReminders loads reminders for the contact', async () => {
  vi.mocked(getRemindersForContact).mockResolvedValue([reminder(1, 'one'), reminder(2, 'two')]);

  const { result } = renderHook(() => useReminderManagement('7'));
  await act(async () => {
    await result.current.refreshReminders();
  });

  expect(getRemindersForContact).toHaveBeenCalledWith(7);
  expect(result.current.reminders.map((r) => r.ID)).toEqual([1, 2]);
  expect(result.current.error).toBeNull();
});

test('refreshReminders is a no-op without a contact id', async () => {
  const { result } = renderHook(() => useReminderManagement(undefined));
  await act(async () => {
    await result.current.refreshReminders();
  });

  expect(getRemindersForContact).not.toHaveBeenCalled();
});

test('refreshReminders sets an error when the fetch fails', async () => {
  vi.mocked(getRemindersForContact).mockRejectedValue(new Error('boom'));

  const { result } = renderHook(() => useReminderManagement('7'));
  await act(async () => {
    await result.current.refreshReminders();
  });

  expect(result.current.error).toBe('boom');
  expect(result.current.reminders).toEqual([]);
});

test('handleSaveReminder creates a new reminder when not editing', async () => {
  vi.mocked(createReminder).mockResolvedValue(reminder(1, 'call Ada'));
  vi.mocked(getRemindersForContact).mockResolvedValue([reminder(1, 'call Ada')]);

  const { result } = renderHook(() => useReminderManagement('7'));
  await act(async () => {
    await result.current.handleSaveReminder(formData());
  });

  expect(createReminder).toHaveBeenCalledWith(7, formData());
  expect(updateReminder).not.toHaveBeenCalled();
  expect(result.current.reminders.map((r) => r.ID)).toEqual([1]);
  expect(result.current.reminderDialogOpen).toBe(false);
});

test('handleSaveReminder updates when editing an existing reminder', async () => {
  vi.mocked(updateReminder).mockResolvedValue(reminder(1, 'edited'));
  vi.mocked(getRemindersForContact).mockResolvedValue([reminder(1, 'edited')]);

  const { result } = renderHook(() => useReminderManagement('7'));
  act(() => {
    result.current.handleEditReminder(reminder(1, 'old'));
  });
  expect(result.current.reminderDialogOpen).toBe(true);
  expect(result.current.editingReminder?.ID).toBe(1);

  await act(async () => {
    await result.current.handleSaveReminder(formData({ message: 'edited' }));
  });

  expect(updateReminder).toHaveBeenCalledWith(1, formData({ message: 'edited' }));
  expect(createReminder).not.toHaveBeenCalled();
  expect(result.current.reminders.map((r) => r.ID)).toEqual([1]);
  expect(result.current.editingReminder).toBeNull();
  expect(result.current.reminderDialogOpen).toBe(false);
});

test('handleSaveReminder is a no-op without a contact id', async () => {
  const { result } = renderHook(() => useReminderManagement(undefined));
  await act(async () => {
    await result.current.handleSaveReminder(formData());
  });

  expect(createReminder).not.toHaveBeenCalled();
  expect(updateReminder).not.toHaveBeenCalled();
});

test('handleSaveReminder rethrows and notifies on failure', async () => {
  const notifier = { showError: vi.fn() };
  vi.mocked(createReminder).mockRejectedValue(new Error('save failed'));

  const { result } = renderHook(() => useReminderManagement('7', notifier));
  await expect(
    act(async () => {
      await result.current.handleSaveReminder(formData());
    }),
  ).rejects.toThrow('save failed');

  expect(notifier.showError).toHaveBeenCalledWith('save failed');
});

test('handleCompleteReminder completes and refreshes', async () => {
  vi.mocked(completeReminder).mockResolvedValue({ message: 'completed' });
  vi.mocked(getRemindersForContact).mockResolvedValue([reminder(1, 'one'), reminder(2, 'two')]);

  const { result } = renderHook(() => useReminderManagement('7'));
  await act(async () => {
    await result.current.refreshReminders();
  });
  expect(result.current.reminders).toHaveLength(2);

  await act(async () => {
    await result.current.handleCompleteReminder(1);
  });

  expect(completeReminder).toHaveBeenCalledWith(1);
  expect(getRemindersForContact).toHaveBeenCalledTimes(2);
});

test('handleCompleteReminder rethrows on failure', async () => {
  vi.mocked(completeReminder).mockRejectedValue(new Error('complete failed'));

  const { result } = renderHook(() => useReminderManagement('7'));
  await expect(
    act(async () => {
      await result.current.handleCompleteReminder(1);
    }),
  ).rejects.toThrow('complete failed');
});

test('handleDeleteReminder deletes and refreshes', async () => {
  vi.mocked(deleteReminder).mockResolvedValue(undefined);
  vi.mocked(getRemindersForContact).mockResolvedValue([reminder(2, 'remaining')]);

  const { result } = renderHook(() => useReminderManagement('7'));
  await act(async () => {
    await result.current.refreshReminders();
  });

  await act(async () => {
    await result.current.handleDeleteReminder(1);
  });

  expect(deleteReminder).toHaveBeenCalledWith(1);
  expect(result.current.reminders.map((r) => r.ID)).toEqual([2]);
});

test('handleDeleteReminder rethrows on failure', async () => {
  vi.mocked(deleteReminder).mockRejectedValue(new Error('delete failed'));

  const { result } = renderHook(() => useReminderManagement('7'));
  await expect(
    act(async () => {
      await result.current.handleDeleteReminder(1);
    }),
  ).rejects.toThrow('delete failed');
});

test('handleAddReminder opens the dialog in create mode', async () => {
  const { result } = renderHook(() => useReminderManagement('7'));
  act(() => {
    result.current.handleAddReminder();
  });

  expect(result.current.reminderDialogOpen).toBe(true);
  expect(result.current.editingReminder).toBeNull();
});

test('setReminderDialogOpen and setReminderEditing expose setters', async () => {
  const { result } = renderHook(() => useReminderManagement('7'));

  act(() => {
    result.current.setReminderDialogOpen(true);
  });
  expect(result.current.reminderDialogOpen).toBe(true);

  act(() => {
    result.current.setEditingReminder(reminder(9, 'nine'));
  });
  expect(result.current.editingReminder?.ID).toBe(9);
});
