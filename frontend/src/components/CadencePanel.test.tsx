import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { CadencePolicy } from '../api/cadencePolicies';
import { DateFormatProvider } from '../DateFormatProvider';
import CadencePanel from './CadencePanel';

afterEach(cleanup);

function renderPanel(
  policy: CadencePolicy | null,
  overrides: Partial<React.ComponentProps<typeof CadencePanel>> = {},
) {
  const defaults: React.ComponentProps<typeof CadencePanel> = {
    policy,
    loading: false,
    onAdd: vi.fn(),
    onEdit: vi.fn(),
    onDelete: vi.fn(),
    ...overrides,
  };
  return render(
    <DateFormatProvider>
      <CadencePanel {...defaults} />
    </DateFormatProvider>,
  );
}

function basePolicy(health: CadencePolicy['health']): CadencePolicy {
  return {
    id: 'policy-1',
    entity_id: 'alice-uid',
    target_interval_days: 30,
    qualifying_types: ['call'],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    health,
  };
}

test('no policy shows the empty state with an add button', () => {
  const onAdd = vi.fn();
  renderPanel(null, { onAdd });

  expect(screen.getByText('No cadence set for this contact.')).toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'Add Cadence' }));
  expect(onAdd).toHaveBeenCalled();
});

test('overdue health renders the warning badge', () => {
  renderPanel(
    basePolicy({
      has_qualifying_interaction: true,
      overdue_by: 3,
      ambient: false,
      next_due: '2026-01-01T00:00:00Z',
      last_interaction: '2025-12-01T00:00:00Z',
    }),
  );

  expect(screen.getByText('3 days overdue')).toBeInTheDocument();
  expect(screen.queryByText('On track')).not.toBeInTheDocument();
});

test('on-track health renders the success badge and next-due line', () => {
  renderPanel(
    basePolicy({
      has_qualifying_interaction: true,
      overdue_by: 0,
      ambient: false,
      next_due: '2026-02-01T00:00:00Z',
      last_interaction: '2026-01-02T00:00:00Z',
    }),
  );

  expect(screen.getByText('On track')).toBeInTheDocument();
  expect(screen.queryByText('3 days overdue')).not.toBeInTheDocument();
});

test('no qualifying interactions yet renders the neutral hint, not overdue', () => {
  renderPanel(basePolicy({ has_qualifying_interaction: false, overdue_by: 0, ambient: false }));

  expect(
    screen.getByText('No qualifying interactions yet — cadence starts once you record one.'),
  ).toBeInTheDocument();
  expect(screen.queryByText('On track')).not.toBeInTheDocument();
  expect(screen.queryByText('3 days overdue')).not.toBeInTheDocument();
});

test('edit and delete actions fire their handlers', () => {
  const onEdit = vi.fn();
  const onDelete = vi.fn();
  const policy = basePolicy({ has_qualifying_interaction: false, overdue_by: 0, ambient: false });
  renderPanel(policy, { onEdit, onDelete });

  fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
  expect(onEdit).toHaveBeenCalledWith(policy);

  fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
  expect(onDelete).toHaveBeenCalledWith(policy.id);
});

// Issue #1735 (backend #1733): an ambient contact never reads as overdue and
// explains why instead of showing day counts.
test.each([
  ['household', 'Ambient contact (lives with you)'],
  ['flag', 'Ambient contact (marked)'],
] as const)('ambient health (%s) shows its reason, never overdue', (reason, text) => {
  renderPanel(
    basePolicy({
      has_qualifying_interaction: false,
      overdue_by: 0,
      ambient: true,
      ambient_reason: reason,
    }),
  );

  expect(screen.getByText(text)).toBeInTheDocument();
  expect(screen.queryByText(/days overdue/)).not.toBeInTheDocument();
  expect(screen.queryByText('On track')).not.toBeInTheDocument();
  expect(screen.queryByText(/No qualifying interactions yet/)).not.toBeInTheDocument();
});

test('ambient wins even if a stale overdue_by slips through', () => {
  renderPanel(
    basePolicy({
      has_qualifying_interaction: true,
      overdue_by: 9,
      ambient: true,
      ambient_reason: 'flag',
      next_due: '2026-01-01T00:00:00Z',
      last_interaction: '2025-12-01T00:00:00Z',
    }),
  );

  expect(screen.queryByText(/days overdue/)).not.toBeInTheDocument();
  expect(screen.queryByText(/Next due/)).not.toBeInTheDocument();
  expect(screen.getByText('Ambient contact (marked)')).toBeInTheDocument();
});
