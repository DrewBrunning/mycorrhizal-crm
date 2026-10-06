import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';
import '../i18n/config';
import { SnackbarProvider } from '../context/SnackbarContext';
import EditableField from './EditableField';

afterEach(cleanup);

function setup(overrides: Partial<React.ComponentProps<typeof EditableField>> = {}) {
  const handlers = {
    onEditStart: vi.fn(),
    onEditCancel: vi.fn(),
    onEditSave: vi.fn(),
    onEditValueChange: vi.fn(),
  };
  const props: React.ComponentProps<typeof EditableField> = {
    icon: <span data-testid="icon" />,
    label: 'Nickname',
    field: 'nickname',
    value: 'Ali',
    isEditing: false,
    editValue: '',
    validationError: '',
    ...handlers,
    ...overrides,
  };
  render(
    <SnackbarProvider>
      <EditableField {...props} />
    </SnackbarProvider>,
  );
  return handlers;
}

describe('EditableField display mode', () => {
  test('shows the label, icon and value, and a copy button for a non-empty value', () => {
    setup();
    expect(screen.getByText('Nickname')).toBeInTheDocument();
    expect(screen.getByTestId('icon')).toBeInTheDocument();
    expect(screen.getByText('Ali')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /copy/i })).toBeInTheDocument();
  });

  test('shows a dash and no copy button when empty', () => {
    setup({ value: '' });
    expect(screen.getByText('-')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /copy/i })).not.toBeInTheDocument();
  });

  test('a formatted display value replaces the raw value in the text only', () => {
    const h = setup({ value: '1990-05-04', formattedDisplayValue: 'May 4, 1990' });
    expect(screen.getByText('May 4, 1990')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    expect(h.onEditStart).toHaveBeenCalledWith('nickname', '1990-05-04');
  });

  test('appends the suffix to a non-empty value but not to the empty dash', () => {
    cleanup();
    setup({ value: '30', displaySuffix: 'years' });
    expect(screen.getByText('30 years')).toBeInTheDocument();
    cleanup();
    setup({ value: '', displaySuffix: 'years' });
    expect(screen.getByText('-')).toBeInTheDocument();
  });

  test('the Edit pencil starts editing with the field name and the raw value', () => {
    const h = setup();
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    expect(h.onEditStart).toHaveBeenCalledWith('nickname', 'Ali');
  });

  test('multiline values preserve line breaks; single-line ones do not', () => {
    setup({ value: 'a\nb', multiline: true });
    expect(screen.getByText(/a\s*b/)).toHaveStyle({ whiteSpace: 'pre-wrap' });
    cleanup();
    setup({ value: 'x' });
    expect(screen.getByText('x')).not.toHaveStyle({ whiteSpace: 'pre-wrap' });
  });
});

describe('EditableField edit mode', () => {
  test('hides the pencil/copy and shows a text box that reports changes', () => {
    const h = setup({ isEditing: true, editValue: 'Al' });
    expect(screen.queryByRole('button', { name: 'Edit' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /copy/i })).not.toBeInTheDocument();
    const input = screen.getByDisplayValue('Al');
    fireEvent.change(input, { target: { value: 'Alex' } });
    expect(h.onEditValueChange).toHaveBeenCalledWith('Alex');
  });

  test('Save reports the field name; Cancel reports cancel', () => {
    const h = setup({ isEditing: true, editValue: 'x' });
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    expect(h.onEditSave).toHaveBeenCalledWith('nickname');
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(h.onEditCancel).toHaveBeenCalledTimes(1);
  });

  test('multiline renders a 3-row textarea', () => {
    setup({ isEditing: true, editValue: 'x', multiline: true });
    expect(screen.getByRole('textbox')).toHaveAttribute('rows', '3');
  });

  test('shows the placeholder', () => {
    setup({ isEditing: true, editValue: '', placeholder: 'Type here' });
    expect(screen.getByPlaceholderText('Type here')).toBeInTheDocument();
  });

  test('shows the validation error only while editing', () => {
    setup({ isEditing: true, editValue: 'x', validationError: 'Too short' });
    expect(screen.getByText('Too short')).toBeInTheDocument();
    cleanup();
    setup({ isEditing: false, validationError: 'Too short' });
    expect(screen.queryByText('Too short')).not.toBeInTheDocument();
  });

  test('with options it renders a free-solo autocomplete that offers and accepts suggestions', async () => {
    const h = setup({
      isEditing: true,
      editValue: '',
      options: ['alpha', 'beta'],
      getOptionLabel: (o) => o.toUpperCase(),
      placeholder: 'pick',
    });
    const input = screen.getByPlaceholderText('pick');
    fireEvent.mouseDown(input);
    fireEvent.click(await screen.findByRole('option', { name: 'BETA' }));
    expect(h.onEditValueChange).toHaveBeenCalledWith('beta');
  });

  test('autocomplete accepts free text verbatim and clearing reports an empty string', () => {
    const h = setup({ isEditing: true, editValue: 'alpha', options: ['alpha'] });
    const input = screen.getByRole('combobox');
    fireEvent.change(input, { target: { value: 'custom' } });
    expect(h.onEditValueChange).toHaveBeenCalledWith('custom');
    fireEvent.change(input, { target: { value: '' } });
    expect(h.onEditValueChange).toHaveBeenCalledWith('');
  });

  test('autocomplete default label is the option itself', async () => {
    setup({ isEditing: true, editValue: '', options: ['gamma'] });
    fireEvent.mouseDown(screen.getByRole('combobox'));
    expect(await screen.findByRole('option', { name: 'gamma' })).toBeInTheDocument();
  });
});
