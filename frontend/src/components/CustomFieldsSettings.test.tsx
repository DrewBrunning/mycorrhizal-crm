import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import {
  createFieldDefinition,
  deleteFieldDefinition,
  type FieldDefinition,
  getFieldDefinitions,
  reorderFieldDefinitions,
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
    reorderFieldDefinitions: vi.fn(),
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
    position: 0,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

function response(definitions: FieldDefinition[]) {
  return {
    field_definitions: definitions,
    total: definitions.length,
    next_cursor: '',
    limit: 100,
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
  vi.mocked(reorderFieldDefinitions).mockReset();
  // The component now fetches on mount (issue #1210), so every test starts
  // from a fetched list; default to none unless a test overrides it.
  vi.mocked(getFieldDefinitions).mockResolvedValue(response([]));
});

test('fetches and renders existing definitions on mount', async () => {
  vi.mocked(getFieldDefinitions).mockResolvedValue(response([definition()]));

  renderSettings();

  await waitFor(() => expect(screen.getByText('Pronouns')).toBeInTheDocument());
  expect(getFieldDefinitions).toHaveBeenCalled();
});

test('shows the empty state when there are no definitions', async () => {
  renderSettings();

  await waitFor(() =>
    expect(screen.getByText('No custom fields defined yet.')).toBeInTheDocument(),
  );
});

test('creating a field opens the dialog and, on save, posts the input and refreshes', async () => {
  vi.mocked(getFieldDefinitions)
    .mockResolvedValueOnce(response([])) // mount
    .mockResolvedValueOnce(response([definition()])); // post-create refresh
  vi.mocked(createFieldDefinition).mockResolvedValue(definition());

  renderSettings();
  await waitFor(() => expect(getFieldDefinitions).toHaveBeenCalledTimes(1));

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
  await waitFor(() => expect(screen.getByText('Pronouns')).toBeInTheDocument());
});

test('a loaded definition lists type/sensitivity chips', async () => {
  vi.mocked(getFieldDefinitions).mockResolvedValue(
    response([
      definition({ id: 'def-2', label: 'Secret Note', type: 'text', sensitivity: 'secret' }),
    ]),
  );

  renderSettings();

  await waitFor(() => expect(screen.getByText('Secret Note')).toBeInTheDocument());
  expect(screen.getByText('Secret')).toBeInTheDocument();
});

test('editing a definition opens the dialog prefilled and saves via update', async () => {
  vi.mocked(getFieldDefinitions)
    .mockResolvedValueOnce(response([definition()])) // mount
    .mockResolvedValueOnce(response([definition({ label: 'Preferred Pronouns' })])); // refresh
  vi.mocked(updateFieldDefinition).mockResolvedValue(definition({ label: 'Preferred Pronouns' }));

  renderSettings();
  await waitFor(() => expect(screen.getByText('Pronouns')).toBeInTheDocument());

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
  vi.mocked(getFieldDefinitions)
    .mockResolvedValueOnce(response([definition()])) // mount
    .mockResolvedValueOnce(response([])); // refresh after delete
  vi.mocked(deleteFieldDefinition).mockResolvedValue(undefined);

  renderSettings();
  await waitFor(() => expect(screen.getByText('Pronouns')).toBeInTheDocument());

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
  vi.mocked(getFieldDefinitions).mockResolvedValue(response([definition()]));

  renderSettings();
  await waitFor(() => expect(screen.getByText('Pronouns')).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

  expect(deleteFieldDefinition).not.toHaveBeenCalled();
  await waitFor(() => expect(screen.queryByText('Delete Custom Field')).not.toBeInTheDocument());
  expect(screen.getByText('Pronouns')).toBeInTheDocument();
});

test('a validation error in the dialog (missing label) blocks the save call', async () => {
  renderSettings();
  // Wait for the mount fetch to settle and the list (with its Add button) to render.
  await waitFor(() => expect(screen.getByRole('button', { name: 'Add' })).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: 'Add' }));
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(screen.getByText('Label is required.')).toBeInTheDocument();
  expect(createFieldDefinition).not.toHaveBeenCalled();
});

async function renderTwoDefinitions() {
  const first = definition({ id: 'a', label: 'Signal', position: 0 });
  const second = definition({ id: 'b', label: 'Telegram', position: 1 });
  vi.mocked(getFieldDefinitions).mockResolvedValue(response([first, second]));
  renderSettings();
  await waitFor(() => expect(screen.getByText('Signal')).toBeInTheDocument());
  return { first, second };
}

test('moving a definition down persists the swapped full order', async () => {
  const { first, second } = await renderTwoDefinitions();
  vi.mocked(reorderFieldDefinitions).mockResolvedValue([
    { ...second, position: 0 },
    { ...first, position: 1 },
  ]);

  fireEvent.click(screen.getByLabelText('Move Signal down'));

  await waitFor(() => expect(reorderFieldDefinitions).toHaveBeenCalledWith(['b', 'a']));
});

test('moving a definition up persists the swapped full order', async () => {
  const { first, second } = await renderTwoDefinitions();
  vi.mocked(reorderFieldDefinitions).mockResolvedValue([
    { ...second, position: 0 },
    { ...first, position: 1 },
  ]);

  fireEvent.click(screen.getByLabelText('Move Telegram up'));

  await waitFor(() => expect(reorderFieldDefinitions).toHaveBeenCalledWith(['b', 'a']));
});

test('the first definition cannot move up and the last cannot move down', async () => {
  await renderTwoDefinitions();

  expect(screen.getByLabelText('Move Signal up')).toBeDisabled();
  expect(screen.getByLabelText('Move Telegram down')).toBeDisabled();
  expect(screen.getByLabelText('Move Signal down')).toBeEnabled();
  expect(screen.getByLabelText('Move Telegram up')).toBeEnabled();
});
