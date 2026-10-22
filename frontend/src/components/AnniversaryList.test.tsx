import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { CardAnniversary } from '../api/contacts';
import { DateFormatProvider } from '../DateFormatProvider';
import AnniversaryList from './AnniversaryList';

afterEach(() => {
  cleanup();
  window.localStorage.removeItem('dateFormat');
});

function renderList(
  anniversaries: CardAnniversary[],
  callbacks: {
    onEdit?: (a: CardAnniversary) => void;
    onDelete?: (a: CardAnniversary) => void;
  } = {},
) {
  const onEdit = callbacks.onEdit ?? vi.fn();
  const onDelete = callbacks.onDelete ?? vi.fn();
  render(
    <DateFormatProvider>
      <AnniversaryList anniversaries={anniversaries} onEdit={onEdit} onDelete={onDelete} />
    </DateFormatProvider>,
  );
  return { onEdit, onDelete };
}

const wedding: CardAnniversary = {
  id: 'ann-1',
  kind: 'wedding',
  date: { partial: { year: 2015, month: 6, day: 1 } },
};
const death: CardAnniversary = {
  id: 'ann-2',
  kind: 'death',
  date: { partial: { month: 3, day: 14 } },
};

test('shows the empty state when there are no non-birth anniversaries', () => {
  renderList([]);
  expect(screen.getByText('No anniversaries yet.')).toBeInTheDocument();
});

test('never renders the birthday even if handed the full Card array', () => {
  renderList([
    { id: 'birth', kind: 'birth', date: { partial: { year: 1990, month: 4, day: 30 } } },
  ]);
  expect(screen.getByText('No anniversaries yet.')).toBeInTheDocument();
  expect(screen.queryByText(/1990/)).not.toBeInTheDocument();
});

test('renders wedding and death entries with their kind and formatted date', () => {
  // Default provider format is "eu" (DD.MM.YYYY).
  renderList([wedding, death]);

  expect(screen.getByText('Wedding anniversary')).toBeInTheDocument();
  expect(screen.getByText('01.06.2015')).toBeInTheDocument();
  expect(screen.getByText('Date of death')).toBeInTheDocument();
});

test('honors a US date-format preference', () => {
  window.localStorage.setItem('dateFormat', 'us');
  renderList([wedding]);
  expect(screen.getByText('06/01/2015')).toBeInTheDocument();
  expect(screen.queryByText(/2015-06-01/)).toBeNull();
});

test('renders a place when the anniversary carries one', () => {
  renderList([
    {
      ...wedding,
      place: { full: 'Lisbon, Portugal' },
    },
  ]);
  expect(screen.getByText('Lisbon, Portugal')).toBeInTheDocument();
});

test('edit calls back with the exact entry', () => {
  const onEdit = vi.fn();
  renderList([wedding, death], { onEdit });

  fireEvent.click(screen.getAllByLabelText('Edit')[1]);

  expect(onEdit).toHaveBeenCalledWith(death);
});

test('delete confirms before calling back', () => {
  const confirm = vi.fn(() => true);
  vi.stubGlobal('confirm', confirm);
  const onDelete = vi.fn();
  renderList([wedding], { onDelete });

  fireEvent.click(screen.getByLabelText('Delete'));

  expect(confirm).toHaveBeenCalled();
  expect(onDelete).toHaveBeenCalledWith(wedding);
  vi.unstubAllGlobals();
});

test('declining the delete confirmation makes no callback', () => {
  vi.stubGlobal(
    'confirm',
    vi.fn(() => false),
  );
  const onDelete = vi.fn();
  renderList([wedding], { onDelete });

  fireEvent.click(screen.getByLabelText('Delete'));

  expect(onDelete).not.toHaveBeenCalled();
  vi.unstubAllGlobals();
});
