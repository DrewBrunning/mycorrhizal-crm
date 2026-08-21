import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import '../i18n/config';
import MultiValueField from './MultiValueField';
import { ContactValue } from '../api/contacts';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(cleanup);

function value(overrides: Partial<ContactValue> = {}): ContactValue {
  return { type: 'home', value: 'alice@example.com', ...overrides };
}

function renderEditor(props: Partial<React.ComponentProps<typeof MultiValueField>> = {}) {
  const defaults: React.ComponentProps<typeof MultiValueField> = {
    label: 'Email',
    value: [],
    onChange: vi.fn(),
  };
  const merged = { ...defaults, ...props };
  render(<MultiValueField {...merged} />);
  return { onChange: merged.onChange };
}

test('renders the label and each row type and value', () => {
  renderEditor({
    value: [value(), value({ type: 'work', value: 'bob@example.com' })],
  });

  expect(screen.getAllByText('Email').length).toBeGreaterThan(0);
  expect(screen.getByDisplayValue('alice@example.com')).toBeInTheDocument();
  expect(screen.getByDisplayValue('bob@example.com')).toBeInTheDocument();
});

test('editing a row value reports the patched array', () => {
  const { onChange } = renderEditor({ value: [value()] });

  fireEvent.change(screen.getByLabelText('Email'), { target: { value: 'new@example.com' } });

  expect(onChange).toHaveBeenCalledWith([{ type: 'home', value: 'new@example.com' }]);
});

test('the add button appends a row with the default type', () => {
  const { onChange } = renderEditor({ value: [value()] });

  fireEvent.click(screen.getByRole('button', { name: 'Add' }));

  expect(onChange).toHaveBeenCalledWith([value(), { type: 'home', value: '' }]);
});

test('removing a row reports the array without it', () => {
  const work = value({ type: 'work', value: 'bob@example.com' });
  const { onChange } = renderEditor({ value: [value(), work] });

  fireEvent.click(screen.getAllByLabelText('Delete')[0]);

  expect(onChange).toHaveBeenCalledWith([work]);
});

test('the star toggles preferred and enforces a single preferred row', () => {
  const { onChange } = renderEditor({
    value: [
      { type: 'home', value: 'a@example.com', pref: 1 },
      { type: 'work', value: 'b@example.com' },
    ],
  });

  // Row 0 is preferred; clicking its star clears the preference.
  fireEvent.click(screen.getByLabelText('Preferred'));
  expect(onChange).toHaveBeenLastCalledWith([
    { type: 'home', value: 'a@example.com', pref: undefined },
    { type: 'work', value: 'b@example.com' },
  ]);

  // Clicking row 1's star demotes row 0 and promotes row 1.
  fireEvent.click(screen.getAllByLabelText('Set as preferred')[0]);
  expect(onChange).toHaveBeenLastCalledWith([
    { type: 'home', value: 'a@example.com', pref: undefined },
    { type: 'work', value: 'b@example.com', pref: 1 },
  ]);
});

test('typing in the type autocomplete updates the row type', () => {
  const { onChange } = renderEditor({ value: [value()] });

  fireEvent.change(screen.getByLabelText('Type'), { target: { value: 'work' } });

  expect(onChange).toHaveBeenCalledWith([{ type: 'work', value: 'alice@example.com' }]);
});

test('free-text type mode renders a plain input and accepts custom tokens', () => {
  const onChange = vi.fn();

  render(
    <MultiValueField
      label="IMPP"
      value={[{ type: 'impp-x', value: 'user@example.com' }]}
      onChange={onChange}
      freeTextType
    />
  );

  const typeInput = screen.getByLabelText('Type');
  expect(typeInput).toHaveValue('impp-x');

  fireEvent.change(typeInput, { target: { value: 'impp-y' } });
  expect(onChange).toHaveBeenCalledWith([{ type: 'impp-y', value: 'user@example.com' }]);
});
