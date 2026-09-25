import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { DataDecayPolicy } from '../api/dataDecayPolicies';
import { SnackbarProvider } from '../context/SnackbarContext';
import DataDecayDialog from './DataDecayDialog';

afterEach(cleanup);

function renderDialog(props: Partial<React.ComponentProps<typeof DataDecayDialog>> = {}) {
  const defaults: React.ComponentProps<typeof DataDecayDialog> = {
    open: true,
    onClose: vi.fn(),
    onSave: vi.fn().mockResolvedValue(undefined),
    entityId: 'alice-uid',
    ...props,
  };
  return render(
    <SnackbarProvider>
      <DataDecayDialog {...defaults} />
    </SnackbarProvider>,
  );
}

function existingPolicy(): DataDecayPolicy {
  return {
    id: 'policy-1',
    entity_id: 'alice-uid',
    interval_days: 90,
    active: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    health: { overdue_by: 3, next_due: '2026-01-01T00:00:00Z' },
  };
}

test('create mode defaults to a 365-day interval and submits active:true', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  // MUI appends " *" to a required field's accessible label.
  const interval = screen.getByLabelText('Interval (days) *');
  expect((interval as HTMLInputElement).value).toBe('365');
  expect((screen.getByLabelText('Active') as HTMLInputElement).checked).toBe(true);

  screen.getByRole('button', { name: 'Save' }).click();
  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());

  const submitted = onSave.mock.calls[0][0];
  expect(submitted.entity_id).toBe('alice-uid');
  expect(submitted.interval_days).toBe(365);
  expect(submitted.active).toBe(true);
});

test('edit mode pre-fills the interval and active state', () => {
  renderDialog({ policy: existingPolicy() });

  expect((screen.getByLabelText('Interval (days) *') as HTMLInputElement).value).toBe('90');
  expect((screen.getByLabelText('Active') as HTMLInputElement).checked).toBe(false);
});

test('toggling active off submits active:false', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  fireEvent.click(screen.getByLabelText('Active'));
  screen.getByRole('button', { name: 'Save' }).click();

  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave.mock.calls[0][0].active).toBe(false);
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
