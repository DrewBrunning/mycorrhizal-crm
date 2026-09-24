import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { OccasionEvent } from '../api/occasionEvents';
import OccasionEventList, { formatEventWhen } from './OccasionEventList';

afterEach(cleanup);

const baseEvent: OccasionEvent = {
  id: 'ev-1',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  title: 'Summer BBQ',
  starts_at: '2026-07-04T15:00:00Z',
  sensitivity: 'normal',
};

function renderList(overrides: Partial<React.ComponentProps<typeof OccasionEventList>> = {}) {
  return render(
    <OccasionEventList
      events={[baseEvent]}
      onEdit={vi.fn()}
      onDelete={vi.fn()}
      onManageAttendees={vi.fn()}
      {...overrides}
    />,
  );
}

test('shows the empty state', () => {
  render(
    <OccasionEventList
      events={[]}
      onEdit={vi.fn()}
      onDelete={vi.fn()}
      onManageAttendees={vi.fn()}
    />,
  );
  expect(
    screen.getByText('No events yet. Create one to start planning who to invite.'),
  ).toBeInTheDocument();
});

test('renders the title, when, and location', () => {
  renderList({ events: [{ ...baseEvent, location: 'The park' }] });
  expect(screen.getByText('Summer BBQ')).toBeInTheDocument();
  expect(screen.getByText(formatEventWhen(baseEvent))).toBeInTheDocument();
  expect(screen.getByText('The park')).toBeInTheDocument();
});

test('renders an end time when present', () => {
  const withEnd = { ...baseEvent, ends_at: '2026-07-04T20:00:00Z' };
  renderList({ events: [withEnd] });
  expect(screen.getByText(formatEventWhen(withEnd))).toHaveTextContent('–');
});

test('shows a sensitivity chip for a non-normal event', () => {
  renderList({ events: [{ ...baseEvent, sensitivity: 'secret' }] });
  expect(screen.getByText('Secret')).toBeInTheDocument();
});

test('calls onEdit and onManageAttendees', () => {
  const onEdit = vi.fn();
  const onManageAttendees = vi.fn();
  renderList({ onEdit, onManageAttendees });
  fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
  expect(onEdit).toHaveBeenCalledWith(baseEvent);
  fireEvent.click(screen.getByRole('button', { name: 'Attendees' }));
  expect(onManageAttendees).toHaveBeenCalledWith(baseEvent);
});

test('confirms before deleting', () => {
  const onDelete = vi.fn();
  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
  renderList({ onDelete });
  fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
  expect(confirmSpy).toHaveBeenCalledWith('Delete this event?');
  expect(onDelete).toHaveBeenCalledWith('ev-1');
  confirmSpy.mockRestore();
});

test('does not delete when the confirm is dismissed', () => {
  const onDelete = vi.fn();
  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false);
  renderList({ onDelete });
  fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
  expect(onDelete).not.toHaveBeenCalled();
  confirmSpy.mockRestore();
});
