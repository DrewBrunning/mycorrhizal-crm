import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { FieldDefinition } from '../api/fieldDefinitions';
import BulkEditFieldsDialog from './BulkEditFieldsDialog';

afterEach(cleanup);

const def = (over: Partial<FieldDefinition>): FieldDefinition => ({
  id: 'd1',
  label: 'Pronouns',
  key: 'pronouns',
  target: 'contact',
  type: 'string',
  projection: 'internal-only',
  sensitivity: 'normal',
  position: 0,
  group: '',
  created_at: '',
  updated_at: '',
  ...over,
});

function renderDialog(defs: FieldDefinition[] = []) {
  const onSubmit = vi.fn();
  const onClose = vi.fn();
  render(
    <BulkEditFieldsDialog
      open
      selectedCount={3}
      busy={false}
      fieldDefinitions={defs}
      onClose={onClose}
      onSubmit={onSubmit}
    />,
  );
  return { onSubmit, onClose };
}

test('Apply is disabled until something is filled or marked Clear', () => {
  renderDialog();
  const apply = screen.getByRole('button', { name: 'Apply' });
  expect(apply).toBeDisabled();
  fireEvent.change(screen.getByLabelText('Organization'), {
    target: { value: 'Acme' },
  });
  expect(apply).toBeEnabled();
});

test('submits set for filled fields, clear for marked fields, and omits the rest', () => {
  const { onSubmit } = renderDialog([
    def({}),
    def({ id: 'd2', label: 'Age', key: 'age', type: 'number' }),
  ]);
  fireEvent.change(screen.getByLabelText('Organization'), {
    target: { value: 'Acme' },
  });
  fireEvent.click(screen.getByLabelText('Clear: Job title'));
  fireEvent.change(screen.getByLabelText('Pronouns'), {
    target: { value: 'they/them' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Apply' }));
  expect(onSubmit).toHaveBeenCalledWith({
    set: { organization: 'Acme' },
    clear: ['job_title'],
    custom_set: [{ field_definition_id: 'd1', value: 'they/them' }],
  });
  // Non-text custom types are not offered in the bulk dialog.
  expect(screen.queryByLabelText('Age')).toBeNull();
});

test('marking Clear wins over typed text for that field', () => {
  const { onSubmit } = renderDialog();
  fireEvent.change(screen.getByLabelText('Role'), {
    target: { value: 'Lead' },
  });
  fireEvent.click(screen.getByLabelText('Clear: Role'));
  fireEvent.click(screen.getByRole('button', { name: 'Apply' }));
  expect(onSubmit).toHaveBeenCalledWith({ clear: ['role'] });
});
