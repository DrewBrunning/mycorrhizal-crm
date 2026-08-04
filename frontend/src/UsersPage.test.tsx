import { test, expect, vi, afterEach, beforeEach } from 'vitest';
import { render, screen, cleanup, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import './i18n/config';
import UsersPage from './UsersPage';
import { SnackbarProvider } from './context/SnackbarContext';
import { getUsers, updateUser, deleteUser, triggerReminders } from './api/admin';
import { isAdmin } from './auth';
import type { User } from './types';

// This codebase's vitest setup has no auto-cleanup and no globals: true.
afterEach(cleanup);

vi.mock('./api/admin', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/admin')>();
  return {
    ...actual,
    getUsers: vi.fn(),
    updateUser: vi.fn(),
    deleteUser: vi.fn(),
    triggerReminders: vi.fn(),
  };
});
vi.mock('./auth', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./auth')>();
  return { ...actual, isAdmin: vi.fn() };
});

function user(overrides: Partial<User> = {}): User {
  return {
    id: 1,
    username: 'alice',
    email: 'alice@example.com',
    language: 'en',
    is_admin: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

beforeEach(() => {
  vi.mocked(getUsers).mockReset();
  vi.mocked(updateUser).mockReset();
  vi.mocked(deleteUser).mockReset();
  vi.mocked(triggerReminders).mockReset();
  vi.mocked(isAdmin).mockReset();
  vi.mocked(isAdmin).mockReturnValue(true);
});

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/users']}>
      <SnackbarProvider>
        <Routes>
          <Route path="/users" element={<UsersPage />} />
          <Route path="/" element={<div>DASHBOARD PAGE</div>} />
        </Routes>
      </SnackbarProvider>
    </MemoryRouter>
  );
}

test('a non-admin visiting the page is redirected away', async () => {
  vi.mocked(isAdmin).mockReturnValue(false);
  vi.mocked(getUsers).mockResolvedValue({ users: [], total: 0, page: 1, limit: 25, total_pages: 0 });

  renderPage();

  await waitFor(() => expect(screen.getByText('DASHBOARD PAGE')).toBeInTheDocument());
});

test('lists users with their role', async () => {
  vi.mocked(getUsers).mockResolvedValue({
    users: [user({ username: 'root-admin', is_admin: true }), user({ id: 2, username: 'plain-user' })],
    total: 2,
    page: 1,
    limit: 25,
    total_pages: 1,
  });

  renderPage();

  await waitFor(() => expect(screen.getByText('root-admin')).toBeInTheDocument());
  expect(screen.getByText('plain-user')).toBeInTheDocument();
  expect(screen.getByText('Admin')).toBeInTheDocument();
  expect(screen.getByText('User')).toBeInTheDocument();
});

test('shows the empty state when there are no users', async () => {
  vi.mocked(getUsers).mockResolvedValue({ users: [], total: 0, page: 1, limit: 25, total_pages: 0 });
  renderPage();

  await waitFor(() => expect(screen.getByText('No users found')).toBeInTheDocument());
});

test('a load failure surfaces the error', async () => {
  vi.mocked(getUsers).mockRejectedValue(new Error('network down'));
  renderPage();

  await waitFor(() => expect(screen.getByText('network down')).toBeInTheDocument());
});

test('editing a user prefills the form and only submits changed fields', async () => {
  vi.mocked(getUsers).mockResolvedValue({
    users: [user({ id: 7, username: 'original', email: 'original@example.com' })],
    total: 1,
    page: 1,
    limit: 25,
    total_pages: 1,
  });
  vi.mocked(updateUser).mockResolvedValue(user({ id: 7, username: 'renamed', email: 'original@example.com' }));

  renderPage();
  await waitFor(() => expect(screen.getByText('original')).toBeInTheDocument());

  fireEvent.click(screen.getByTitle('Edit'));
  const usernameInput = screen.getByLabelText('Username') as HTMLInputElement;
  expect(usernameInput.value).toBe('original');

  fireEvent.change(usernameInput, { target: { value: 'renamed' } });
  fireEvent.click(screen.getByRole('button', { name: /^save$/i }));

  await waitFor(() => expect(updateUser).toHaveBeenCalledWith(7, { username: 'renamed' }));
});

test('promoting a user to admin includes is_admin in the update', async () => {
  vi.mocked(getUsers).mockResolvedValue({
    users: [user({ id: 9, username: 'future-admin', is_admin: false })],
    total: 1,
    page: 1,
    limit: 25,
    total_pages: 1,
  });
  vi.mocked(updateUser).mockResolvedValue(user({ id: 9, username: 'future-admin', is_admin: true }));

  renderPage();
  await waitFor(() => expect(screen.getByText('future-admin')).toBeInTheDocument());

  fireEvent.click(screen.getByTitle('Edit'));
  fireEvent.click(screen.getByLabelText('Administrator'));
  fireEvent.click(screen.getByRole('button', { name: /^save$/i }));

  await waitFor(() => expect(updateUser).toHaveBeenCalledWith(9, { is_admin: true }));
});

test('deleting a user requires confirmation and warns it is permanent', async () => {
  vi.mocked(getUsers).mockResolvedValue({
    users: [user({ id: 4, username: 'to-delete' })],
    total: 1,
    page: 1,
    limit: 25,
    total_pages: 1,
  });
  vi.mocked(deleteUser).mockResolvedValue(undefined);

  renderPage();
  await waitFor(() => expect(screen.getByText('to-delete')).toBeInTheDocument());

  fireEvent.click(screen.getByTitle('Delete'));
  expect(deleteUser).not.toHaveBeenCalled();
  expect(screen.getByText(/permanently delete the user/i)).toBeInTheDocument();

  fireEvent.click(screen.getAllByRole('button', { name: /^delete$/i })[0]);

  await waitFor(() => expect(deleteUser).toHaveBeenCalledWith(4));
  await waitFor(() => expect(screen.queryByText('to-delete')).not.toBeInTheDocument());
});

test('a failed delete surfaces the error and keeps the user listed', async () => {
  vi.mocked(getUsers).mockResolvedValue({
    users: [user({ id: 5, username: 'stubborn' })],
    total: 1,
    page: 1,
    limit: 25,
    total_pages: 1,
  });
  vi.mocked(deleteUser).mockRejectedValue(new Error('cannot delete the last admin'));

  renderPage();
  await waitFor(() => expect(screen.getByText('stubborn')).toBeInTheDocument());

  fireEvent.click(screen.getByTitle('Delete'));
  fireEvent.click(screen.getAllByRole('button', { name: /^delete$/i })[0]);

  await waitFor(() => expect(screen.getByText('cannot delete the last admin')).toBeInTheDocument());
  expect(screen.getByText('stubborn')).toBeInTheDocument();
});

test('triggering reminder emails calls the endpoint', async () => {
  vi.mocked(getUsers).mockResolvedValue({ users: [], total: 0, page: 1, limit: 25, total_pages: 0 });
  vi.mocked(triggerReminders).mockResolvedValue(undefined);

  renderPage();
  await waitFor(() => expect(screen.getByText('No users found')).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: /send reminder emails/i }));

  await waitFor(() => expect(triggerReminders).toHaveBeenCalled());
});
