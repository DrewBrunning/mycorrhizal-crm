import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { FieldDefinition } from '../api/fieldDefinitions';
import FieldDefinitionDialog from './FieldDefinitionDialog';

afterEach(cleanup);

function renderDialog(props: Partial<React.ComponentProps<typeof FieldDefinitionDialog>> = {}) {
  const defaults: React.ComponentProps<typeof FieldDefinitionDialog> = {
    open: true,
    onClose: vi.fn(),
    onSave: vi.fn().mockResolvedValue(undefined),
    ...props,
  };
  return render(<FieldDefinitionDialog {...defaults} />);
}

function existingDef(overrides: Partial<FieldDefinition> = {}): FieldDefinition {
  return {
    id: 'def-1',
    label: 'Pronouns',
    key: 'pronouns',
    target: 'contact',
    type: 'enum',
    constraints: { values: ['she/her', 'he/him'], multi: true },
    projection: 'internal-only',
    sensitivity: 'normal',
    position: 0,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

test('create mode shows label, key, type, and defaults', () => {
  renderDialog();
  // MUI appends " *" to required labels.
  expect(screen.getByLabelText('Label *')).toBeInTheDocument();
  expect(screen.getByLabelText('Key *')).toBeInTheDocument();
  expect(screen.getByLabelText('Type')).toBeInTheDocument();
  expect(screen.getByLabelText('Export mapping')).toBeInTheDocument();
  expect(screen.getByLabelText('Sensitivity')).toBeInTheDocument();
});

test('an enum definition saves its allowed values with multi', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  fireEvent.change(screen.getByLabelText('Label *'), { target: { value: 'Pronouns' } });
  fireEvent.change(screen.getByLabelText('Key *'), { target: { value: 'pronouns' } });

  // Pick the enum type so the allowed-values editor appears.
  fireEvent.mouseDown(screen.getByLabelText('Type'));
  fireEvent.click(await screen.findByRole('option', { name: 'Choice' }));

  // Add two allowed values: click Add, then type into the newly added row.
  fireEvent.click(screen.getByRole('button', { name: 'Add' }));
  fireEvent.change(screen.getAllByDisplayValue('')[0], { target: { value: 'she/her' } });
  fireEvent.click(screen.getByRole('button', { name: 'Add' }));
  fireEvent.change(screen.getAllByDisplayValue('')[0], { target: { value: 'he/him' } });

  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave).toHaveBeenCalledWith({
    label: 'Pronouns',
    key: 'pronouns',
    type: 'enum',
    // multi:false is omitted (omitempty), matching the backend's FieldConstraints
    constraints: { values: ['she/her', 'he/him'] },
    projection: 'internal-only',
    sensitivity: 'normal',
  });
});

test('edit mode pre-fills the definition and disables the key field', () => {
  renderDialog({ definition: existingDef() });
  expect(screen.getByLabelText('Label *')).toHaveValue('Pronouns');
  const keyField = screen.getByLabelText('Key *');
  expect(keyField).toHaveValue('pronouns');
  expect(keyField).toBeDisabled();
});

test('an empty enum value list is rejected before save', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  fireEvent.change(screen.getByLabelText('Label *'), { target: { value: 'Size' } });
  fireEvent.change(screen.getByLabelText('Key *'), { target: { value: 'size' } });
  fireEvent.mouseDown(screen.getByLabelText('Type'));
  fireEvent.click(await screen.findByRole('option', { name: 'Choice' }));

  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(
    await screen.findByText('Enum fields need at least one allowed value.'),
  ).toBeInTheDocument();
  expect(onSave).not.toHaveBeenCalled();
});

