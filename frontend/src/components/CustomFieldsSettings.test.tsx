import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import {
  createFieldDefinition,
  deleteFieldDefinition,
  type FieldDefinition,
  getFieldDefinitions,
  updateFieldDefinition,
} from '../api/fieldDefinitions';
import { SnackbarProvider } from '../context/SnackbarContext';
import CustomFieldsSettings from './CustomFieldsSettings';

// This codebase's vitest setup has no auto-cleanup and no globals: true.
afterEach(cleanup);

vi.mock('../api/fieldDefinitions', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/fieldDefinitions')>();
  return {
    ...actual,
    getFieldDefinitions: vi.fn(),
    createFieldDefinition: vi.fn(),
    updateFieldDefinition: vi.fn(),
    deleteFieldDefinition: vi.fn(),
  };
});

function definition(overrides: Partial<FieldDefinition> = {}): FieldDefinition {
  return {
    id: 'def-1',
    label: 'Pronouns',
    key: 'pronouns',
    target: 'contact',
    type: 'string',
    projection: 'internal-only',
    sensitivity: 'normal',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

function renderSettings() {
  return render(
    <SnackbarProvider>
      <CustomFieldsSettings />
    </SnackbarProvider>,
  );
}

beforeEach(() => {
  vi.mocked(getFieldDefinitions).mockReset();
  vi.mocked(createFieldDefinition).mockReset();
  vi.mocked(updateFieldDefinition).mockReset();
  vi.mocked(deleteFieldDefinition).mockReset();
});

// NOTE: CustomFieldsSettings mounts via `useFieldDefinitions()` but never
// calls the hook's `refresh()` on mount (no useEffect anywhere in the
// component or the hook) -- unlike ImportContactsDialog, which does call
// `refresh`. So on a fresh mount the definitions list is always empty and
// getFieldDefinitions is never requested, even though `loading` starts
// `false` so no spinner appears either. This looks like a real bug (existing
// custom fields never show up on this settings page without some other
// trigger), not intended behavior -- pinned here rather than silently
// worked around.
test('on mount, shows the empty state and never requests the definitions (documents a real bug: no fetch-on-mount)', () => {
  renderSettings();

  expect(screen.getByText('No custom fields defined yet.')).toBeInTheDocument();
  expect(getFieldDefinitions).not.toHaveBeenCalled();
});

test('creating a field opens the dialog and, on save, posts the input and shows a success message', async () => {
  vi.mocked(createFieldDefinition).mockResolvedValue(definition());
  vi.mocked(getFieldDefinitions).mockResolvedValue({
    field_definitions: [definition()],
    total: 1,
    next_cursor: '',
    limit: 100,
  });

  renderSettings();

  fireEvent.click(screen.getByRole('button', { name: 'Add' }));
  expect(screen.getByText('Add Custom Field')).toBeInTheDocument();

  fireEvent.change(screen.getByLabelText('Label *'), { target: { value: 'Pronouns' } });
  fireEvent.change(screen.getByLabelText('Key *'), { target: { value: 'pronouns' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await waitFor(() =>
    expect(createFieldDefinition).toHaveBeenCalledWith(
      expect.objectContaining({ label: 'Pronouns', key: 'pronouns', type: 'string' }),
    ),
  );
  // handleCreate refreshes the list via getFieldDefinitions after creating.
  await waitFor(() => expect(getFieldDefinitions).toHaveBeenCalled());
  await waitFor(() => expect(screen.getByText('Pronouns')).toBeInTheDocument());
});

test('after a definition is loaded (via a refresh triggered by create), it lists type/sensitivity chips', async () => {
  vi.mocked(createFieldDefinition).mockResolvedValue(definition());
  vi.mocked(getFieldDefinitions).mockResolvedValue({
    field_definitions: [
      definition({ id: 'def-2', label: 'Secret Note', type: 'text', sensitivity: 'secret' }),
    ],
    total: 1,
    next_cursor: '',
    limit: 100,
  });

  renderSettings();
  fireEvent.click(screen.getByRole('button', { name: 'Add' }));
  fireEvent.change(screen.getByLabelText('Label *'), { target: { value: 'Secret Note' } });
  fireEvent.change(screen.getByLabelText('Key *'), { target: { value: 'secret_note' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await waitFor(() => expect(screen.getByText('Secret Note')).toBeInTheDocument());
  expect(screen.getByText('Secret')).toBeInTheDocument();
});

test('editing a definition opens the dialog prefilled and saves via update', async () => {
  vi.mocked(createFieldDefinition).mockResolvedValue(definition());
  vi.mocked(getFieldDefinitions)
    .mockResolvedValueOnce({
      field_definitions: [definition()],
      total: 1,
      next_cursor: '',
      limit: 100,
    })
    .mockResolvedValueOnce({
      field_definitions: [definition({ label: 'Preferred Pronouns' })],
      total: 1,
      next_cursor: '',
      limit: 100,
    });
  vi.mocked(updateFieldDefinition).mockResolvedValue(definition({ label: 'Preferred Pronouns' }));

  renderSettings();
  // Seed the list by going through the create flow first (mount doesn't fetch).
  fireEvent.click(screen.getByRole('button', { name: 'Add' }));
  fireEvent.change(screen.getByLabelText('Label *'), { target: { value: 'Pronouns' } });
  fireEvent.change(screen.getByLabelText('Key *'), { target: { value: 'pronouns' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  await waitFor(() => expect(screen.getByText('Pronouns')).toBeInTheDocument());
  // The create dialog's own close (a separate state update from the list
  // refresh above) can still be pending here -- wait for it to actually
  // unmount before querying by role, since MUI's Modal marks the rest of the
  // page aria-hidden="true" while any dialog is open, which hides the list's
  // Edit/Delete buttons from the accessibility tree the role queries use.
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
  expect(screen.getByText('Edit Custom Field')).toBeInTheDocument();
  const labelInput = screen.getByLabelText('Label *') as HTMLInputElement;
  expect(labelInput.value).toBe('Pronouns');
  // Key is read-only in edit mode.
  expect(screen.getByLabelText('Key *')).toBeDisabled();

  fireEvent.change(labelInput, { target: { value: 'Preferred Pronouns' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await waitFor(() =>
    expect(updateFieldDefinition).toHaveBeenCalledWith(
      'def-1',
      expect.objectContaining({ label: 'Preferred Pronouns' }),
    ),
  );
  await waitFor(() => expect(screen.getByText('Preferred Pronouns')).toBeInTheDocument());
});

test('deleting a definition requires confirmation and calls delete with the id', async () => {
  vi.mocked(createFieldDefinition).mockResolvedValue(definition());
  vi.mocked(getFieldDefinitions)
    .mockResolvedValueOnce({
      field_definitions: [definition()],
      total: 1,
      next_cursor: '',
      limit: 100,
    })
    .mockResolvedValueOnce({ field_definitions: [], total: 0, next_cursor: '', limit: 100 });
  vi.mocked(deleteFieldDefinition).mockResolvedValue(undefined);

  renderSettings();
  fireEvent.click(screen.getByRole('button', { name: 'Add' }));
  fireEvent.change(screen.getByLabelText('Label *'), { target: { value: 'Pronouns' } });
  fireEvent.change(screen.getByLabelText('Key *'), { target: { value: 'pronouns' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  await waitFor(() => expect(screen.getByText('Pronouns')).toBeInTheDocument());
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
  expect(deleteFieldDefinition).not.toHaveBeenCalled();
  expect(screen.getByText('Delete Custom Field')).toBeInTheDocument();
  expect(
    screen.getByText(
      'Are you sure you want to delete the field "Pronouns"? This will remove the field from all contacts.',
    ),
  ).toBeInTheDocument();

  // Confirm within the dialog (the second "Delete" button, the destructive one).
  const deleteButtons = screen.getAllByRole('button', { name: 'Delete' });
  fireEvent.click(deleteButtons[deleteButtons.length - 1]);

  await waitFor(() => expect(deleteFieldDefinition).toHaveBeenCalledWith('def-1'));
  await waitFor(() => expect(screen.queryByText('Pronouns')).not.toBeInTheDocument());
});

test('canceling the delete dialog does not delete anything', async () => {
  vi.mocked(createFieldDefinition).mockResolvedValue(definition());
  vi.mocked(getFieldDefinitions).mockResolvedValue({
    field_definitions: [definition()],
    total: 1,
    next_cursor: '',
    limit: 100,
  });

  renderSettings();
  fireEvent.click(screen.getByRole('button', { name: 'Add' }));
  fireEvent.change(screen.getByLabelText('Label *'), { target: { value: 'Pronouns' } });
  fireEvent.change(screen.getByLabelText('Key *'), { target: { value: 'pronouns' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  await waitFor(() => expect(screen.getByText('Pronouns')).toBeInTheDocument());
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

  expect(deleteFieldDefinition).not.toHaveBeenCalled();
  await waitFor(() => expect(screen.queryByText('Delete Custom Field')).not.toBeInTheDocument());
  expect(screen.getByText('Pronouns')).toBeInTheDocument();
});

test('a validation error in the dialog (missing label) blocks the save call', () => {
  renderSettings();

  fireEvent.click(screen.getByRole('button', { name: 'Add' }));
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(screen.getByText('Label is required.')).toBeInTheDocument();
  expect(createFieldDefinition).not.toHaveBeenCalled();
});
