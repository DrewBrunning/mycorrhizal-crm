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
    last_attempt_at: null,
    last_success_at: null,
    last_failure_at: null,
    consecutive_failures: 0,
    incident_first_failure_at: null,
    last_run_duration_ms: null,
    last_run_stats: {},
    terminal_failure_at: null,
    terminal_reason: '',
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

test('shows a standing-failure health line with the incident start and consecutive count', async () => {
  vi.mocked(getCalendarSubscriptions).mockResolvedValue([
    calendar({
      name: 'Flaky',
      last_sync_status: 'error',
      last_sync_error: 'auth failed',
      consecutive_failures: 9,
      incident_first_failure_at: '2026-08-27T14:03:00Z',
      last_success_at: '2026-08-26T17:04:00Z',
    }),
  ]);
  render(<CalendarSyncSettings />);

  await waitFor(() => expect(screen.getByText('Flaky')).toBeInTheDocument());
  // Chip switches to the counted form past the first failure.
  expect(screen.getByText('Sync failed ×9')).toBeInTheDocument();
  expect(screen.getByText(/9 consecutive failures/)).toBeInTheDocument();
  expect(screen.getByText(/last success/)).toBeInTheDocument();
});

test('shows the last run tallies for a healthy subscription', async () => {
  vi.mocked(getCalendarSubscriptions).mockResolvedValue([
    calendar({
      name: 'Healthy',
      last_sync_status: 'success',
      last_synced_at: '2026-08-27T09:00:00Z',
      last_run_stats: { created: 1, updated: 2, skipped: 3 },
    }),
  ]);
  render(<CalendarSyncSettings />);

  await waitFor(() => expect(screen.getByText('Healthy')).toBeInTheDocument());
  expect(screen.getByText(/1 created, 2 updated, 3 unchanged/)).toBeInTheDocument();
  expect(screen.queryByText(/consecutive failures/)).not.toBeInTheDocument();
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

test('surfaces a terminal-failure notice with an actionable message (INT-04)', async () => {
  vi.mocked(getCalendarSubscriptions).mockResolvedValue([
    calendar({
      name: 'Broken',
      last_sync_status: 'error',
      consecutive_failures: 9,
      last_success_at: null,
      terminal_failure_at: '2026-07-01T00:00:00Z',
      terminal_reason: 'auth-expiry',
    }),
  ]);
  render(<CalendarSyncSettings />);

  await waitFor(() => expect(screen.getByText('Broken')).toBeInTheDocument());
  expect(screen.getByText('Sync stopped — action needed')).toBeInTheDocument();
  expect(screen.getByText(/password or token was rejected/i)).toBeInTheDocument();
  expect(screen.getByText('This subscription has never synced successfully.')).toBeInTheDocument();
  // The generic "failing since" line is replaced by the terminal notice.
  expect(screen.queryByText(/consecutive failures/)).not.toBeInTheDocument();
});

test('terminal notice shows a staleness age when a past success exists', async () => {
  const fortyDaysAgo = new Date(Date.now() - 40 * 86_400_000).toISOString();
  vi.mocked(getCalendarSubscriptions).mockResolvedValue([
    calendar({
      name: 'Stale',
      last_success_at: fortyDaysAgo,
      terminal_failure_at: '2026-07-01T00:00:00Z',
      terminal_reason: 'remote-resource-deleted',
    }),
  ]);
  render(<CalendarSyncSettings />);

  await waitFor(() => expect(screen.getByText('Stale')).toBeInTheDocument());
  expect(screen.getByText(/no longer exists/i)).toBeInTheDocument();
  expect(screen.getByText(/Last successful sync: 40 days ago/)).toBeInTheDocument();
});
