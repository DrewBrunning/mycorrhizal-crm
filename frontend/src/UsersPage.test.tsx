import { test, expect, vi, afterEach, beforeEach } from 'vitest';
import { render, screen, cleanup, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter, Routes, Route } from 'react-router';
import './i18n/config';
import UsersPage from './UsersPage';
import { SnackbarProvider } from './context/SnackbarContext';
import { getUsers, createUser, updateUser, deleteUser, triggerReminders } from './api/admin';
import { isAdmin } from './auth';
import type { User } from './types';

// This codebase's vitest setup has no auto-cleanup and no globals: true.
afterEach(cleanup);

vi.mock('./api/admin', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/admin')>();
  return {
    ...actual,
    getUsers: vi.fn(),
    createUser: vi.fn(),
    updateUser: vi.fn(),
    deleteUser: vi.fn(),
    triggerReminders: vi.fn(),
  };
});
vi.mock('./auth', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./auth')>();
  return { ...actual, isAdmin: vi.fn() };
});

// MUI's useMediaQuery needs window.matchMedia; jsdom provides none, so tests
// must install it. Default to `false` (desktop/table layout); the T32 stacked
// test flips it to simulate a phone-width viewport.
function mockMatchMedia(matches: boolean) {
  window.matchMedia = vi.fn().mockImplementation(() => ({
    matches,
    media: '',
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  }));
}

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
  mockMatchMedia(false);
  vi.mocked(getUsers).mockReset();
  vi.mocked(createUser).mockReset();
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

test('reflows to stacked user cards below the sm breakpoint (T32)', async () => {
  mockMatchMedia(true);
  vi.mocked(getUsers).mockResolvedValue({
    users: [
      user({ username: 'card-admin', email: 'admin@example.com', is_admin: true }),
      user({ id: 3, username: 'card-user', email: 'a-very-long-address@example.com' }),
    ],
    total: 2,
    page: 1,
    limit: 25,
    total_pages: 1,
  });

  renderPage();

  await waitFor(() => expect(screen.getByText('card-admin')).toBeInTheDocument());
  // No table at phone width — each user is a card, and both the edit and
  // delete actions remain reachable.
  expect(screen.queryByRole('table')).toBeNull();
  expect(screen.getByText('admin@example.com')).toBeInTheDocument();
  expect(screen.getAllByTitle('Edit')).toHaveLength(2);
  expect(screen.getAllByTitle('Delete')).toHaveLength(2);
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

test('creating a user submits the form, refetches the current page, and shows the new user', async () => {
  // Initial load is empty; after create, the page is refetched (not spliced
  // locally — the list is server-paginated) and now includes the new user.
  vi.mocked(getUsers)
    .mockResolvedValueOnce({ users: [], total: 0, page: 1, limit: 25, total_pages: 0 })
    .mockResolvedValueOnce({
      users: [user({ id: 11, username: 'brand-new', email: 'brand-new@example.com' })],
      total: 1,
      page: 1,
      limit: 25,
      total_pages: 1,
    });
  vi.mocked(createUser).mockResolvedValue(
    user({ id: 11, username: 'brand-new', email: 'brand-new@example.com' })
  );

  renderPage();
  await waitFor(() => expect(screen.getByText('No users found')).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: /add user/i }));

  fireEvent.change(screen.getByLabelText('Username *'), { target: { value: 'brand-new' } });
  fireEvent.change(screen.getByLabelText('Email *'), { target: { value: 'brand-new@example.com' } });
  fireEvent.change(screen.getByLabelText('Password *'), { target: { value: 'brandNewPassw0rd!' } });
  fireEvent.click(screen.getByRole('button', { name: /^save$/i }));

  await waitFor(() =>
    expect(createUser).toHaveBeenCalledWith({
      username: 'brand-new',
      email: 'brand-new@example.com',
      password: 'brandNewPassw0rd!',
      is_admin: false,
    })
  );
  await waitFor(() => expect(screen.getByText('brand-new')).toBeInTheDocument());
  // The refetch, not a local splice, is what supplied the row above.
  expect(getUsers).toHaveBeenCalledTimes(2);
});

test('creating an admin user includes is_admin in the create payload', async () => {
  vi.mocked(getUsers)
    .mockResolvedValueOnce({ users: [], total: 0, page: 1, limit: 25, total_pages: 0 })
    .mockResolvedValueOnce({
      users: [user({ id: 12, username: 'new-admin', email: 'new-admin@example.com', is_admin: true })],
      total: 1,
      page: 1,
      limit: 25,
      total_pages: 1,
    });
  vi.mocked(createUser).mockResolvedValue(
    user({ id: 12, username: 'new-admin', email: 'new-admin@example.com', is_admin: true })
  );

  renderPage();
  await waitFor(() => expect(screen.getByText('No users found')).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: /add user/i }));

  fireEvent.change(screen.getByLabelText('Username *'), { target: { value: 'new-admin' } });
  fireEvent.change(screen.getByLabelText('Email *'), { target: { value: 'new-admin@example.com' } });
  fireEvent.change(screen.getByLabelText('Password *'), { target: { value: 'brandNewPassw0rd!' } });
  fireEvent.click(screen.getByLabelText('Administrator'));
  fireEvent.click(screen.getByRole('button', { name: /^save$/i }));

  await waitFor(() =>
    expect(createUser).toHaveBeenCalledWith({
      username: 'new-admin',
      email: 'new-admin@example.com',
      password: 'brandNewPassw0rd!',
      is_admin: true,
    })
  );
});

test('a failed create surfaces the error and keeps the dialog open', async () => {
  vi.mocked(getUsers).mockResolvedValue({ users: [], total: 0, page: 1, limit: 25, total_pages: 0 });
  vi.mocked(createUser).mockRejectedValue(new Error('Username or email already in use'));

  renderPage();
  await waitFor(() => expect(screen.getByText('No users found')).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: /add user/i }));

  fireEvent.change(screen.getByLabelText('Username *'), { target: { value: 'taken' } });
  fireEvent.change(screen.getByLabelText('Email *'), { target: { value: 'taken@example.com' } });
  fireEvent.change(screen.getByLabelText('Password *'), { target: { value: 'brandNewPassw0rd!' } });
  fireEvent.click(screen.getByRole('button', { name: /^save$/i }));

  await waitFor(() => expect(screen.getByText('Username or email already in use')).toBeInTheDocument());
  // Dialog stays open with the entered values, ready to retry.
  expect(screen.getByLabelText('Username *')).toBeInTheDocument();
});

test('closing and reopening the create dialog resets the form', async () => {
  vi.mocked(getUsers).mockResolvedValue({ users: [], total: 0, page: 1, limit: 25, total_pages: 0 });

  renderPage();
  await waitFor(() => expect(screen.getByText('No users found')).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: /add user/i }));
  fireEvent.change(screen.getByLabelText('Username *'), { target: { value: 'abandoned' } });
  fireEvent.click(screen.getByRole('button', { name: /^cancel$/i }));

  // The dialog's exit transition is async even under fireEvent, so wait for
  // it to fully unmount (and stop shadowing the underlying page from the
  // accessibility tree) before reopening it.
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: /add user/i }));
  expect((screen.getByLabelText('Username *') as HTMLInputElement).value).toBe('');
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
