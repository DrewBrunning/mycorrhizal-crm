import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { MemoryRouter } from 'react-router';
import type { UpcomingOccasion } from '../api/occasionObligations';
import UpcomingOccasionsWidget from './UpcomingOccasionsWidget';

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

// Countdowns follow the viewer's local day (issue #2679), so pin it.
beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] });
  vi.setSystemTime(new Date(2026, 11, 20, 12, 0));
});

function renderWidget(props: Partial<React.ComponentProps<typeof UpcomingOccasionsWidget>> = {}) {
  const defaults: React.ComponentProps<typeof UpcomingOccasionsWidget> = {
    occasions: [],
    loading: false,
    error: null,
    days: 30,
    onDaysChange: vi.fn(),
    ...props,
  };
  return render(
    <MemoryRouter>
      <UpcomingOccasionsWidget {...defaults} />
    </MemoryRouter>,
  );
}

test('shows the empty state when there are no occasions', () => {
  renderWidget();
  expect(screen.getByText('No upcoming occasions in this window.')).toBeInTheDocument();
});

test('renders a birthday occasion with its contact name and days-until', () => {
  const occasions: UpcomingOccasion[] = [
    {
      contact_id: 1,
      contact_name: 'Alice',
      source: 'birthday',
      label: 'Alice',
      date: '2026-12-25',
      days_until: 5,
    },
  ];
  renderWidget({ occasions });
  expect(screen.getByText('Alice')).toBeInTheDocument();
  expect(screen.getByText('Birthday')).toBeInTheDocument();
  expect(screen.getByText('in 5 days')).toBeInTheDocument();
});

test('renders an obligation occasion with its label and kind', () => {
  const occasions: UpcomingOccasion[] = [
    {
      contact_id: 2,
      contact_name: 'Bob',
      source: 'obligation',
      label: 'Christmas card',
      date: '2026-12-20',
      days_until: 0,
      kind: 'card',
    },
  ];
  renderWidget({ occasions });
  expect(screen.getByText('Bob — Christmas card')).toBeInTheDocument();
  expect(screen.getByText('Card')).toBeInTheDocument();
  expect(screen.getByText('Today')).toBeInTheDocument();
});

test('calls onDaysChange when the 90-day toggle is clicked', () => {
  const onDaysChange = vi.fn();
  renderWidget({ onDaysChange });
  fireEvent.click(screen.getByRole('button', { name: '90 days' }));
  expect(onDaysChange).toHaveBeenCalledWith(90);
});

test('shows the error message when present', () => {
  renderWidget({ error: 'Something went wrong' });
  expect(screen.getByText('Something went wrong')).toBeInTheDocument();
});

// Issue #2679: the server counts days from its own REMINDER_TIMEZONE, so for a
// viewer in another zone its days_until can be a day off the date shown. The
// label follows the date against the viewer's own day.
test('labels from the date and the viewer day, not the server days_until', () => {
  const occasions: UpcomingOccasion[] = [
    {
      contact_id: 1,
      contact_name: 'Soren',
      source: 'birthday',
      label: 'Soren',
      date: '2026-12-20', // today on this device
      days_until: 1, // the server is already on the next day
    },
    {
      contact_id: 2,
      contact_name: 'Eve',
      source: 'birthday',
      label: 'Eve',
      date: '2026-12-21', // tomorrow on this device
      days_until: 0, // the server calls it today
    },
  ];
  renderWidget({ occasions });
  const soren = screen.getByText('Soren').closest('a') as HTMLElement;
  const eve = screen.getByText('Eve').closest('a') as HTMLElement;
  expect(soren).toHaveTextContent('Today');
  expect(eve).toHaveTextContent('in 1 day');
  expect(eve).not.toHaveTextContent('Today');
});
