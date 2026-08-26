import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { useState } from 'react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import {
  emptyEditorValue,
  type FieldDefinition,
  type FieldValueEditorState,
} from '../api/fieldDefinitions';
import { DateFormatProvider } from '../DateFormatProvider';
import FieldValueEditor from './FieldValueEditor';

afterEach(cleanup);

function def(overrides: Partial<FieldDefinition>): FieldDefinition {
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

function renderEditor(
  defn: FieldDefinition,
  value: string | boolean | string[],
  onChange = vi.fn(),
) {
  return render(
    <DateFormatProvider>
      <FieldValueEditor definition={defn} value={value} onChange={onChange} />
    </DateFormatProvider>,
  );
}

// The editor is a controlled component; StatefulEditor lets a test drive it
// the way the real parents (AddContactDialog, CustomFieldValueRow) do, so an
// Add click actually adds a row instead of only recording an onChange call.
function StatefulEditor({ definition }: { definition: FieldDefinition }) {
  const [value, setValue] = useState<FieldValueEditorState>(() => emptyEditorValue(definition));
  return (
    <DateFormatProvider>
      <FieldValueEditor definition={definition} value={value} onChange={setValue} />
    </DateFormatProvider>
  );
}

// T7 requires component tests for at least the enum and Multi rendering paths.

test('enum scalar renders a Select of the definition allowed values', () => {
  const enumDef = def({ type: 'enum', constraints: { values: ['S', 'M', 'L'] } });
  renderEditor(enumDef, 'M');

  fireEvent.mouseDown(screen.getByLabelText('Value'));
  const options = screen.getAllByRole('option');
  expect(options.map((o) => o.textContent)).toEqual(['Select value', 'S', 'M', 'L']);
});

test('enum scalar reports the picked value through onChange', () => {
  const onChange = vi.fn();
  const enumDef = def({ type: 'enum', constraints: { values: ['S', 'M', 'L'] } });
  renderEditor(enumDef, '', onChange);

  fireEvent.mouseDown(screen.getByLabelText('Value'));
  fireEvent.click(screen.getByRole('option', { name: 'L' }));

  expect(onChange).toHaveBeenCalledWith('L');
});

test('Multi enum renders an add/remove list, not a single select', () => {
  const multiEnum = def({
    type: 'enum',
    constraints: { values: ['she/her', 'he/him'], multi: true },
  });
  renderEditor(multiEnum, ['she/her']);

  // The list starts with the existing element as its own row.
  expect(screen.getByDisplayValue('she/her')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Add' })).toBeInTheDocument();
});

test('Multi adds a row and typing into it records the value', () => {
  const multiString = def({ type: 'string', constraints: { multi: true } });
  render(<StatefulEditor definition={multiString} />);

  fireEvent.click(screen.getByRole('button', { name: 'Add' }));
  const input = screen.getByDisplayValue('');
  fireEvent.change(input, { target: { value: 'first' } });

  expect(screen.getByDisplayValue('first')).toBeInTheDocument();
});

test('Multi removes a row through the delete button', () => {
  const onChange = vi.fn();
  const multiString = def({ type: 'string', constraints: { multi: true } });
  renderEditor(multiString, ['a', 'b'], onChange);

  const deleteButtons = screen.getAllByRole('button', { name: 'Delete' });
  expect(deleteButtons).toHaveLength(2);
  fireEvent.click(deleteButtons[0]);

  expect(onChange).toHaveBeenCalledWith(['b']);
});

test('boolean scalar renders a Switch', () => {
  const boolDef = def({ type: 'boolean' });
  renderEditor(boolDef, true);
  expect(screen.getByRole('switch')).toBeChecked();
});
