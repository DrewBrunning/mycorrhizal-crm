import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { FieldDefinition } from '../api/fieldDefinitions';
import { SnackbarProvider } from '../context/SnackbarContext';
import { DateFormatProvider } from '../DateFormatProvider';
import CustomFieldValueRow from './CustomFieldValueRow';

// This codebase's vitest setup has no auto-cleanup and no globals: true.
afterEach(cleanup);

function stringDefinition(overrides: Partial<FieldDefinition> = {}): FieldDefinition {
  return {
    id: 'def-1',
    label: 'Favorite Color',
    key: 'favorite_color',
    target: 'contact',
    type: 'string',
    projection: 'internal-only',
    sensitivity: 'normal',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

function renderRow(props: Partial<React.ComponentProps<typeof CustomFieldValueRow>> = {}) {
  const defaults: React.ComponentProps<typeof CustomFieldValueRow> = {
    definition: stringDefinition(),
    value: undefined,
    onSave: vi.fn().mockResolvedValue(undefined),
    ...props,
  };
  return render(
    <SnackbarProvider>
      <DateFormatProvider>
        <CustomFieldValueRow {...defaults} />
      </DateFormatProvider>
    </SnackbarProvider>,
  );
}

test('shows the definition label and an em-dash placeholder when there is no value', () => {
  renderRow();

  expect(screen.getByText('Favorite Color')).toBeInTheDocument();
  expect(screen.getByText('—')).toBeInTheDocument();
});

test('shows the current display value when present', () => {
  renderRow({ value: 'Teal' });

  expect(screen.getByText('Teal')).toBeInTheDocument();
});

test('entering edit mode reveals the editor and cancel/save controls', () => {
  renderRow({ value: 'Teal' });

  fireEvent.click(screen.getByLabelText('Edit'));

  expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Save' })).toBeInTheDocument();
});

test('cancel restores the display value and leaves edit mode without saving', () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderRow({ value: 'Teal', onSave });

  fireEvent.click(screen.getByLabelText('Edit'));
  const input = screen.getByRole('textbox') as HTMLInputElement;
  fireEvent.change(input, { target: { value: 'Blue' } });

  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

  expect(onSave).not.toHaveBeenCalled();
  expect(screen.getByText('Teal')).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Save' })).not.toBeInTheDocument();
});

test('saving a new value calls onSave with the definition id and the wire-encoded value', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderRow({ value: 'Teal', onSave });

  fireEvent.click(screen.getByLabelText('Edit'));
  const input = screen.getByRole('textbox') as HTMLInputElement;
  fireEvent.change(input, { target: { value: 'Blue' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await waitFor(() => expect(onSave).toHaveBeenCalledWith('def-1', 'Blue'));
  // Edit mode closes once the save resolves.
  await waitFor(() =>
    expect(screen.queryByRole('button', { name: 'Save' })).not.toBeInTheDocument(),
  );
});

test('clearing the value to empty saves null (remove signal)', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderRow({ value: 'Teal', onSave });

  fireEvent.click(screen.getByLabelText('Edit'));
  const input = screen.getByRole('textbox') as HTMLInputElement;
  fireEvent.change(input, { target: { value: '' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await waitFor(() => expect(onSave).toHaveBeenCalledWith('def-1', null));
});

test('a save failure surfaces the error via the snackbar and keeps edit mode open', async () => {
  const onSave = vi.fn().mockRejectedValue(new Error('Network down'));
  renderRow({ value: 'Teal', onSave });

  fireEvent.click(screen.getByLabelText('Edit'));
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await waitFor(() => expect(screen.getByText('Network down')).toBeInTheDocument());
  // Still in edit mode: the Save button is still there.
  expect(screen.getByRole('button', { name: 'Save' })).toBeInTheDocument();
});

test('boolean definitions render a checkbox-style editor and save the boolean wire value', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  const def = stringDefinition({ id: 'def-bool', label: 'Newsletter', type: 'boolean' });
  renderRow({ definition: def, value: false, onSave });

  // Boolean false is a real value, so it displays as "false", not the em-dash.
  expect(screen.getByText('false')).toBeInTheDocument();

  fireEvent.click(screen.getByLabelText('Edit'));
  // MUI's Switch exposes role="switch", not "checkbox".
  fireEvent.click(screen.getByRole('switch'));
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await waitFor(() => expect(onSave).toHaveBeenCalledWith('def-bool', true));
});
