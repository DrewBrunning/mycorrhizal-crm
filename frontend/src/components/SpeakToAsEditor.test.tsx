import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent } from '@testing-library/react';
import '../i18n/config';
import SpeakToAsEditor from './SpeakToAsEditor';
import { CardSpeakToAs } from '../api/contacts';

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
