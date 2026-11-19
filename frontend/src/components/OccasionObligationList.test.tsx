import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { OccasionObligation } from '../api/occasionObligations';
import OccasionObligationList from './OccasionObligationList';

afterEach(cleanup);

const baseObligation: OccasionObligation = {
  id: 'o1',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  entity_id: 'alice-uid',
  kind: 'card',
  label: 'Christmas card',
  anchor_month: 12,
  anchor_day: 25,
  lead_time_days: 0,
  active: true,
  sensitivity: 'normal',
};

test('shows the empty state when there are no obligations', () => {
  render(<OccasionObligationList obligations={[]} onEdit={vi.fn()} onDelete={vi.fn()} />);
  expect(
    screen.getByText('No standing occasions yet. Add one for a recurring card, gift, or invite.'),
  ).toBeInTheDocument();
});

test('renders the label, kind, and anchor date', () => {
  render(
    <OccasionObligationList obligations={[baseObligation]} onEdit={vi.fn()} onDelete={vi.fn()} />,
  );
  expect(screen.getByText('Christmas card')).toBeInTheDocument();
  expect(screen.getByText('Card')).toBeInTheDocument();
  expect(screen.getByText('December 25')).toBeInTheDocument();
});

test('says a legacy dateless occasion has no date and no reminder (#2670)', () => {
  render(
    <OccasionObligationList
      obligations={[{ ...baseObligation, anchor_month: undefined, anchor_day: undefined }]}
      onEdit={vi.fn()}
      onDelete={vi.fn()}
    />,
  );
  expect(screen.getByText('No date \u2014 no reminder')).toBeInTheDocument();
});

test('shows an Inactive chip for a paused obligation', () => {
  render(
    <OccasionObligationList
      obligations={[{ ...baseObligation, active: false }]}
      onEdit={vi.fn()}
      onDelete={vi.fn()}
    />,
  );
  expect(screen.getByText('Inactive')).toBeInTheDocument();
});

test('calls onEdit when the edit button is clicked', () => {
  const onEdit = vi.fn();
  render(
    <OccasionObligationList obligations={[baseObligation]} onEdit={onEdit} onDelete={vi.fn()} />,
  );
  fireEvent.click(screen.getByRole('button', { name: /^Edit\b/ }));
  expect(onEdit).toHaveBeenCalledWith(baseObligation);
});

test('confirms before calling onDelete', () => {
  const onDelete = vi.fn();
  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
  render(
    <OccasionObligationList obligations={[baseObligation]} onEdit={vi.fn()} onDelete={onDelete} />,
  );
  fireEvent.click(screen.getByRole('button', { name: /^Delete\b/ }));
  expect(confirmSpy).toHaveBeenCalledWith('Delete this occasion?');
  expect(onDelete).toHaveBeenCalledWith('o1');
  confirmSpy.mockRestore();
});

test('does not call onDelete when the confirm is dismissed', () => {
  const onDelete = vi.fn();
  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false);
  render(
    <OccasionObligationList obligations={[baseObligation]} onEdit={vi.fn()} onDelete={onDelete} />,
  );
  fireEvent.click(screen.getByRole('button', { name: /^Delete\b/ }));
  expect(onDelete).not.toHaveBeenCalled();
  confirmSpy.mockRestore();
});

// Issue #2681: the date and the lead time ran together as "December 25 14 days
// lead time".
test('separates the anchor date from the lead time', () => {
  render(
    <OccasionObligationList
      obligations={[{ ...baseObligation, lead_time_days: 14 }]}
      onEdit={vi.fn()}
      onDelete={vi.fn()}
    />,
  );
  const leadTime = screen.getByText(/14 days lead time/);
  expect(leadTime.textContent).toMatch(/^·\s+14 days lead time$/);
});

test('separates the lead time from the "No date" label of a dateless row', () => {
  render(
    <OccasionObligationList
      obligations={[
        { ...baseObligation, anchor_month: undefined, anchor_day: undefined, lead_time_days: 14 },
      ]}
      onEdit={vi.fn()}
      onDelete={vi.fn()}
    />,
  );
  // #2670: the "No date" label now precedes the lead time, so the dot belongs.
  expect(screen.getByText('No date \u2014 no reminder')).toBeInTheDocument();
  expect(screen.getByText('14 days lead time').textContent).toContain('·');
});

// #2673: every row's Edit/Delete icon button names its item.
test('edit and delete buttons name the obligation they act on', () => {
  render(
    <OccasionObligationList obligations={[baseObligation]} onEdit={vi.fn()} onDelete={vi.fn()} />,
  );
  expect(screen.getByRole('button', { name: 'Edit Christmas card' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Delete Christmas card' })).toBeInTheDocument();
});
