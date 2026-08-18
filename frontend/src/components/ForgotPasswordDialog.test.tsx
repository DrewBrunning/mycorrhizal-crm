import { describe, test, expect, vi, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent, waitFor } from '@testing-library/react';
import '../i18n/config';
import ForgotPasswordDialog from './ForgotPasswordDialog';
import { AppThemeProvider } from '../AppThemeProvider';
import { requestPasswordReset } from '../api/auth';

vi.mock('../api/auth', () => ({
  requestPasswordReset: vi.fn(),
  confirmPasswordReset: vi.fn(),
}));

const requestPasswordResetMock = vi.mocked(requestPasswordReset);

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function renderDialog() {
  return render(
    <AppThemeProvider>
      <ForgotPasswordDialog open onClose={() => {}} />
    </AppThemeProvider>
  );
}

// Issue #192: same fix as LoginPage.tsx -- a failed submission used to drop
// keyboard focus to <body> with no programmatic link between the error and
// the field it concerns.
describe('ForgotPasswordDialog failed-request focus and error association', () => {
  test('moves focus to the error and associates it with the email field on a rejected request', async () => {
    requestPasswordResetMock.mockRejectedValue(new Error('No account with that email'));
    renderDialog();

    const emailInput = screen.getByLabelText(/email/i);
    fireEvent.change(emailInput, { target: { value: 'nobody@example.com' } });
    fireEvent.click(screen.getByRole('button', { name: /send reset email/i }));

    await screen.findByText('No account with that email');
    const alert = screen.getByRole('alert');
    await waitFor(() => expect(document.activeElement).toBe(alert));

    expect(emailInput).toHaveAttribute('aria-describedby', alert.id);
    expect(emailInput).toHaveAttribute('aria-invalid', 'true');
  });
});
