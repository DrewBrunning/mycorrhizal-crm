import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import '../i18n/config';
import KeywordsEditor from './KeywordsEditor';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(cleanup);

function renderEditor(props: Partial<React.ComponentProps<typeof KeywordsEditor>> = {}) {
  const defaults: React.ComponentProps<typeof KeywordsEditor> = {
    label: 'Keywords',
    value: [],
    onChange: vi.fn(),
  };
  const merged = { ...defaults, ...props };
  render(<KeywordsEditor {...merged} />);
  return { onChange: merged.onChange };
}

test('renders the label and existing keywords as chips', () => {
  renderEditor({ value: ['pottery', 'climbing'] });

  expect(screen.getByText('Keywords')).toBeInTheDocument();
  expect(screen.getByText('pottery')).toBeInTheDocument();
  expect(screen.getByText('climbing')).toBeInTheDocument();
});

test('pressing Enter adds the keyword', () => {
  const { onChange } = renderEditor({ value: ['pottery'] });

  const input = screen.getByPlaceholderText('Type a keyword and press Enter');
  fireEvent.change(input, { target: { value: 'climbing' } });
  fireEvent.keyDown(input, { key: 'Enter' });

  expect(onChange).toHaveBeenCalledWith(['pottery', 'climbing']);
});

test('clicking Add adds the keyword', () => {
  const { onChange } = renderEditor();

  fireEvent.change(screen.getByPlaceholderText('Type a keyword and press Enter'), {
    target: { value: 'climbing' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Add' }));

  expect(onChange).toHaveBeenCalledWith(['climbing']);
});

test('the Add button is disabled while the draft is blank', () => {
  renderEditor();

  const addButton = screen.getByRole('button', { name: 'Add' });
  expect(addButton).toBeDisabled();

  fireEvent.change(screen.getByPlaceholderText('Type a keyword and press Enter'), {
    target: { value: '  ' },
  });
  expect(addButton).toBeDisabled();
});

test('blank and duplicate keywords are ignored but the draft clears', () => {
  const { onChange } = renderEditor({ value: ['pottery'] });

  const input = screen.getByPlaceholderText('Type a keyword and press Enter');

  fireEvent.change(input, { target: { value: '   ' } });
  fireEvent.keyDown(input, { key: 'Enter' });
  expect(onChange).not.toHaveBeenCalled();
  expect(input).toHaveValue('');

  fireEvent.change(input, { target: { value: 'pottery' } });
  fireEvent.keyDown(input, { key: 'Enter' });
  expect(onChange).not.toHaveBeenCalled();
  expect(input).toHaveValue('');
});

test('deleting a chip removes that keyword', () => {
  const { onChange } = renderEditor({ value: ['pottery', 'climbing'] });

  fireEvent.click(screen.getAllByTestId('CancelIcon')[0]);

  expect(onChange).toHaveBeenCalledWith(['climbing']);
});
