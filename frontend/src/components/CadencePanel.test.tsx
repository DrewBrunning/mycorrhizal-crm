import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent } from '@testing-library/react';
import '../i18n/config';
import CadencePanel from './CadencePanel';
import { CadencePolicy } from '../api/cadencePolicies';
import { DateFormatProvider } from '../DateFormatProvider';

afterEach(cleanup);

function renderPanel(policy: CadencePolicy | null, overrides: Partial<React.ComponentProps<typeof CadencePanel>> = {}) {
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
    </DateFormatProvider>
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
  renderPanel(basePolicy({ has_qualifying_interaction: true, overdue_by: 3, next_due: '2026-01-01T00:00:00Z', last_interaction: '2025-12-01T00:00:00Z' }));

  expect(screen.getByText('3 days overdue')).toBeInTheDocument();
  expect(screen.queryByText('On track')).not.toBeInTheDocument();
});

test('on-track health renders the success badge and next-due line', () => {
  renderPanel(basePolicy({ has_qualifying_interaction: true, overdue_by: 0, next_due: '2026-02-01T00:00:00Z', last_interaction: '2026-01-02T00:00:00Z' }));

  expect(screen.getByText('On track')).toBeInTheDocument();
  expect(screen.queryByText('3 days overdue')).not.toBeInTheDocument();
});

test('no qualifying interactions yet renders the neutral hint, not overdue', () => {
  renderPanel(basePolicy({ has_qualifying_interaction: false, overdue_by: 0 }));

  expect(screen.getByText('No qualifying interactions yet — cadence starts once you record one.')).toBeInTheDocument();
  expect(screen.queryByText('On track')).not.toBeInTheDocument();
  expect(screen.queryByText('3 days overdue')).not.toBeInTheDocument();
});

test('edit and delete actions fire their handlers', () => {
  const onEdit = vi.fn();
  const onDelete = vi.fn();
  const policy = basePolicy({ has_qualifying_interaction: false, overdue_by: 0 });
  renderPanel(policy, { onEdit, onDelete });

  fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
  expect(onEdit).toHaveBeenCalledWith(policy);

  fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
  expect(onDelete).toHaveBeenCalledWith(policy.id);
});
