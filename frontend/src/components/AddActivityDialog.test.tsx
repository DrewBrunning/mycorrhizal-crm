import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { type Contact, getContacts } from '../api/contacts';
import AddActivityDialog from './AddActivityDialog';

// This codebase's vitest setup has no auto-cleanup and no globals: true.
afterEach(() => {
  cleanup();
  // Issue #557: AddActivityDialog persists a draft to sessionStorage while
  // dirty. jsdom's sessionStorage is shared across tests in this file, so a
  // draft left behind by one test (e.g. a failed save, which deliberately
  // keeps the dialog open with its content intact) would otherwise leak into
  // the next test's fresh render.
  sessionStorage.clear();
});

vi.mock('../api/contacts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/contacts')>();
  return { ...actual, getContacts: vi.fn() };
});

function contact(overrides: Partial<Contact> = {}): Contact {
  return { ID: 1, firstname: 'Alice', lastname: 'Johnson', ...overrides };
}

beforeEach(() => {
  vi.mocked(getContacts).mockReset();
  vi.mocked(getContacts).mockResolvedValue({ contacts: [contact()], next_cursor: '' } as never);
});

function renderDialog(props: Partial<React.ComponentProps<typeof AddActivityDialog>> = {}) {
  const defaults: React.ComponentProps<typeof AddActivityDialog> = {
    open: true,
    onClose: vi.fn(),
    onSave: vi.fn().mockResolvedValue(undefined),
    ...props,
  };
  return render(<AddActivityDialog {...defaults} />);
}

test('renders the title, description, location, date, and contacts fields', async () => {
  renderDialog();
  await waitFor(() => expect(getContacts).toHaveBeenCalled());

  expect(screen.getByLabelText('Title *')).toBeInTheDocument();
  expect(screen.getByLabelText('Description')).toBeInTheDocument();
  expect(screen.getByLabelText('Location')).toBeInTheDocument();
  expect(screen.getByLabelText('Date *')).toBeInTheDocument();
  expect(screen.getByLabelText('Contacts')).toBeInTheDocument();
});

test('requires a title and date before saving', async () => {
  const onSave = vi.fn();
  renderDialog({ onSave });
  await waitFor(() => expect(getContacts).toHaveBeenCalled());

  fireEvent.change(screen.getByLabelText('Title *'), { target: { value: '' } });
  fireEvent.click(screen.getByRole('button', { name: /^save$/i }));

  expect(screen.getByText('Title and date are required')).toBeInTheDocument();
  expect(onSave).not.toHaveBeenCalled();
});

test('saves with the entered title, description, location, and date', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  const onClose = vi.fn();
  renderDialog({ onSave, onClose });
  await waitFor(() => expect(getContacts).toHaveBeenCalled());

  fireEvent.change(screen.getByLabelText('Title *'), { target: { value: 'Coffee catchup' } });
  fireEvent.change(screen.getByLabelText('Description'), {
    target: { value: 'Talked about the new job' },
  });
  fireEvent.change(screen.getByLabelText('Location'), { target: { value: 'Blue Bottle' } });
  fireEvent.click(screen.getByRole('button', { name: /^save$/i }));

  await waitFor(() =>
    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({
        title: 'Coffee catchup',
        description: 'Talked about the new job',
        location: 'Blue Bottle',
        contact_ids: [],
      }),
    ),
  );
  // The dialog closes itself (via the parent's onClose) once the save resolves.
  await waitFor(() => expect(onClose).toHaveBeenCalled());
});

test('preselects the contact passed via preselectedContactId', async () => {
  vi.mocked(getContacts).mockResolvedValue({
    contacts: [
      contact({ ID: 1, firstname: 'Alice', lastname: 'Johnson' }),
      contact({ ID: 2, firstname: 'Bob', lastname: 'Smith' }),
    ],
    next_cursor: '',
  } as never);

  renderDialog({ preselectedContactId: 2 });

  await waitFor(() => expect(screen.getByText('Bob Smith')).toBeInTheDocument());
});

