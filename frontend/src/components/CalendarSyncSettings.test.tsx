import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import {
  type CalendarSubscription,
  createCalendarSubscription,
  deleteCalendarSubscription,
  getCalendarSubscriptions,
  syncCalendarSubscription,
  updateCalendarSubscription,
} from '../api/calendars';
import CalendarSyncSettings from './CalendarSyncSettings';

// This codebase's vitest setup has no auto-cleanup and no globals: true.
afterEach(cleanup);

vi.mock('../api/calendars', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/calendars')>();
  return {
    ...actual,
    getCalendarSubscriptions: vi.fn(),
    createCalendarSubscription: vi.fn(),
    updateCalendarSubscription: vi.fn(),
    deleteCalendarSubscription: vi.fn(),
    syncCalendarSubscription: vi.fn(),
  };
});

function calendar(overrides: Partial<CalendarSubscription> = {}): CalendarSubscription {
  return {
    id: 1,
    name: 'Personal',
    url: 'https://cloud.example.com/dav/calendars/user/personal/',
    username: '',
    has_password: false,
    sync_enabled: true,
    past_days: 5,
    future_days: 10,
    last_synced_at: null,
    last_sync_status: '',
    last_sync_error: '',
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

beforeEach(() => {
  vi.mocked(getCalendarSubscriptions).mockReset();
  vi.mocked(createCalendarSubscription).mockReset();
  vi.mocked(updateCalendarSubscription).mockReset();
  vi.mocked(deleteCalendarSubscription).mockReset();
  vi.mocked(syncCalendarSubscription).mockReset();
});

test('shows the empty state when no calendars are connected', async () => {
  vi.mocked(getCalendarSubscriptions).mockResolvedValue([]);
  render(<CalendarSyncSettings />);

  await waitFor(() => expect(screen.getByText('No calendars connected yet.')).toBeInTheDocument());
});

test('lists an existing calendar with its sync status', async () => {
  vi.mocked(getCalendarSubscriptions).mockResolvedValue([
    calendar({ name: 'Work', last_sync_status: 'error', last_sync_error: 'auth failed' }),
  ]);
  render(<CalendarSyncSettings />);

  await waitFor(() => expect(screen.getByText('Work')).toBeInTheDocument());
  expect(screen.getByText('Sync failed')).toBeInTheDocument();
});

test('adding a calendar creates it and triggers an immediate sync', async () => {
  vi.mocked(getCalendarSubscriptions).mockResolvedValue([]);
  const created = calendar({ id: 5, name: 'New Calendar', url: 'https://example.com/cal/' });
  vi.mocked(createCalendarSubscription).mockResolvedValue(created);
  vi.mocked(syncCalendarSubscription).mockResolvedValue({
    message: 'ok',
    created: 3,
    updated: 0,
    skipped: 0,
  });

  render(<CalendarSyncSettings />);
  await waitFor(() => expect(screen.getByText('No calendars connected yet.')).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: /add calendar/i }));
  fireEvent.change(screen.getByLabelText('Name *'), { target: { value: 'New Calendar' } });
  fireEvent.change(screen.getByLabelText(/calendar url/i), {
    target: { value: 'https://example.com/cal/' },
  });
  fireEvent.click(screen.getByRole('button', { name: /^save$/i }));

  await waitFor(() =>
    expect(createCalendarSubscription).toHaveBeenCalledWith(
      expect.objectContaining({ name: 'New Calendar', url: 'https://example.com/cal/' }),
    ),
  );
  await waitFor(() => expect(syncCalendarSubscription).toHaveBeenCalledWith(5));
  await waitFor(() => expect(screen.getByText(/sync complete: 3 created/i)).toBeInTheDocument());
});

test('editing a calendar prefills the form and saves via update', async () => {
  vi.mocked(getCalendarSubscriptions).mockResolvedValue([calendar({ id: 7, name: 'Personal' })]);
  vi.mocked(updateCalendarSubscription).mockResolvedValue(calendar({ id: 7, name: 'Renamed' }));

  render(<CalendarSyncSettings />);
  await waitFor(() => expect(screen.getByText('Personal')).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: /edit/i }));
  const nameInput = screen.getByLabelText('Name *') as HTMLInputElement;
  expect(nameInput.value).toBe('Personal');

  fireEvent.change(nameInput, { target: { value: 'Renamed' } });
  fireEvent.click(screen.getByRole('button', { name: /^save$/i }));

  await waitFor(() =>
    expect(updateCalendarSubscription).toHaveBeenCalledWith(
      7,
      expect.objectContaining({ name: 'Renamed' }),
    ),
  );
});

test('deleting a calendar asks for confirmation before removing it', async () => {
  vi.mocked(getCalendarSubscriptions).mockResolvedValue([calendar({ id: 3, name: 'To Delete' })]);
  vi.mocked(deleteCalendarSubscription).mockResolvedValue(undefined);

  render(<CalendarSyncSettings />);
  await waitFor(() => expect(screen.getByText('To Delete')).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: /delete/i }));
  expect(screen.getByText('Delete calendar?')).toBeInTheDocument();
  expect(deleteCalendarSubscription).not.toHaveBeenCalled();

  // The confirm dialog's own Delete button (there are now two "Delete" buttons on screen).
  fireEvent.click(screen.getAllByRole('button', { name: /^delete$/i })[0]);

  await waitFor(() => expect(deleteCalendarSubscription).toHaveBeenCalledWith(3));
  await waitFor(() => expect(screen.queryByText('To Delete')).not.toBeInTheDocument());
});

test('manual sync reports the result and reloads the list', async () => {
  vi.mocked(getCalendarSubscriptions).mockResolvedValue([
    calendar({ id: 9, name: 'Manual Sync Cal' }),
  ]);
  vi.mocked(syncCalendarSubscription).mockResolvedValue({
    message: 'ok',
    created: 0,
    updated: 2,
    skipped: 1,
  });

  render(<CalendarSyncSettings />);
  await waitFor(() => expect(screen.getByText('Manual Sync Cal')).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: /sync now/i }));

  await waitFor(() => expect(syncCalendarSubscription).toHaveBeenCalledWith(9));
  await waitFor(() =>
    expect(screen.getByText(/sync complete: 0 created, 2 updated/i)).toBeInTheDocument(),
  );
});

test('a sync failure surfaces the error instead of throwing', async () => {
  vi.mocked(getCalendarSubscriptions).mockResolvedValue([calendar({ id: 2, name: 'Broken Cal' })]);
  vi.mocked(syncCalendarSubscription).mockRejectedValue(new Error('calendar unreachable'));

  render(<CalendarSyncSettings />);
  await waitFor(() => expect(screen.getByText('Broken Cal')).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: /sync now/i }));

  await waitFor(() => expect(screen.getByText('calendar unreachable')).toBeInTheDocument());
});

test('warns when an http:// URL is paired with credentials', async () => {
  vi.mocked(getCalendarSubscriptions).mockResolvedValue([]);
  render(<CalendarSyncSettings />);
  await waitFor(() => expect(screen.getByText('No calendars connected yet.')).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: /add calendar/i }));
  fireEvent.change(screen.getByLabelText(/calendar url/i), {
    target: { value: 'http://example.com/cal/' },
  });

  expect(screen.queryByText(/unencrypted http/i)).not.toBeInTheDocument();

  fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'alice' } });

  expect(screen.getByText(/unencrypted http/i)).toBeInTheDocument();
});
