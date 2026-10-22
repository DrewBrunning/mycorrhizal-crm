import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { CardAnniversary } from '../api/contacts';
import AnniversaryDialog, { anniversaryFromForm } from './AnniversaryDialog';

afterEach(cleanup);

function renderDialog(props: Partial<React.ComponentProps<typeof AnniversaryDialog>> = {}) {
  const defaults: React.ComponentProps<typeof AnniversaryDialog> = {
    open: true,
    onClose: vi.fn(),
    onSave: vi.fn().mockResolvedValue(undefined),
    ...props,
  };
  return render(<AnniversaryDialog {...defaults} />);
}

const existing: CardAnniversary = {
  id: 'ann-1',
  kind: 'wedding',
  date: { partial: { year: 2015, month: 6, day: 1 } },
  place: { full: 'Lisbon, Portugal' },
};

test('create mode defaults to a wedding anniversary with no date', () => {
  renderDialog();
  expect(screen.getByRole('heading', { name: 'Add anniversary' })).toBeInTheDocument();
  expect(screen.getByLabelText('Type')).toHaveTextContent('Wedding anniversary');
  expect(screen.getByLabelText('Date *')).toHaveValue('');
});

// Issue #1725: the accepted date shapes are easier to copy than to infer.
test('shows the accepted date shapes with literal examples', () => {
  renderDialog();

  expect(
    screen.getByText(
      'Use YYYY-MM-DD for a full date (e.g. 1990-06-15) or --MM-DD when the year is unknown (e.g. --12-25).',
    ),
  ).toBeInTheDocument();
});

test('requires a date', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(await screen.findByText('Enter a date.')).toBeInTheDocument();
  expect(onSave).not.toHaveBeenCalled();
});

test('rejects an unparseable date instead of storing an opaque timestamp', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  fireEvent.change(screen.getByLabelText('Date *'), { target: { value: 'June 1' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(await screen.findByText('Enter a date as YYYY-MM-DD or --MM-DD.')).toBeInTheDocument();
  expect(onSave).not.toHaveBeenCalled();
});

test('saves a full date', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  const onClose = vi.fn();
  renderDialog({ onSave, onClose });

  fireEvent.change(screen.getByLabelText('Date *'), { target: { value: '2015-06-01' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave).toHaveBeenCalledWith({ kind: 'wedding', date: '2015-06-01' });
  expect(onClose).toHaveBeenCalled();
});

test('saves a death anniversary with a year-less date', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  fireEvent.mouseDown(screen.getByLabelText('Type'));
  fireEvent.click(await screen.findByRole('option', { name: 'Date of death' }));
  fireEvent.change(screen.getByLabelText('Date *'), { target: { value: '--03-14' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave).toHaveBeenCalledWith({ kind: 'death', date: '--03-14' });
});

test('edit mode pre-fills the existing anniversary', () => {
  renderDialog({ anniversary: existing });
  expect(screen.getByRole('heading', { name: 'Edit anniversary' })).toBeInTheDocument();
  expect(screen.getByLabelText('Type')).toHaveTextContent('Wedding anniversary');
  expect(screen.getByLabelText('Date *')).toHaveValue('2015-06-01');
});

test('a failed save keeps the dialog open and shows an error', async () => {
  const onSave = vi.fn().mockRejectedValue(new Error('nope'));
  const onClose = vi.fn();
  renderDialog({ onSave, onClose, anniversary: existing });

  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(await screen.findByText("Couldn't save this anniversary.")).toBeInTheDocument();
  expect(onClose).not.toHaveBeenCalled();
});

test('anniversaryFromForm preserves the entry id and any fields it does not edit', () => {
  const saved = anniversaryFromForm(existing, { kind: 'death', date: '--03-14' });
  expect(saved.id).toBe('ann-1');
  expect(saved.place).toEqual({ full: 'Lisbon, Portugal' });
  expect(saved.kind).toBe('death');
  expect(saved.date).toEqual({ partial: { month: 3, day: 14 } });
});

test('anniversaryFromForm builds a new entry when there is no existing one', () => {
  const saved = anniversaryFromForm(null, { kind: 'wedding', date: '2015-06-01' });
  expect(saved.id).toBeUndefined();
  expect(saved.kind).toBe('wedding');
  expect(saved.date).toEqual({ partial: { year: 2015, month: 6, day: 1 } });
});
