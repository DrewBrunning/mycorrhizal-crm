import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { SnackbarProvider } from '../context/SnackbarContext';
import * as clipboard from '../utils/clipboard';
import CopyButton from './CopyButton';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

test('has an accessible name derived from the label prop', () => {
  render(
    <SnackbarProvider>
      <CopyButton value="alice@example.com" label="Email" />
    </SnackbarProvider>,
  );
  expect(screen.getByLabelText('Copy Email')).toBeInTheDocument();
});

test('falls back to a generic accessible name without a label', () => {
  render(
    <SnackbarProvider>
      <CopyButton value="alice@example.com" />
    </SnackbarProvider>,
  );
  expect(screen.getByLabelText('Copy')).toBeInTheDocument();
});

test('copies the raw value and confirms via the snackbar on success', async () => {
  vi.spyOn(clipboard, 'copyToClipboard').mockResolvedValue(true);
  render(
    <SnackbarProvider>
      <CopyButton value="+15551234567" label="Phone" />
    </SnackbarProvider>,
  );

  fireEvent.click(screen.getByLabelText('Copy Phone'));

  expect(clipboard.copyToClipboard).toHaveBeenCalledWith('+15551234567');
  await waitFor(() => expect(screen.getByText('Copied to clipboard')).toBeInTheDocument());
});

test('shows an error snackbar when the copy fails', async () => {
  vi.spyOn(clipboard, 'copyToClipboard').mockResolvedValue(false);
  render(
    <SnackbarProvider>
      <CopyButton value="+15551234567" label="Phone" />
    </SnackbarProvider>,
  );

  fireEvent.click(screen.getByLabelText('Copy Phone'));

  await waitFor(() => expect(screen.getByText("Couldn't copy to clipboard")).toBeInTheDocument());
});
