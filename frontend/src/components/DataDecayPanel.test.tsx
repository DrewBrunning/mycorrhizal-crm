import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { DataDecayPolicy } from '../api/dataDecayPolicies';
import { DateFormatProvider } from '../DateFormatProvider';
import DataDecayPanel from './DataDecayPanel';

afterEach(cleanup);

function renderPanel(
  policy: DataDecayPolicy | null,
  overrides: Partial<React.ComponentProps<typeof DataDecayPanel>> = {},
) {
  const defaults: React.ComponentProps<typeof DataDecayPanel> = {
    policy,
    loading: false,
    onAdd: vi.fn(),
    onEdit: vi.fn(),
    onDelete: vi.fn(),
    onVerify: vi.fn(),
    ...overrides,
  };
  return render(
    <DateFormatProvider>
      <DataDecayPanel {...defaults} />
    </DateFormatProvider>,
  );
}

function basePolicy(
  overrides: Partial<Pick<DataDecayPolicy, 'active' | 'last_verified_at' | 'health'>> = {},
): DataDecayPolicy {
  return {
    id: 'policy-1',
    entity_id: 'alice-uid',
    interval_days: 365,
    active: true,
    last_verified_at: null,
    created_at: '2025-01-01T00:00:00Z',
    updated_at: '2025-01-01T00:00:00Z',
    health: { next_due: '2026-01-01T00:00:00Z', overdue_by: 0 },
    ...overrides,
  };
}

test('no policy shows the empty state with an add button', () => {
  const onAdd = vi.fn();
  renderPanel(null, { onAdd });

  expect(screen.getByText('No verification schedule set for this contact.')).toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'Set up verification' }));
  expect(onAdd).toHaveBeenCalled();
});

test('overdue health renders the warning badge', () => {
  renderPanel(basePolicy({ health: { next_due: '2026-01-01T00:00:00Z', overdue_by: 3 } }));

  expect(screen.getByText('3 days overdue')).toBeInTheDocument();
  expect(screen.queryByText('Info recently verified')).not.toBeInTheDocument();
});

test('on-track health renders the success badge and next-due line', () => {
  renderPanel(basePolicy({ health: { next_due: '2026-02-01T00:00:00Z', overdue_by: 0 } }));

  expect(screen.getByText('Info recently verified')).toBeInTheDocument();
  expect(screen.queryByText('3 days overdue')).not.toBeInTheDocument();
});

test('never verified renders the neutral hint', () => {
  renderPanel(basePolicy({ last_verified_at: null }));
  expect(screen.getByText('Never verified')).toBeInTheDocument();
});

test('a verified policy shows the last-verified date, not the neutral hint', () => {
  renderPanel(basePolicy({ last_verified_at: '2026-01-05T00:00:00Z' }));
  expect(screen.queryByText('Never verified')).not.toBeInTheDocument();
});

test('a paused policy shows the paused badge', () => {
  renderPanel(basePolicy({ active: false }));
  expect(screen.getByText('Paused')).toBeInTheDocument();
});

test('confirm, edit and delete actions fire their handlers', () => {
  const onEdit = vi.fn();
  const onDelete = vi.fn();
  const onVerify = vi.fn();
  const policy = basePolicy();
  renderPanel(policy, { onEdit, onDelete, onVerify });

  fireEvent.click(screen.getByRole('button', { name: 'Confirm still current' }));
  expect(onVerify).toHaveBeenCalled();

  fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
  expect(onEdit).toHaveBeenCalledWith(policy);

  fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
  expect(onDelete).toHaveBeenCalledWith(policy.id);
});
