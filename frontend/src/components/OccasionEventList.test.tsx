import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { OccasionEvent } from '../api/occasionEvents';
import { setDateFormatPreference } from '../DateFormatProvider';
import { formatPreferredDateTime, formatTimeWith, setTimeFormat } from '../timeFormat';
import OccasionEventList, { formatEventWhen } from './OccasionEventList';

const formatDateTime = (value: string | number | Date) =>
  formatPreferredDateTime(value, 'eu', '24h', 'en-US');
const formatTime = (value: string | number | Date) =>
  formatTimeWith(value, '24h', 'en-US', { hour: 'numeric', minute: '2-digit' });
const when = (event: OccasionEvent) => formatEventWhen(event, formatDateTime, formatTime);

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

// Issue #1830: the occasion range follows the user's 12/24-hour preference.
test('the event range renders 24-hour times when the preference is 24h', () => {
  const withEnd = { ...baseEvent, ends_at: '2026-07-04T20:00:00Z' };
  try {
    setTimeFormat('24h');
    const text = formatEventWhen(withEnd);
    expect(text).not.toMatch(/AM|PM/i);
    expect(text.match(/\d{2}:\d{2}/g)).toHaveLength(2);
    renderList({ events: [withEnd] });
    expect(screen.getByText(text)).toBeInTheDocument();
  } finally {
    setTimeFormat('auto');
  }
});

test('the event range renders 12-hour times when the preference is 12h', () => {
  const withEnd = { ...baseEvent, ends_at: '2026-07-04T20:00:00Z' };
  try {
    setTimeFormat('12h');
    expect(formatEventWhen(withEnd).match(/AM|PM/gi)).toHaveLength(2);
  } finally {
    setTimeFormat('auto');
  }
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

// Issue #1789 (web counterpart of Android #1753): events are real instants, so
// they render in the local zone, and a range never repeats the start.
test('no end renders only the start', () => {
  expect(when({ ...baseEvent, starts_at: '2026-10-26T13:06:00' })).toBe('26.10.2026 13:06');
  expect(when({ ...baseEvent, starts_at: '2026-10-26T13:06:00', ends_at: '' })).toBe(
    '26.10.2026 13:06',
  );
});

test('a zero-length event shows no range, even with a different spelling of the same instant', () => {
  const start = '2026-10-26T13:06:00';
  expect(when({ ...baseEvent, starts_at: start, ends_at: start })).toBe('26.10.2026 13:06');
  expect(when({ ...baseEvent, starts_at: start, ends_at: '2026-10-26T13:06:00.000' })).toBe(
    '26.10.2026 13:06',
  );
});

test('a same-day event shows the date once, then the end time only', () => {
  expect(
    when({
      ...baseEvent,
      starts_at: '2026-10-26T13:06:00',
      ends_at: '2026-10-26T15:00:00',
    }),
  ).toBe('26.10.2026 13:06 – 15:00');
});

test('a multi-day event shows the full start and end', () => {
  expect(
    when({
      ...baseEvent,
      starts_at: '2026-10-26T13:06:00',
      ends_at: '2026-10-28T12:00:00',
    }),
  ).toBe('26.10.2026 13:06 – 28.10.2026 12:00');
});

test('an unparseable end falls back to the start rather than a broken range', () => {
  expect(when({ ...baseEvent, starts_at: '2026-10-26T13:06:00', ends_at: 'soon' })).toBe(
    '26.10.2026 13:06',
  );
});

test('the list uses the formatters bound to the user preferences', () => {
  const withEnd = {
    ...baseEvent,
    starts_at: '2026-10-26T13:06:00',
    ends_at: '2026-10-26T15:00:00',
  };
  setDateFormatPreference('eu');
  setTimeFormat('24h');
  try {
    renderList({ events: [withEnd] });
    expect(screen.getByText('26.10.2026 13:06 – 15:00')).toBeInTheDocument();
  } finally {
    setTimeFormat('auto');
    setDateFormatPreference('eu');
  }
});
