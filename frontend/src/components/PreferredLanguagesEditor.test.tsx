import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { CardLanguagePref } from '../api/contacts';
import PreferredLanguagesEditor from './PreferredLanguagesEditor';

// This codebase's vitest setup has no auto-cleanup and no globals: true.
afterEach(cleanup);

function renderEditor(props: Partial<React.ComponentProps<typeof PreferredLanguagesEditor>> = {}) {
  const defaults: React.ComponentProps<typeof PreferredLanguagesEditor> = {
    label: 'Preferred Languages',
    value: [],
    onChange: vi.fn(),
  };
  const merged = { ...defaults, ...props };
  render(<PreferredLanguagesEditor {...merged} />);
  return { onChange: merged.onChange };
}

test('renders the label and no rows when the value is empty', () => {
  renderEditor();

  expect(screen.getByText('Preferred Languages')).toBeInTheDocument();
  expect(screen.queryByLabelText('Language')).not.toBeInTheDocument();
});

test('renders one row per existing language preference', () => {
  const value: CardLanguagePref[] = [{ language: 'en' }, { language: 'fr-CA' }];
  renderEditor({ value });

  const inputs = screen.getAllByLabelText('Language') as HTMLInputElement[];
  expect(inputs).toHaveLength(2);
  expect(inputs[0].value).toBe('en');
  expect(inputs[1].value).toBe('fr-CA');
});

test('adding a row appends a new blank language preference', () => {
  const { onChange } = renderEditor({ value: [{ language: 'en' }] });

  fireEvent.click(screen.getByRole('button', { name: 'Add' }));

  expect(onChange).toHaveBeenCalledWith([{ language: 'en' }, { language: '' }]);
});

test('editing the language field of a row updates only that row', () => {
  const value: CardLanguagePref[] = [{ language: 'en' }, { language: 'de' }];
  const { onChange } = renderEditor({ value });

  const inputs = screen.getAllByLabelText('Language');
  fireEvent.change(inputs[1], { target: { value: 'fr' } });

  expect(onChange).toHaveBeenCalledWith([{ language: 'en' }, { language: 'fr' }]);
});

test('removing a row drops that entry and leaves the others intact', () => {
  const value: CardLanguagePref[] = [{ language: 'en' }, { language: 'de' }];
  const { onChange } = renderEditor({ value });

  const deleteButtons = screen.getAllByLabelText('Delete');
  fireEvent.click(deleteButtons[0]);

  expect(onChange).toHaveBeenCalledWith([{ language: 'de' }]);
});

test('selecting a context option on a row updates that row’s contexts', async () => {
  const value: CardLanguagePref[] = [{ language: 'en', contexts: [] }];
  const { onChange } = renderEditor({ value });

  fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Contexts' }));
  fireEvent.click(await screen.findByRole('option', { name: 'Work' }));

  expect(onChange).toHaveBeenCalledWith([{ language: 'en', contexts: ['work'] }]);
});

test('removing every row leaves the add button as the only control', () => {
  const value: CardLanguagePref[] = [{ language: 'en' }];
  const { onChange } = renderEditor({ value });

  fireEvent.click(screen.getByLabelText('Delete'));

  expect(onChange).toHaveBeenCalledWith([]);
});
