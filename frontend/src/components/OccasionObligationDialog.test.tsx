import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import OccasionObligationDialog from './OccasionObligationDialog';

afterEach(cleanup);

function renderDialog(props: Partial<React.ComponentProps<typeof OccasionObligationDialog>> = {}) {
  const defaults: React.ComponentProps<typeof OccasionObligationDialog> = {
    open: true,
    onClose: vi.fn(),
    onSave: vi.fn().mockResolvedValue(undefined),
    ...props,
  };
  return render(<OccasionObligationDialog {...defaults} />);
}

test('create mode shows the kind, label, anchor, lead time, active, sensitivity, and notes fields', () => {
  renderDialog();
  expect(screen.getByLabelText('Kind')).toBeInTheDocument();
  expect(screen.getByLabelText('Label *')).toBeInTheDocument();
  expect(screen.getByLabelText('Month')).toBeInTheDocument();
  expect(screen.getByLabelText('Day')).toBeInTheDocument();
  expect(screen.getByLabelText('Lead time (days)')).toBeInTheDocument();
  expect(screen.getByLabelText('Active')).toBeInTheDocument();
  expect(screen.getByLabelText('Sensitivity')).toBeInTheDocument();
  expect(screen.getByLabelText('Notes')).toBeInTheDocument();
});

test('requires a label before saving', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(await screen.findByText('Enter a label for this occasion.')).toBeInTheDocument();
  expect(onSave).not.toHaveBeenCalled();
});

test('rejects a month set with no day', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  fireEvent.change(screen.getByLabelText('Label *'), { target: { value: 'Christmas card' } });
  fireEvent.mouseDown(screen.getByLabelText('Month'));
  fireEvent.click(await screen.findByRole('option', { name: 'December' }));
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(
    await screen.findByText('Set both a month and a day, or leave both unset.'),
  ).toBeInTheDocument();
  expect(onSave).not.toHaveBeenCalled();
});

test('saves with the label, kind, anchor date, and lead time', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  fireEvent.change(screen.getByLabelText('Label *'), { target: { value: 'Christmas card' } });
  fireEvent.mouseDown(screen.getByLabelText('Month'));
  fireEvent.click(await screen.findByRole('option', { name: 'December' }));
  fireEvent.mouseDown(screen.getByLabelText('Day'));
  fireEvent.click(await screen.findByRole('option', { name: '25' }));
  fireEvent.change(screen.getByLabelText('Lead time (days)'), { target: { value: '14' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave).toHaveBeenCalledWith({
    kind: 'card',
    label: 'Christmas card',
    anchorMonth: 12,
    anchorDay: 25,
    leadTimeDays: 14,
    active: true,
    sensitivity: 'normal',
    notes: undefined,
  });
});

test('rejects a negative lead time', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  fireEvent.change(screen.getByLabelText('Label *'), { target: { value: 'Christmas card' } });
  fireEvent.change(screen.getByLabelText('Lead time (days)'), { target: { value: '-1' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(await screen.findByText('Enter a valid number of lead-time days.')).toBeInTheDocument();
  expect(onSave).not.toHaveBeenCalled();
});

test('shows an error and stays open when onSave rejects', async () => {
  const onSave = vi.fn().mockRejectedValue(new Error('boom'));
  const onClose = vi.fn();
  renderDialog({ onSave, onClose });

  fireEvent.change(screen.getByLabelText('Label *'), { target: { value: 'Christmas card' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(await screen.findByText('Failed to save occasion.')).toBeInTheDocument();
  expect(onClose).not.toHaveBeenCalled();
});

test('saves a custom kind, inactive, secret, and notes', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  fireEvent.change(screen.getByLabelText('Kind'), { target: { value: 'thank-you note' } });
  fireEvent.change(screen.getByLabelText('Label *'), { target: { value: 'Thank-you note' } });
  fireEvent.click(screen.getByLabelText('Active'));
  fireEvent.mouseDown(screen.getByLabelText('Sensitivity'));
  fireEvent.click(await screen.findByRole('option', { name: 'Secret' }));
  fireEvent.change(screen.getByLabelText('Notes'), { target: { value: 'Keep this quiet' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave).toHaveBeenCalledWith({
    kind: 'thank-you note',
    label: 'Thank-you note',
    anchorMonth: null,
    anchorDay: null,
    leadTimeDays: 0,
    active: false,
    sensitivity: 'secret',
    notes: 'Keep this quiet',
  });
});

test('edit mode pre-fills the existing obligation', () => {
  renderDialog({
    obligation: {
      id: 'o1',
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
      entity_id: 'alice-uid',
      kind: 'gift',
      label: 'Birthday gift',
      anchor_month: 7,
      anchor_day: 4,
      lead_time_days: 10,
      active: false,
      sensitivity: 'private',
      notes: 'Loves board games',
    },
  });
  expect(screen.getByLabelText('Label *')).toHaveValue('Birthday gift');
  expect(screen.getByLabelText('Lead time (days)')).toHaveValue(10);
  expect(screen.getByLabelText('Active')).not.toBeChecked();
  expect(screen.getByLabelText('Notes')).toHaveValue('Loves board games');
});
