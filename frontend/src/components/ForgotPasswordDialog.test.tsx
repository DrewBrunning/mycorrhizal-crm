import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';
import '../i18n/config';
import { AppThemeProvider } from '../AppThemeProvider';
import { requestPasswordReset } from '../api/auth';
import ForgotPasswordDialog from './ForgotPasswordDialog';

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
    </AppThemeProvider>,
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

  // Regression test: handleConfirm's password-mismatch check calls
  // setError(sameMessage) directly with no intervening clear, unlike the
  // network-error path (which does setError('') before the try). Unlike the
  // empty-field checks (redundant with each TextField's `required`, so
  // native constraint validation blocks the submit before React ever sees
  // it), two non-empty-but-mismatched passwords are NOT caught by `required`
  // -- resubmitting without fixing anything sets React state to the exact
  // same string. React bails out of re-rendering (and therefore effects)
  // when a state update is Object.is-equal to the current value, so the
  // focus-on-error effect would only fire on the first of two identical
  // mismatch errors.
  test('moves focus to the error again on a second identical password-mismatch failure', async () => {
    requestPasswordResetMock.mockResolvedValue('If an account exists, instructions were sent.');
    renderDialog();

    fireEvent.change(screen.getByLabelText(/email/i), { target: { value: 'a@example.com' } });
    fireEvent.click(screen.getByRole('button', { name: /send reset email/i }));
    await screen.findByLabelText(/reset token/i);

    const fillMismatch = () => {
      fireEvent.change(screen.getByLabelText(/reset token/i), { target: { value: 'tok123' } });
      fireEvent.change(screen.getByLabelText(/^new password/i), {
        target: { value: 'NewPassword1!' },
      });
      fireEvent.change(screen.getByLabelText(/confirm new password/i), {
        target: { value: 'Different1!' },
      });
    };
    const submit = () => screen.getByRole('button', { name: /reset password/i });

    // The "instructions were sent" info Alert from the request step is
    // still mounted here (also role="alert" -- MUI sets that regardless of
    // severity), so target the error alert by its stable id rather than by
    // role to avoid ambiguity between the two.
    const getErrorAlert = () => document.getElementById('forgot-password-error');

    fillMismatch();
    fireEvent.click(submit());
    await waitFor(() => expect(getErrorAlert()).not.toBeNull());
    const alert = getErrorAlert();
    await waitFor(() => expect(document.activeElement).toBe(alert));

    // Simulate the user tabbing away, then submitting again without fixing
    // anything -- the exact same mismatch message fires a second time.
    (document.activeElement as HTMLElement)?.blur();
    expect(document.activeElement).not.toBe(alert);

    fireEvent.click(submit());
    await waitFor(() => expect(document.activeElement).toBe(getErrorAlert()));
  });
});