test('a blank label is rejected before save', () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  fireEvent.change(screen.getByLabelText('Key *'), { target: { value: 'size' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(screen.getByText('Label is required.')).toBeInTheDocument();
  expect(onSave).not.toHaveBeenCalled();
});

test('a blank key is rejected before save in create mode', () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  fireEvent.change(screen.getByLabelText('Label *'), { target: { value: 'Size' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(screen.getByText('Key is required.')).toBeInTheDocument();
  expect(onSave).not.toHaveBeenCalled();
});

test('edit mode does not require a key (it is disabled, not validated)', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave, definition: existingDef({ type: 'string', constraints: {} }) });

  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(screen.queryByText('Key is required.')).not.toBeInTheDocument();
});

test('a vCard projection requires a property name before saving, then round-trips it on save', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  fireEvent.change(screen.getByLabelText('Label *'), { target: { value: 'Pronouns' } });
  fireEvent.change(screen.getByLabelText('Key *'), { target: { value: 'pronouns' } });

  fireEvent.mouseDown(screen.getByLabelText('Export mapping'));
  fireEvent.click(await screen.findByRole('option', { name: 'vCard property' }));

  // No name yet -- rejected before save.
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  expect(await screen.findByText('A vCard property name is required.')).toBeInTheDocument();
  expect(onSave).not.toHaveBeenCalled();

  fireEvent.change(screen.getByLabelText('vCard property name *'), {
    target: { value: 'PRONOUNS' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ projection: 'vcard:X-PRONOUNS' }));
});

test('edit mode decodes an existing vcard: projection back into the property-name field', () => {
  renderDialog({ definition: existingDef({ type: 'string', projection: 'vcard:X-NICKNAME' }) });

  expect(screen.getByLabelText('Export mapping')).toHaveTextContent('vCard property');
  expect(screen.getByLabelText('vCard property name *')).toHaveValue('NICKNAME');
});

test('a number field builds min/max constraints from the entered values', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  fireEvent.change(screen.getByLabelText('Label *'), { target: { value: 'Age' } });
  fireEvent.change(screen.getByLabelText('Key *'), { target: { value: 'age' } });
  fireEvent.mouseDown(screen.getByLabelText('Type'));
  fireEvent.click(await screen.findByRole('option', { name: 'Number' }));

  fireEvent.change(screen.getByLabelText('Min'), { target: { value: '1' } });
  fireEvent.change(screen.getByLabelText('Max'), { target: { value: '120' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave).toHaveBeenCalledWith(
    expect.objectContaining({ constraints: { min: 1, max: 120 } }),
  );
});

test('a string field builds maxLength/pattern constraints from the entered values', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  // Default type is already "string".
  fireEvent.change(screen.getByLabelText('Label *'), { target: { value: 'Zip' } });
  fireEvent.change(screen.getByLabelText('Key *'), { target: { value: 'zip' } });
  fireEvent.change(screen.getByLabelText('Max length'), { target: { value: '10' } });
  fireEvent.change(screen.getByLabelText('Pattern (regex)'), { target: { value: '^[0-9]{5}$' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave).toHaveBeenCalledWith(
    expect.objectContaining({ constraints: { maxLength: 10, pattern: '^[0-9]{5}$' } }),
  );
});

test('a rejected save surfaces the thrown Error message and re-enables the form', async () => {
  const onSave = vi.fn().mockRejectedValue(new Error('field key already exists'));
  renderDialog({ onSave });

  fireEvent.change(screen.getByLabelText('Label *'), { target: { value: 'Size' } });
  fireEvent.change(screen.getByLabelText('Key *'), { target: { value: 'size' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(await screen.findByText('field key already exists')).toBeInTheDocument();
  // The dialog stayed open and the Save button is usable again (not stuck on "Saving...").
  expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled();
});

test('a rejected save with a non-Error value falls back to the generic message', async () => {
  const onSave = vi.fn().mockRejectedValue('server exploded');
  renderDialog({ onSave });

  fireEvent.change(screen.getByLabelText('Label *'), { target: { value: 'Size' } });
  fireEvent.change(screen.getByLabelText('Key *'), { target: { value: 'size' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(await screen.findByText('Failed to save custom field.')).toBeInTheDocument();
});
