import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import {
  type ContactSubscription,
  createContactSubscription,
  deleteContactSubscription,
  getContactSubscriptions,
  syncContactSubscription,
  updateContactSubscription,
} from '../api/contactSubscriptions';
import ContactSyncSettings from './ContactSyncSettings';

// This codebase's vitest setup has no auto-cleanup and no globals: true.
afterEach(cleanup);

vi.mock('../api/contactSubscriptions', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/contactSubscriptions')>();
  return {
    ...actual,
    getContactSubscriptions: vi.fn(),
    createContactSubscription: vi.fn(),
    updateContactSubscription: vi.fn(),
    deleteContactSubscription: vi.fn(),
    syncContactSubscription: vi.fn(),
  };
});

function sub(overrides: Partial<ContactSubscription> = {}): ContactSubscription {
  return {
    id: 1,
    name: 'Personal',
    url: 'https://dav.example.com/addressbooks/me/contacts/',
    username: '',
    has_password: false,
    sync_enabled: true,
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
    pending_conflicts: 0,
    ...overrides,
  };
}

beforeEach(() => {
  vi.mocked(getContactSubscriptions).mockReset();
  vi.mocked(createContactSubscription).mockReset();
  vi.mocked(updateContactSubscription).mockReset();
  vi.mocked(deleteContactSubscription).mockReset();
  vi.mocked(syncContactSubscription).mockReset();
});

test('shows the empty state when no address books are connected', async () => {
  vi.mocked(getContactSubscriptions).mockResolvedValue([]);
  render(<ContactSyncSettings />);

  await waitFor(() =>
    expect(screen.getByText('No address books connected yet.')).toBeInTheDocument(),
  );
});

test('adding an address book creates it and triggers an immediate sync', async () => {
  vi.mocked(getContactSubscriptions).mockResolvedValue([]);
  const created = sub({ id: 5, name: 'Family', url: 'https://dav.example.com/ab/family/' });
  vi.mocked(createContactSubscription).mockResolvedValue(created);
  vi.mocked(syncContactSubscription).mockResolvedValue({
    message: 'ok',
    created: 4,
    updated: 0,
    archived: 0,
    skipped: 0,
  });

  render(<ContactSyncSettings />);
  await waitFor(() =>
    expect(screen.getByText('No address books connected yet.')).toBeInTheDocument(),
  );

  fireEvent.click(screen.getByRole('button', { name: /add address book/i }));
  fireEvent.change(screen.getByLabelText('Name *'), { target: { value: 'Family' } });
  fireEvent.change(screen.getByLabelText(/address book url/i), {
    target: { value: 'https://dav.example.com/ab/family/' },
  });
  fireEvent.click(screen.getByRole('button', { name: /^save$/i }));

  await waitFor(() =>
    expect(createContactSubscription).toHaveBeenCalledWith(
      expect.objectContaining({ name: 'Family' }),
    ),
  );
  await waitFor(() => expect(syncContactSubscription).toHaveBeenCalledWith(5));
  await waitFor(() => expect(screen.getByText(/sync complete: 4 created/i)).toBeInTheDocument());
});

test('editing an address book prefills the form and saves via update', async () => {
  vi.mocked(getContactSubscriptions).mockResolvedValue([sub({ id: 7, name: 'Personal' })]);
  vi.mocked(updateContactSubscription).mockResolvedValue(sub({ id: 7, name: 'Renamed' }));

  render(<ContactSyncSettings />);
  await waitFor(() => expect(screen.getByText('Personal')).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: /edit/i }));
  const nameInput = screen.getByLabelText('Name *') as HTMLInputElement;
  expect(nameInput.value).toBe('Personal');
  fireEvent.change(nameInput, { target: { value: 'Renamed' } });
  fireEvent.click(screen.getByRole('button', { name: /^save$/i }));

  await waitFor(() =>
    expect(updateContactSubscription).toHaveBeenCalledWith(
      7,
      expect.objectContaining({ name: 'Renamed' }),
    ),
  );
});

test('a sync failure surfaces the error instead of throwing', async () => {
  vi.mocked(getContactSubscriptions).mockResolvedValue([sub({ id: 2, name: 'Broken' })]);
  vi.mocked(syncContactSubscription).mockRejectedValue(new Error('address book not found'));

  render(<ContactSyncSettings />);
  await waitFor(() => expect(screen.getByText('Broken')).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: /sync now/i }));
  await waitFor(() => expect(screen.getByText('address book not found')).toBeInTheDocument());
});

test('surfaces a terminal-failure notice (INT-04) with the actionable message and staleness', async () => {
  const fortyDaysAgo = new Date(Date.now() - 40 * 86_400_000).toISOString();
  vi.mocked(getContactSubscriptions).mockResolvedValue([
    sub({
      name: 'Stale book',
      last_sync_status: 'error',
      consecutive_failures: 12,
      last_success_at: fortyDaysAgo,
      terminal_failure_at: '2026-07-20T00:00:00Z',
      terminal_reason: 'auth-expiry',
    }),
  ]);
  render(<ContactSyncSettings />);

  await waitFor(() => expect(screen.getByText('Stale book')).toBeInTheDocument());
  expect(screen.getByText('Sync stopped — action needed')).toBeInTheDocument();
  expect(screen.getByText(/password or token was rejected/i)).toBeInTheDocument();
  expect(screen.getByText(/Last successful sync: 40 days ago/)).toBeInTheDocument();
  // The generic "Sync failed" chip is suppressed while terminal.
  expect(screen.queryByText('Sync failed')).not.toBeInTheDocument();
});

test('a never-succeeded terminal subscription says so', async () => {
  vi.mocked(getContactSubscriptions).mockResolvedValue([
    sub({
      name: 'New book',
      last_success_at: null,
      terminal_failure_at: '2026-07-20T00:00:00Z',
      terminal_reason: 'remote-resource-deleted',
    }),
  ]);
  render(<ContactSyncSettings />);

  await waitFor(() => expect(screen.getByText('New book')).toBeInTheDocument());
  expect(screen.getByText('This subscription has never synced successfully.')).toBeInTheDocument();
});