test('selecting a contact from the autocomplete includes it in contact_ids', async () => {
  vi.mocked(getContacts).mockResolvedValue({
    contacts: [contact({ ID: 3, firstname: 'Carol', lastname: 'Diaz' })],
    next_cursor: '',
  } as never);
  const onSave = vi.fn().mockResolvedValue(undefined);

  renderDialog({ onSave });
  await waitFor(() => expect(getContacts).toHaveBeenCalled());

  fireEvent.change(screen.getByLabelText('Title *'), { target: { value: 'Lunch' } });
  fireEvent.mouseDown(screen.getByLabelText('Contacts'));
  fireEvent.click(await screen.findByRole('option', { name: 'Carol Diaz' }));

  fireEvent.click(screen.getByRole('button', { name: /^save$/i }));

  await waitFor(() =>
    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ contact_ids: [3] })),
  );
});

test('a save failure keeps the dialog open and shows an error', async () => {
  const onSave = vi.fn().mockRejectedValue(new Error('boom'));
  const onClose = vi.fn();
  renderDialog({ onSave, onClose });
  await waitFor(() => expect(getContacts).toHaveBeenCalled());

  fireEvent.change(screen.getByLabelText('Title *'), { target: { value: 'Will fail' } });
  fireEvent.click(screen.getByRole('button', { name: /^save$/i }));

  await waitFor(() => expect(screen.getByText('Failed to save activity')).toBeInTheDocument());
  expect(onClose).not.toHaveBeenCalled();
});

test('cancel closes the dialog without saving', async () => {
  const onSave = vi.fn();
  const onClose = vi.fn();
  renderDialog({ onSave, onClose });
  await waitFor(() => expect(getContacts).toHaveBeenCalled());

  fireEvent.click(screen.getByRole('button', { name: /cancel/i }));

  expect(onSave).not.toHaveBeenCalled();
  expect(onClose).toHaveBeenCalled();
});

// Issue #557 item 3: a dirty activity is not silently discarded on Cancel.
test('cancel with typed fields asks for confirmation before discarding', async () => {
  const onClose = vi.fn();
  renderDialog({ onClose });
  await waitFor(() => expect(getContacts).toHaveBeenCalled());

  fireEvent.change(screen.getByLabelText('Title *'), { target: { value: 'Coffee catch-up' } });
  fireEvent.click(screen.getByRole('button', { name: /cancel/i }));

  expect(screen.getByText('Discard unsaved changes?')).toBeInTheDocument();
  expect(onClose).not.toHaveBeenCalled();

  // "Keep editing" backs out without losing what was typed.
  fireEvent.click(screen.getByRole('button', { name: 'Keep editing' }));
  await waitFor(() =>
    expect(screen.queryByText('Discard unsaved changes?')).not.toBeInTheDocument(),
  );
  expect(onClose).not.toHaveBeenCalled();
  expect(screen.getByLabelText('Title *')).toHaveValue('Coffee catch-up');

  // Confirming the discard actually closes.
  fireEvent.click(screen.getByRole('button', { name: /cancel/i }));
  fireEvent.click(screen.getByRole('button', { name: 'Discard' }));
  expect(onClose).toHaveBeenCalledTimes(1);
});

// Issue #557 item 4: a draft persisted to sessionStorage turns a killed tab
// or a crash-and-retry into an interruption, not a loss.
test('a dirty draft survives an unmount/remount (tab close, crash recovery)', async () => {
  const { unmount } = renderDialog();
  await waitFor(() => expect(getContacts).toHaveBeenCalled());

  fireEvent.change(screen.getByLabelText('Title *'), { target: { value: 'Draft activity' } });
  await waitFor(() => expect(screen.getByLabelText('Title *')).toHaveValue('Draft activity'));

  unmount();

  renderDialog();
  await waitFor(() => expect(screen.getByLabelText('Title *')).toHaveValue('Draft activity'));
});

// A successful save must not leave a stale draft behind to reappear the next
// time the dialog opens.
test('a successful save clears the draft', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  const { unmount } = renderDialog({ onSave });
  await waitFor(() => expect(getContacts).toHaveBeenCalled());

  fireEvent.change(screen.getByLabelText('Title *'), { target: { value: 'Saved activity' } });
  fireEvent.change(screen.getByLabelText('Date *'), { target: { value: '2026-01-01' } });
  fireEvent.click(screen.getByRole('button', { name: /^save$/i }));
  await waitFor(() => expect(onSave).toHaveBeenCalled());

  unmount();

  renderDialog();
  await waitFor(() => expect(screen.getByLabelText('Title *')).toBeInTheDocument());
  expect(screen.getByLabelText('Title *')).toHaveValue('');
});
