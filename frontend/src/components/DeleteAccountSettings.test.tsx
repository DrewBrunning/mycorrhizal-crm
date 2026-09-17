import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { logoutAndRedirect } from '../auth';
import { SnackbarProvider } from '../context/SnackbarContext';
import DeleteAccountSettings from './DeleteAccountSettings';

vi.mock('../auth', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../auth')>();
  return {
    ...actual,
    logoutAndRedirect: vi.fn(),
  };
});

const logoutAndRedirectMock = vi.mocked(logoutAndRedirect);

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

function jsonResponse(body: unknown, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    text: async () => JSON.stringify(body),
  };
}

function mockFetchByUrl(
  handlers: Record<string, (init?: RequestInit) => ReturnType<typeof jsonResponse>>,
) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      for (const [pattern, respond] of Object.entries(handlers)) {
        if (url.includes(pattern)) {
          return respond(init);
        }
      }
      throw new Error(`unexpected fetch: ${url}`);
    }),
  );
}

function renderComponent() {
  return render(
    <SnackbarProvider>
      <DeleteAccountSettings />
    </SnackbarProvider>,
  );
}

test('the confirm button stays disabled until a password is entered', async () => {
  mockFetchByUrl({ '/users/2fa/status': () => jsonResponse({ enabled: false }) });
  renderComponent();

  fireEvent.click(screen.getByText('Delete my account'));
  const confirmButton = await screen.findByText('Permanently delete my account');
  expect(confirmButton.closest('button')).toBeDisabled();

  fireEvent.change(screen.getByLabelText('Current password *'), {
    target: { value: 'my-password' },
  });
  expect(confirmButton.closest('button')).not.toBeDisabled();
});

test('shows a TOTP field only when the account has 2FA enabled', async () => {
  mockFetchByUrl({ '/users/2fa/status': () => jsonResponse({ enabled: true }) });
  renderComponent();

  fireEvent.click(screen.getByText('Delete my account'));
  await screen.findByLabelText('Verification code *');
});

test('does not show a TOTP field when 2FA is disabled', async () => {
  mockFetchByUrl({ '/users/2fa/status': () => jsonResponse({ enabled: false }) });
  renderComponent();

  fireEvent.click(screen.getByText('Delete my account'));
  await screen.findByLabelText('Current password *');
  expect(screen.queryByLabelText('Verification code *')).not.toBeInTheDocument();
});

test('a 409 with candidates renders a promotion picker; picking one and resubmitting succeeds', async () => {
  mockFetchByUrl({
    '/users/2fa/status': () => jsonResponse({ enabled: false }),
    '/account': (init?: RequestInit) => {
      const body = JSON.parse(String(init?.body));
      if (body.promote_user_id === undefined) {
        return jsonResponse(
          {
            error: {
              code: 'CONFLICT',
              message: 'choose someone',
              details: { candidates: [{ id: 2, username: 'alice' }] },
            },
          },
          409,
        );
      }
      return jsonResponse({ message: 'Your account and all its data have been deleted.' });
    },
  });
  renderComponent();

  fireEvent.click(screen.getByText('Delete my account'));
  fireEvent.change(await screen.findByLabelText('Current password *'), {
    target: { value: 'my-password' },
  });
  fireEvent.click(screen.getByText('Permanently delete my account'));

  await screen.findByText(/You are the only admin/);
  expect(screen.getByText('Permanently delete my account').closest('button')).toBeDisabled();

  // MUI Select renders a hidden combobox trigger keyed by its label id.
  fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Promote to admin' }));
  fireEvent.click(await screen.findByRole('option', { name: 'alice' }));

  fireEvent.click(screen.getByText('Permanently delete my account'));

  await waitFor(() => expect(logoutAndRedirectMock).toHaveBeenCalledTimes(1));
});
