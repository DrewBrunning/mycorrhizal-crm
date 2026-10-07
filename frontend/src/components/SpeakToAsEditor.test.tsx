import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { useState } from 'react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { CardSpeakToAs } from '../api/contacts';
import SpeakToAsEditor from './SpeakToAsEditor';

afterEach(cleanup);

test('renders existing pronouns and grammatical genders', () => {
  const value: CardSpeakToAs = {
    pronouns: [{ pronouns: 'they/them' }],
    grammaticalGenders: [{ value: 'feminine', language: 'fr' }],
  };
  render(<SpeakToAsEditor value={value} onChange={vi.fn()} />);
  expect(screen.getByDisplayValue('they/them')).toBeInTheDocument();
  expect(screen.getByDisplayValue('fr')).toBeInTheDocument();
});

test('adds a new pronoun row and reports it upward', () => {
  const onChange = vi.fn();
  const value: CardSpeakToAs = { pronouns: [], grammaticalGenders: [] };
  render(<SpeakToAsEditor value={value} onChange={onChange} />);

  const addButtons = screen.getAllByRole('button', { name: 'Add' });
  fireEvent.click(addButtons[0]);

  expect(onChange).toHaveBeenCalledWith({
    pronouns: [{ pronouns: '' }],
    grammaticalGenders: [],
  });
});

test('removing a grammatical gender reports the filtered list upward', () => {
  const onChange = vi.fn();
  const value: CardSpeakToAs = {
    pronouns: [],
    grammaticalGenders: [{ value: 'masculine' }, { value: 'neuter' }],
  };
  render(<SpeakToAsEditor value={value} onChange={onChange} />);

  const deleteButtons = screen.getAllByLabelText('Delete');
  fireEvent.click(deleteButtons[0]);

  expect(onChange).toHaveBeenCalledWith({
    pronouns: [],
    grammaticalGenders: [{ value: 'neuter' }],
  });
});

// Save-payload tests (issue #1482).
function Harness({
  initial,
  onChange,
}: {
  initial: CardSpeakToAs;
  onChange: (next: CardSpeakToAs) => void;
}) {
  const [value, setValue] = useState(initial);
  return (
    <SpeakToAsEditor
      value={value}
      onChange={(n) => {
        setValue(n);
        onChange(n);
      }}
    />
  );
}
const last = (fn: ReturnType<typeof vi.fn>) => fn.mock.calls[fn.mock.calls.length - 1][0];

test('tolerates a value with neither list present (omitted by the backend)', () => {
  const onChange = vi.fn();
  render(<SpeakToAsEditor value={{}} onChange={onChange} />);
  fireEvent.click(screen.getAllByRole('button', { name: 'Add' })[1]);
  expect(onChange).toHaveBeenCalledWith({ grammaticalGenders: [{ value: '' }] });
});

test('adding a gender row keeps pronouns untouched', () => {
  const onChange = vi.fn();
  render(
    <Harness
      initial={{ pronouns: [{ pronouns: 'she/her' }], grammaticalGenders: [] }}
      onChange={onChange}
    />,
  );
  fireEvent.click(screen.getAllByRole('button', { name: 'Add' })[1]);
  expect(last(onChange)).toEqual({
    pronouns: [{ pronouns: 'she/her' }],
    grammaticalGenders: [{ value: '' }],
  });
});

test('editing a pronoun reports the new text and keeps its contexts', () => {
  const onChange = vi.fn();
  render(
    <Harness
      initial={{ pronouns: [{ pronouns: 'they', contexts: ['work'] }], grammaticalGenders: [] }}
      onChange={onChange}
    />,
  );
  fireEvent.change(screen.getByDisplayValue('they'), { target: { value: 'they/them' } });
  expect(last(onChange)).toEqual({
    pronouns: [{ pronouns: 'they/them', contexts: ['work'] }],
    grammaticalGenders: [],
  });
});

test('removing a pronoun reports the filtered list', () => {
  const onChange = vi.fn();
  render(
    <Harness
      initial={{ pronouns: [{ pronouns: 'a' }, { pronouns: 'b' }], grammaticalGenders: [] }}
      onChange={onChange}
    />,
  );
  fireEvent.click(screen.getAllByLabelText('Delete')[0]);
  expect(last(onChange)).toEqual({ pronouns: [{ pronouns: 'b' }], grammaticalGenders: [] });
});

test('picking a context chip via the autocomplete reports contexts', async () => {
  const onChange = vi.fn();
  render(
    <Harness
      initial={{ pronouns: [{ pronouns: 'x' }], grammaticalGenders: [] }}
      onChange={onChange}
    />,
  );
  const input = screen.getByLabelText('Contexts');
  fireEvent.mouseDown(input);
  fireEvent.click(await screen.findByRole('option', { name: /work/i }));
  expect(last(onChange)).toEqual({
    pronouns: [{ pronouns: 'x', contexts: ['work'] }],
    grammaticalGenders: [],
  });
});

test('selecting a grammatical gender and typing a language report both', async () => {
  const onChange = vi.fn();
  render(
    <Harness initial={{ pronouns: [], grammaticalGenders: [{ value: '' }] }} onChange={onChange} />,
  );
  fireEvent.mouseDown(screen.getByRole('combobox'));
  fireEvent.click(await screen.findByRole('option', { name: 'Feminine' }));
  expect(last(onChange)).toEqual({ pronouns: [], grammaticalGenders: [{ value: 'feminine' }] });
  fireEvent.change(screen.getByLabelText('Language'), { target: { value: 'fr' } });
  expect(last(onChange)).toEqual({
    pronouns: [],
    grammaticalGenders: [{ value: 'feminine', language: 'fr' }],
  });
});
