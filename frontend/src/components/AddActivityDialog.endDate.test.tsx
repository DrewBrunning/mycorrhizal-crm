import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { ApiError } from '../api/client';
import { getContacts } from '../api/contacts';
import { DateFormatProvider } from '../DateFormatProvider';
import { pickDate } from '../testSupport/dateField';
import AddActivityDialog from './AddActivityDialog';

// Issue #2006: the Add Activity dialog has an optional end date next to the
// start date; the server's 400 on end_date shows on the end field.

afterEach(() => {
  cleanup();
  sessionStorage.clear();
});

vi.mock('../api/contacts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/contacts')>();
  return { ...actual, getContacts: vi.fn() };
});

beforeEach(() => {
  vi.mocked(getContacts).mockReset();
  vi.mocked(getContacts).mockResolvedValue({ contacts: [], next_cursor: '' } as never);
});

function renderDialog(props: Partial<React.ComponentProps<typeof AddActivityDialog>> = {}) {
  const defaults: React.ComponentProps<typeof AddActivityDialog> = {
    open: true,
    onClose: vi.fn(),
    onSave: vi.fn().mockResolvedValue(undefined),
    prefill: { date: '2026-05-12' },
    ...props,
  };
  return render(
    <DateFormatProvider>
      <AddActivityDialog {...defaults} />
    </DateFormatProvider>,
  );
}

test('shows an optional end date field and sends none when it is left empty', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });
  await waitFor(() => expect(getContacts).toHaveBeenCalled());

  expect(screen.getByLabelText('End date (optional)')).toHaveValue('');
  fireEvent.change(screen.getByLabelText('Title *'), { target: { value: 'Lunch' } });
  fireEvent.click(screen.getByRole('button', { name: /^save$/i }));

  await waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave.mock.calls[0][0]).not.toHaveProperty('end_date');
});

test('a picked end date is sent as end_date alongside date', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });
  await waitFor(() => expect(getContacts).toHaveBeenCalled());

  fireEvent.change(screen.getByLabelText('Title *'), { target: { value: 'Trip' } });
  await pickDate('End date (optional)', '2026-05-15', '2026-05-12');
  expect(screen.getByRole('textbox', { name: 'End date (optional)' })).toHaveValue('15.05.2026');
  fireEvent.click(screen.getByRole('button', { name: /^save$/i }));

  await waitFor(() =>
    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({ date: '2026-05-12', end_date: '2026-05-15' }),
    ),
  );
});

test('an end date typed before the start is refused on the end field', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });
  await waitFor(() => expect(getContacts).toHaveBeenCalled());

  const end = screen.getByLabelText('End date (optional)');
  fireEvent.change(end, { target: { value: '01.05.2026' } });
  fireEvent.blur(end);

  // The field's min is the start date, so an earlier typed day is refused and
  // never becomes the value (so it is never sent).
  expect(await screen.findByText('That date is outside the allowed range.')).toBeInTheDocument();
  fireEvent.change(screen.getByLabelText('Title *'), { target: { value: 'Trip' } });
  fireEvent.click(screen.getByRole('button', { name: /^save$/i }));
  await waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave.mock.calls[0][0]).not.toHaveProperty('end_date');
});

test('moving the start past an already picked end shows the error and blocks Save', async () => {
  const onSave = vi.fn();
  renderDialog({ onSave });
  await waitFor(() => expect(getContacts).toHaveBeenCalled());

  await pickDate('End date (optional)', '2026-05-15', '2026-05-12');
  fireEvent.change(screen.getByLabelText('Title *'), { target: { value: 'Trip' } });
  await pickDate('Date *', '2026-05-20', '2026-05-12');

  expect(screen.getByText('The end date cannot be before the start date')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: /^save$/i })).toBeDisabled();
  expect(onSave).not.toHaveBeenCalled();

  // Clearing the end lifts the block.
  fireEvent.change(screen.getByLabelText('End date (optional)'), { target: { value: '' } });
  expect(screen.queryByText('The end date cannot be before the start date')).toBeNull();
  expect(screen.getByRole('button', { name: /^save$/i })).toBeEnabled();
});

test("the server's 400 on end_date shows on the end field, not as a generic failure", async () => {
  const onSave = vi.fn().mockRejectedValue(
    new ApiError(
      'Invalid value for field end_date',
      'INVALID_INPUT',
      400,
      {
        field: 'end_date',
        reason: 'end date precedes the start date',
      },
      undefined,
      'activity_end_before_start',
    ),
  );
  renderDialog({ onSave });
  await waitFor(() => expect(getContacts).toHaveBeenCalled());

  await pickDate('End date (optional)', '2026-05-15', '2026-05-12');
  fireEvent.change(screen.getByLabelText('Title *'), { target: { value: 'Trip' } });
  fireEvent.click(screen.getByRole('button', { name: /^save$/i }));

  expect(
    await screen.findByText('The end date cannot be before the start date'),
  ).toBeInTheDocument();
  expect(screen.queryByText('Failed to save activity')).toBeNull();

  // Editing the end clears the stale server error.
  fireEvent.change(screen.getByLabelText('End date (optional)'), { target: { value: '' } });
  expect(screen.queryByText('The end date cannot be before the start date')).toBeNull();
});

test('a failure that is not about end_date still shows the generic error', async () => {
  const onSave = vi.fn().mockRejectedValue(new ApiError('nope', 'INTERNAL', 500));
  renderDialog({ onSave });
  await waitFor(() => expect(getContacts).toHaveBeenCalled());

  fireEvent.change(screen.getByLabelText('Title *'), { target: { value: 'Trip' } });
  fireEvent.click(screen.getByRole('button', { name: /^save$/i }));
  expect(await screen.findByText('Failed to save activity')).toBeInTheDocument();
});

test('a dirty draft restores the end date after a remount', async () => {
  const first = renderDialog({ prefill: undefined });
  await waitFor(() => expect(getContacts).toHaveBeenCalled());
  fireEvent.change(screen.getByLabelText('Title *'), { target: { value: 'Trip' } });
  await pickDate('Date *', '2026-05-12');
  await pickDate('End date (optional)', '2026-05-15', '2026-05-12');
  first.unmount();

  renderDialog({ prefill: undefined });
  await waitFor(() =>
    expect(screen.getByRole('textbox', { name: 'End date (optional)' })).toHaveValue('15.05.2026'),
  );
});

test('changing the start clears a stale server error on the end field', async () => {
  const onSave = vi
    .fn()
    .mockRejectedValue(
      new ApiError(
        'bad',
        'INVALID_INPUT',
        400,
        { field: 'end_date' },
        undefined,
        'activity_end_before_start',
      ),
    );
  renderDialog({ onSave });
  await waitFor(() => expect(getContacts).toHaveBeenCalled());
  await pickDate('End date (optional)', '2026-05-15', '2026-05-12');
  fireEvent.change(screen.getByLabelText('Title *'), { target: { value: 'Trip' } });
  fireEvent.click(screen.getByRole('button', { name: /^save$/i }));
  expect(
    await screen.findByText('The end date cannot be before the start date'),
  ).toBeInTheDocument();

  await pickDate('Date *', '2026-05-13', '2026-05-12');
  expect(screen.queryByText('The end date cannot be before the start date')).toBeNull();
});
