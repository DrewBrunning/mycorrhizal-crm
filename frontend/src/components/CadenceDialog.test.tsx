import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { CadencePolicy } from '../api/cadencePolicies';
import { SnackbarProvider } from '../context/SnackbarContext';
import CadenceDialog from './CadenceDialog';

afterEach(cleanup);

function renderDialog(props: Partial<React.ComponentProps<typeof CadenceDialog>> = {}) {
  const defaults: React.ComponentProps<typeof CadenceDialog> = {
    open: true,
    onClose: vi.fn(),
    onSave: vi.fn().mockResolvedValue(undefined),
    entityId: 'alice-uid',
    ...props,
  };
  return render(
    <SnackbarProvider>
      <CadenceDialog {...defaults} />
    </SnackbarProvider>,
  );
}

function existingPolicy(): CadencePolicy {
  return {
    id: 'policy-1',
    entity_id: 'alice-uid',
    target_interval_days: 30,
    qualifying_types: ['call', 'visit'],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    health: { has_qualifying_interaction: true, overdue_by: 3, next_due: '2026-01-01T00:00:00Z' },
  };
}

test('create mode defaults to a 30-day interval and submits the chosen types', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  // MUI appends " *" to a required field's accessible label.
  const interval = screen.getByLabelText('Interval (days) *');
  expect((interval as HTMLInputElement).value).toBe('30');

  fireEvent.click(screen.getByLabelText('Call'));
  fireEvent.click(screen.getByLabelText('Visit'));

  screen.getByRole('button', { name: 'Save' }).click();
  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());

  const submitted = onSave.mock.calls[0][0];
  expect(submitted.entity_id).toBe('alice-uid');
  expect(submitted.target_interval_days).toBe(30);
  expect(submitted.qualifying_types).toEqual(['call', 'visit']);
});

test('edit mode pre-fills the interval and checked types', () => {
  renderDialog({ policy: existingPolicy() });

  expect((screen.getByLabelText('Interval (days) *') as HTMLInputElement).value).toBe('30');
  expect((screen.getByLabelText('Call') as HTMLInputElement).checked).toBe(true);
  expect((screen.getByLabelText('Visit') as HTMLInputElement).checked).toBe(true);
  expect((screen.getByLabelText('Meal') as HTMLInputElement).checked).toBe(false);
});

test('rejects a non-positive interval without saving', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  const interval = screen.getByLabelText('Interval (days) *');
  fireEvent.change(interval, { target: { value: '0' } });
  screen.getByRole('button', { name: 'Save' }).click();

  await vi.waitFor(() =>
    expect(screen.getByText('Enter a positive number of days.')).toBeInTheDocument(),
  );
  expect(onSave).not.toHaveBeenCalled();
});
