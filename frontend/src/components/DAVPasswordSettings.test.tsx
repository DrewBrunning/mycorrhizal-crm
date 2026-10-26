import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { getCurrentUser } from '../api/admin';
import { getTwoFactorStatus, updateDAVPasswordPolicy } from '../api/users';
import { listPasskeys } from '../api/webauthn';
import { SnackbarProvider } from '../context/SnackbarContext';
import DAVPasswordSettings from './DAVPasswordSettings';

vi.mock('../api/admin', () => ({ getCurrentUser: vi.fn() }));
vi.mock('../api/users', () => ({
  getTwoFactorStatus: vi.fn(),
  updateDAVPasswordPolicy: vi.fn(),
}));
vi.mock('../api/webauthn', () => ({ listPasskeys: vi.fn() }));

beforeEach(() => {
  vi.mocked(getCurrentUser).mockReset();
  vi.mocked(getTwoFactorStatus).mockReset();
  vi.mocked(updateDAVPasswordPolicy).mockReset();
  vi.mocked(listPasskeys).mockReset();
  vi.mocked(listPasskeys).mockResolvedValue([]);
  vi.mocked(getCurrentUser).mockResolvedValue({
    id: 1,
    email: 'tester@example.com',
    username: 'tester',
    language: 'en',
    is_admin: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    allow_dav_password: false,
  });
  vi.mocked(getTwoFactorStatus).mockResolvedValue({ enabled: false });
});

afterEach(cleanup);

function renderCard() {
  return render(
    <SnackbarProvider>
      <DAVPasswordSettings />
    </SnackbarProvider>,
  );
}

test('allows the account password on DAV when no second factor is enrolled', async () => {
  vi.mocked(updateDAVPasswordPolicy).mockResolvedValue(true);
  renderCard();

  const toggle = await screen.findByRole('switch');
  fireEvent.click(toggle);

  await waitFor(() => expect(updateDAVPasswordPolicy).toHaveBeenCalledWith(true));
});

test('treats a passkey as an enrolled second factor', async () => {
  vi.mocked(listPasskeys).mockResolvedValue([
    { id: 'pk1', name: 'Laptop', created_at: '2026-01-01T00:00:00Z' },
  ]);
  renderCard();

  const toggle = await screen.findByRole('switch');
  expect(toggle).toBeDisabled();
  expect(screen.getByText(/two-factor authentication is enabled/i)).toBeInTheDocument();
});

test('refreshes when another settings card changes a second factor', async () => {
  vi.mocked(getTwoFactorStatus)
    .mockResolvedValueOnce({ enabled: false })
    .mockResolvedValueOnce({ enabled: true });
  renderCard();

  const toggle = await screen.findByRole('switch');
  expect(toggle).not.toBeDisabled();
  window.dispatchEvent(new Event('mycorrhizal:second-factor-changed'));

  await waitFor(() => expect(screen.getByRole('switch')).toBeDisabled());
});

test('disables the setting while a second factor is enrolled', async () => {
  vi.mocked(getTwoFactorStatus).mockResolvedValue({ enabled: true });
  vi.mocked(getCurrentUser).mockResolvedValue({
    id: 1,
    email: 'tester@example.com',
    username: 'tester',
    language: 'en',
    is_admin: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    allow_dav_password: true,
  });
  renderCard();

  const toggle = await screen.findByRole('switch');
  expect(toggle).toBeDisabled();
  expect(screen.getByText(/two-factor authentication is enabled/i)).toBeInTheDocument();
});
