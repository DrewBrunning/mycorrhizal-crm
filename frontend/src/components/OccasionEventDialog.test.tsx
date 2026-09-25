import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { OccasionEvent } from '../api/occasionEvents';
import OccasionEventDialog, { fromDatetimeLocal, toDatetimeLocal } from './OccasionEventDialog';

afterEach(cleanup);

const baseEvent: OccasionEvent = {
  id: 'ev-1',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  title: 'Summer BBQ',
  starts_at: '2026-07-04T15:00:00Z',
  sensitivity: 'normal',
};

test('shows the create title when no event is given', () => {
  render(<OccasionEventDialog open onClose={vi.fn()} onSave={vi.fn()} />);
  expect(screen.getByText('Create an event')).toBeInTheDocument();
});

test('prefills the form when editing', () => {
  render(<OccasionEventDialog open onClose={vi.fn()} onSave={vi.fn()} event={baseEvent} />);
  expect(screen.getByText('Edit event')).toBeInTheDocument();
  expect(screen.getByDisplayValue('Summer BBQ')).toBeInTheDocument();
});

test('requires a title', async () => {
  const onSave = vi.fn();
  render(<OccasionEventDialog open onClose={vi.fn()} onSave={onSave} />);
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  expect(await screen.findByText('Enter a title for this event.')).toBeInTheDocument();
  expect(onSave).not.toHaveBeenCalled();
});

test('requires a start date and time', async () => {
  render(<OccasionEventDialog open onClose={vi.fn()} onSave={vi.fn()} />);
  fireEvent.change(screen.getByLabelText(/^Title/), { target: { value: 'Party' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  expect(await screen.findByText('Choose a start date and time.')).toBeInTheDocument();
});

test('rejects an end before the start', async () => {
  render(<OccasionEventDialog open onClose={vi.fn()} onSave={vi.fn()} />);
  fireEvent.change(screen.getByLabelText(/^Title/), { target: { value: 'Party' } });
  fireEvent.change(screen.getByLabelText(/^Starts/), {
    target: { value: '2026-07-04T18:00' },
  });
  fireEvent.change(screen.getByLabelText(/^Ends/), {
    target: { value: '2026-07-04T17:00' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  expect(
    await screen.findByText('The end time cannot be before the start time.'),
  ).toBeInTheDocument();
});

test('saves a valid event and closes', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  const onClose = vi.fn();
  render(<OccasionEventDialog open onClose={onClose} onSave={onSave} />);

  fireEvent.change(screen.getByLabelText(/^Title/), { target: { value: 'Party' } });
  fireEvent.change(screen.getByLabelText(/^Starts/), {
    target: { value: '2026-07-04T15:00' },
  });
  fireEvent.change(screen.getByLabelText('Location'), { target: { value: 'The park' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
  const input = onSave.mock.calls[0][0];
  expect(input.title).toBe('Party');
  expect(input.location).toBe('The park');
  expect(input.starts_at).toBe(new Date('2026-07-04T15:00').toISOString());
  await waitFor(() => expect(onClose).toHaveBeenCalled());
});

test('shows a save-failed message when onSave rejects', async () => {
  const onSave = vi.fn().mockRejectedValue(new Error('boom'));
  render(<OccasionEventDialog open onClose={vi.fn()} onSave={onSave} />);

  fireEvent.change(screen.getByLabelText(/^Title/), { target: { value: 'Party' } });
  fireEvent.change(screen.getByLabelText(/^Starts/), {
    target: { value: '2026-07-04T15:00' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(await screen.findByText('Failed to save event.')).toBeInTheDocument();
});

test('toDatetimeLocal / fromDatetimeLocal round-trip and handle blanks', () => {
  expect(toDatetimeLocal(undefined)).toBe('');
  expect(toDatetimeLocal('not-a-date')).toBe('');
  expect(fromDatetimeLocal('')).toBeNull();
  expect(fromDatetimeLocal('nonsense')).toBeNull();
  const iso = new Date('2026-07-04T15:00').toISOString();
  expect(fromDatetimeLocal(toDatetimeLocal(iso))).toBe(iso);
});
