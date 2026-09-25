import { cleanup, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, expect, test } from 'vitest';
import '../i18n/config';
import type { OverdueDataDecayPolicy } from '../api/dataDecayPolicies';
import { DateFormatProvider } from '../DateFormatProvider';
import DataDecayOverdueList from './DataDecayOverdueList';

afterEach(cleanup);

function overdueItem(overrides: Partial<OverdueDataDecayPolicy> = {}): OverdueDataDecayPolicy {
  return {
    policy: {
      id: 'policy-1',
      entity_id: 'alice-uid',
      interval_days: 365,
      active: true,
      created_at: '2025-01-01T00:00:00Z',
      updated_at: '2025-01-01T00:00:00Z',
    },
    health: { overdue_by: 12, next_due: '2026-01-20T00:00:00Z' },
    contact_id: 7,
    contact_name: 'Alice Smith',
    ...overrides,
  };
}

function renderList(props: Partial<React.ComponentProps<typeof DataDecayOverdueList>> = {}) {
  const defaults: React.ComponentProps<typeof DataDecayOverdueList> = {
    overdue: [],
    loading: false,
    error: null,
    ...props,
  };
  return render(
    <MemoryRouter>
      <DateFormatProvider>
        <DataDecayOverdueList {...defaults} />
      </DateFormatProvider>
    </MemoryRouter>,
  );
}

test('renders each overdue contact with the overdue badge and a link to the contact', () => {
  renderList({ overdue: [overdueItem()] });

  expect(screen.getByText('Alice Smith')).toBeInTheDocument();
  expect(screen.getByText('12 days overdue')).toBeInTheDocument();
  // The row is a router Link to /contacts/<numeric id>.
  const link = screen.getByRole('link');
  expect(link.getAttribute('href')).toBe('/contacts/7');
  // #196: the decorative avatar initial must not double into the link name.
  expect(link).toHaveAccessibleName(/^Alice Smith/); // not "AAlice Smith"
});

test('shows the empty state when nothing is overdue', () => {
  renderList({ overdue: [] });
  expect(screen.getByText("Nothing needs a check. You're all caught up.")).toBeInTheDocument();
});

test('falls back to the unknown-contact label when a contact name is missing', () => {
  renderList({ overdue: [overdueItem({ contact_name: '' })] });
  expect(screen.getByText('Unknown contact')).toBeInTheDocument();
});
