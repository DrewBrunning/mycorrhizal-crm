import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/react';
import '../i18n/config';
import ReminderDialog from './ReminderDialog';
import { Reminder } from '../api/reminders';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(cleanup);

function reminder(overrides: Partial<Reminder> = {}): Reminder {
  return {
    ID: 7,
    message: 'Water the plants',
    by_mail: true,
    remind_at: '2026-08-01T00:00:00Z',
    recurrence: 'monthly',
    reoccur_from_completion: false,
    completed: false,
    email_sent: false,
    contact_id: 3,
    ...overrides,
  };
}

function renderDialog(props: Partial<React.ComponentProps<typeof ReminderDialog>> = {}) {
  const defaults: React.ComponentProps<typeof ReminderDialog> = {
    open: true,
    onClose: vi.fn(),
    onSave: vi.fn().mockResolvedValue(undefined),
    contactId: 3,
  };
  const merged = { ...defaults, ...props };
  render(<ReminderDialog {...merged} />);
  return {
    onClose: merged.onClose,
    onSave: vi.mocked(merged.onSave),
  };
}

test('shows the add form with sensible defaults when no reminder is provided', () => {
  renderDialog();

  expect(screen.getByText('Add Reminder')).toBeInTheDocument();
  expect(screen.getByLabelText('Message *')).toHaveValue('');
  expect(screen.getByText('Once')).toBeInTheDocument();
  expect(screen.getByLabelText('Date *')).toHaveValue(new Date().toISOString().split('T')[0]);
  expect(screen.getByRole('checkbox', { name: 'Send email notification' })).toBeChecked();
  expect(screen.queryByRole('checkbox', { name: 'Reschedule from completion date' })).not.toBeInTheDocument();
});

test('saving a new reminder posts the form data and closes', async () => {
  const { onSave, onClose } = renderDialog();

  fireEvent.change(screen.getByLabelText('Message *'), { target: { value: '  Water the plants  ' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
  const data = onSave.mock.calls[0][0];
  expect(data).toMatchObject({
    message: 'Water the plants',
    by_mail: true,
    recurrence: 'once',
    reoccur_from_completion: true,
    contact_id: 3,
  });
  expect(typeof data.remind_at).toBe('string');
  await waitFor(() => expect(onClose).toHaveBeenCalled());
});

test('requires a message before saving', () => {
  const { onSave } = renderDialog();

  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(screen.getByText('Message is required')).toBeInTheDocument();
  expect(onSave).not.toHaveBeenCalled();
});

test('requires a date before saving', () => {
  const { onSave } = renderDialog();

  fireEvent.change(screen.getByLabelText('Message *'), { target: { value: 'Water the plants' } });
  fireEvent.change(screen.getByLabelText('Date *'), { target: { value: '' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(screen.getByText('Date is required')).toBeInTheDocument();
  expect(onSave).not.toHaveBeenCalled();
});

test('pre-fills the form when editing an existing reminder', async () => {
  const { onSave } = renderDialog({ reminder: reminder() });

  expect(screen.getByText('Edit Reminder')).toBeInTheDocument();
  expect(screen.getByLabelText('Message *')).toHaveValue('Water the plants');
  expect(screen.getByLabelText('Date *')).toHaveValue('2026-08-01');
  expect(screen.getByText('Monthly')).toBeInTheDocument();
  expect(screen.getByRole('checkbox', { name: 'Send email notification' })).toBeChecked();
  expect(screen.getByRole('checkbox', { name: 'Reschedule from completion date' })).not.toBeChecked();

  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
  expect(onSave.mock.calls[0][0]).toMatchObject({
    message: 'Water the plants',
    recurrence: 'monthly',
    reoccur_from_completion: false,
    remind_at: '2026-08-01T00:00:00.000Z',
  });
});

test('changing the recurrence re-schedules the date for a new reminder', async () => {
  renderDialog();

  fireEvent.mouseDown(screen.getByLabelText('Recurrence *'));
  fireEvent.click(await screen.findByRole('option', { name: 'Weekly' }));

  const d = new Date();
  d.setDate(d.getDate() + 7);
  expect(screen.getByLabelText('Date *')).toHaveValue(d.toISOString().split('T')[0]);
  expect(screen.getByRole('checkbox', { name: 'Reschedule from completion date' })).toBeInTheDocument();
});

test('a failed save surfaces the error and keeps the dialog open', async () => {
  const { onClose } = renderDialog({
    onSave: vi.fn().mockRejectedValue(new Error('server down')),
  });

  fireEvent.change(screen.getByLabelText('Message *'), { target: { value: 'Water the plants' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await waitFor(() => expect(screen.getByText('server down')).toBeInTheDocument());
  expect(onClose).not.toHaveBeenCalled();
});

test('unchecking the email checkbox saves by_mail as false', async () => {
  const { onSave } = renderDialog();

  fireEvent.click(screen.getByRole('checkbox', { name: 'Send email notification' }));
  fireEvent.change(screen.getByLabelText('Message *'), { target: { value: 'Water the plants' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
  expect(onSave.mock.calls[0][0]).toMatchObject({ by_mail: false });
});
