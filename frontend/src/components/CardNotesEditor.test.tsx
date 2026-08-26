import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { CardNote } from '../api/contacts';
import CardNotesEditor from './CardNotesEditor';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(cleanup);

function note(overrides: Partial<CardNote> = {}): CardNote {
  return { note: 'Remember the ceramics shop', ...overrides };
}

function renderEditor(props: Partial<React.ComponentProps<typeof CardNotesEditor>> = {}) {
  const defaults: React.ComponentProps<typeof CardNotesEditor> = {
    label: 'Notes',
    value: [],
    onChange: vi.fn(),
  };
  const merged = { ...defaults, ...props };
  render(<CardNotesEditor {...merged} />);
  return { onChange: merged.onChange };
}

test('renders the label and each note text', () => {
  renderEditor({ value: [note(), note({ note: 'Buy birthday flowers' })] });

  expect(screen.getByText('Notes')).toBeInTheDocument();
  expect(screen.getByDisplayValue('Remember the ceramics shop')).toBeInTheDocument();
  expect(screen.getByDisplayValue('Buy birthday flowers')).toBeInTheDocument();
});

test('editing a note reports the patched array', () => {
  const { onChange } = renderEditor({ value: [note()] });

  fireEvent.change(screen.getByLabelText('Note'), { target: { value: 'Changed note' } });

  expect(onChange).toHaveBeenCalledWith([{ note: 'Changed note' }]);
});

test('removing a row reports the array without it', () => {
  const second = note({ note: 'Second note' });
  const { onChange } = renderEditor({ value: [note(), second] });

  fireEvent.click(screen.getAllByLabelText('Delete')[0]);

  expect(onChange).toHaveBeenCalledWith([second]);
});

test('the add button appends an empty note row', () => {
  const { onChange } = renderEditor({ value: [note()] });

  fireEvent.click(screen.getByRole('button', { name: 'Add' }));

  expect(onChange).toHaveBeenCalledWith([note(), { note: '' }]);
});

test('renders author and created metadata as a caption', () => {
  renderEditor({
    value: [
      note({
        note: 'With provenance',
        author: { name: 'Alice' },
        created: { utc: '2026-01-01T00:00:00Z' },
      }),
    ],
  });

  expect(screen.getByText(/Alice · Created 2026-01-01T00:00:00Z/)).toBeInTheDocument();
});

test('a plain note renders no metadata caption', () => {
  renderEditor({ value: [note()] });

  expect(screen.queryByText(/Created/)).not.toBeInTheDocument();
});
