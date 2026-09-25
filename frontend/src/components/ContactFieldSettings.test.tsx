import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { getEnabledContactFields, updateEnabledContactFields } from '../api/users';
import ContactFieldSettings from './ContactFieldSettings';

// This codebase's vitest setup has no auto-cleanup and no globals: true.
afterEach(cleanup);

vi.mock('../api/users', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/users')>();
  return {
    ...actual,
    getEnabledContactFields: vi.fn(),
    updateEnabledContactFields: vi.fn(),
  };
});

beforeEach(() => {
  vi.mocked(getEnabledContactFields).mockReset();
  vi.mocked(updateEnabledContactFields).mockReset();
});

test('shows a loading state, then the group headers and switches once fields load', async () => {
  vi.mocked(getEnabledContactFields).mockResolvedValue(['emails']);

  render(<ContactFieldSettings />);

  expect(screen.getByText('Loading…')).toBeInTheDocument();

  await waitFor(() => expect(screen.getByText('Communication')).toBeInTheDocument());
  expect(screen.getByText('Personal')).toBeInTheDocument();
  expect(screen.queryByText('Loading…')).not.toBeInTheDocument();
});

test('when the backend has never stored a preference, the default set is checked', async () => {
  vi.mocked(getEnabledContactFields).mockResolvedValue(null);

  render(<ContactFieldSettings />);

  await waitFor(() => expect(screen.getByLabelText('Email')).toBeInTheDocument());
  // 'emails' is in DEFAULT_ENABLED_CONTACT_FIELDS.
  expect(screen.getByLabelText('Email')).toBeChecked();
  // 'links' (URLs) is not in the default set.
  expect(screen.getByLabelText('Websites')).not.toBeChecked();
});

test('the stored enabled fields determine which switches are checked', async () => {
  vi.mocked(getEnabledContactFields).mockResolvedValue(['links']);

  render(<ContactFieldSettings />);

  await waitFor(() => expect(screen.getByLabelText('Websites')).toBeInTheDocument());
  expect(screen.getByLabelText('Websites')).toBeChecked();
  expect(screen.getByLabelText('Email')).not.toBeChecked();
});

test('a load failure shows an error message', async () => {
  vi.mocked(getEnabledContactFields).mockRejectedValue(new Error('Backend unreachable'));

  render(<ContactFieldSettings />);

  await waitFor(() => expect(screen.getByText('Backend unreachable')).toBeInTheDocument());
});

test('toggling a field off saves the updated set without that key', async () => {
  vi.mocked(getEnabledContactFields).mockResolvedValue(['emails', 'links']);
  vi.mocked(updateEnabledContactFields).mockResolvedValue(['links']);

  render(<ContactFieldSettings />);
  await waitFor(() => expect(screen.getByLabelText('Email')).toBeChecked());

  fireEvent.click(screen.getByLabelText('Email'));

  expect(screen.getByLabelText('Email')).not.toBeChecked();
  await waitFor(() =>
    expect(updateEnabledContactFields).toHaveBeenCalledWith(expect.arrayContaining(['links'])),
  );
  const savedArgs = vi.mocked(updateEnabledContactFields).mock.calls[0][0];
  expect(savedArgs).not.toContain('emails');
});

test('toggling a field on saves the updated set including that key', async () => {
  vi.mocked(getEnabledContactFields).mockResolvedValue(['emails']);
  vi.mocked(updateEnabledContactFields).mockResolvedValue(['emails', 'links']);

  render(<ContactFieldSettings />);
  await waitFor(() => expect(screen.getByLabelText('Websites')).not.toBeChecked());

  fireEvent.click(screen.getByLabelText('Websites'));

  expect(screen.getByLabelText('Websites')).toBeChecked();
  await waitFor(() =>
    expect(updateEnabledContactFields).toHaveBeenCalledWith(
      expect.arrayContaining(['emails', 'links']),
    ),
  );
});

test('a save failure shows an error message but the optimistic toggle stays applied', async () => {
  vi.mocked(getEnabledContactFields).mockResolvedValue(['emails']);
  vi.mocked(updateEnabledContactFields).mockRejectedValue(new Error('Save failed hard'));

  render(<ContactFieldSettings />);
  await waitFor(() => expect(screen.getByLabelText('Websites')).not.toBeChecked());

  fireEvent.click(screen.getByLabelText('Websites'));

  await waitFor(() => expect(screen.getByText('Save failed hard')).toBeInTheDocument());
  expect(screen.getByLabelText('Websites')).toBeChecked();
});
