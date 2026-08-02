import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent } from '@testing-library/react';
import '../i18n/config';
import FieldDefinitionDialog from './FieldDefinitionDialog';
import { FieldDefinition } from '../api/fieldDefinitions';

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

  expect(await screen.findByText('Enum fields need at least one allowed value.')).toBeInTheDocument();
  expect(onSave).not.toHaveBeenCalled();
});
