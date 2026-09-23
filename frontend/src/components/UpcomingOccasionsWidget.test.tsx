import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { MemoryRouter } from 'react-router';
import type { UpcomingOccasion } from '../api/occasionObligations';
import UpcomingOccasionsWidget from './UpcomingOccasionsWidget';

afterEach(cleanup);

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
      date: '2026-12-25',
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
