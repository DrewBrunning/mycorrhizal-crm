import { cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import { ApiError } from '../api/client';
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

test('explains that an active occasion creates its own reminder', () => {
  renderDialog();
  expect(
    screen.getByText(
      'A standing rule, like a yearly card or gift. An active occasion creates its own reminder at the lead time.',
    ),
  ).toBeInTheDocument();
});

test('requires a label before saving', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  // #2672: the error is the Label field's own helper text, not loose text.
  const label = screen.getByLabelText('Label *');
  await vi.waitFor(() => expect(label).toHaveAttribute('aria-invalid', 'true'));
  expect(label).toHaveAccessibleDescription(/Enter a label for this occasion\./);
  await vi.waitFor(() => expect(label).toHaveFocus());
  expect(onSave).not.toHaveBeenCalled();
});

test('requires a month and a day (neither set), naming the Month field and focusing it', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  fireEvent.change(screen.getByLabelText('Label *'), { target: { value: 'Birthday gift' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  const month = screen.getByLabelText('Month');
  await vi.waitFor(() => expect(month).toHaveAttribute('aria-invalid', 'true'));
  expect(month).toHaveAccessibleDescription(
    /An occasion needs a month and a day so it can remind you/,
  );
  expect(screen.getByLabelText('Day')).toHaveAttribute('aria-invalid', 'true');
  await vi.waitFor(() => expect(month).toHaveFocus());
  expect(onSave).not.toHaveBeenCalled();
});

test('the label hint no longer suggests a dateless "Birthday gift"', () => {
  renderDialog();
  const hint = screen.getByLabelText('Label *').getAttribute('aria-describedby');
  expect(document.getElementById(hint ?? '')).toHaveTextContent('Anniversary gift');
  expect(document.getElementById(hint ?? '')).not.toHaveTextContent('Birthday gift');
});

test('a legacy dateless row can still be saved inactive', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({
    onSave,
    obligation: {
      id: 'o1',
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
      entity_id: 'alice-uid',
      kind: 'gift',
      label: 'Old gift',
      anchor_month: undefined,
      anchor_day: undefined,
      lead_time_days: 0,
      active: true,
      sensitivity: 'normal',
    },
  });
  fireEvent.click(screen.getByLabelText('Active')); // retire it
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave.mock.calls[0][0]).toMatchObject({
    anchorMonth: null,
    anchorDay: null,
    active: false,
  });
});

test('a legacy dateless row cannot be re-saved active', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({
    onSave,
    obligation: {
      id: 'o1',
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
      entity_id: 'alice-uid',
      kind: 'gift',
      label: 'Old gift',
      anchor_month: undefined,
      anchor_day: undefined,
      lead_time_days: 0,
      active: true,
      sensitivity: 'normal',
    },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  await vi.waitFor(() =>
    expect(screen.getByLabelText('Month')).toHaveAttribute('aria-invalid', 'true'),
  );
  expect(onSave).not.toHaveBeenCalled();
});

test('the Day list follows the month length (#2671)', async () => {
  renderDialog();
  const dayCount = async () => {
    fireEvent.mouseDown(screen.getByLabelText('Day'));
    const list = await screen.findByRole('listbox');
    const n = within(list).getAllByRole('option').length - 1; // minus "Not set"
    fireEvent.keyDown(list, { key: 'Escape' });
    await vi.waitFor(() => expect(screen.queryByRole('listbox')).not.toBeInTheDocument());
    return n;
  };
  const pick = async (m: string) => {
    fireEvent.mouseDown(screen.getByLabelText('Month'));
    fireEvent.click(await screen.findByRole('option', { name: m }));
  };
  await pick('February');
  expect(await dayCount()).toBe(29);
  await pick('April');
  expect(await dayCount()).toBe(30);
  await pick('January');
  expect(await dayCount()).toBe(31);
});

test('changing to a shorter month clears an out-of-range day and says so (#2671)', async () => {
  renderDialog();
  fireEvent.mouseDown(screen.getByLabelText('Month'));
  fireEvent.click(await screen.findByRole('option', { name: 'January' }));
  fireEvent.mouseDown(screen.getByLabelText('Day'));
  fireEvent.click(await screen.findByRole('option', { name: '31' }));
  fireEvent.mouseDown(screen.getByLabelText('Month'));
  fireEvent.click(await screen.findByRole('option', { name: 'April' }));

  const day = screen.getByLabelText('Day');
  expect(day).not.toHaveTextContent('31');
  expect(day).toHaveAccessibleDescription(
    'April has at most 30 days, so the day was cleared. Choose a day again.',
  );
});

test('an in-range day survives a month change (#2671)', async () => {
  renderDialog();
  fireEvent.mouseDown(screen.getByLabelText('Month'));
  fireEvent.click(await screen.findByRole('option', { name: 'January' }));
  fireEvent.mouseDown(screen.getByLabelText('Day'));
  fireEvent.click(await screen.findByRole('option', { name: '15' }));
  fireEvent.mouseDown(screen.getByLabelText('Month'));
  fireEvent.click(await screen.findByRole('option', { name: 'April' }));
  expect(screen.getByLabelText('Day')).toHaveTextContent('15');
});

test('February 29 stays selectable (#2671)', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });
  fireEvent.change(screen.getByLabelText('Label *'), { target: { value: 'Leap card' } });
  fireEvent.mouseDown(screen.getByLabelText('Month'));
  fireEvent.click(await screen.findByRole('option', { name: 'February' }));
  fireEvent.mouseDown(screen.getByLabelText('Day'));
  fireEvent.click(await screen.findByRole('option', { name: '29' }));
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave.mock.calls[0][0]).toMatchObject({ anchorMonth: 2, anchorDay: 29 });
});

test('rejects a month set with no day', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  fireEvent.change(screen.getByLabelText('Label *'), { target: { value: 'Christmas card' } });
  fireEvent.mouseDown(screen.getByLabelText('Month'));
  fireEvent.click(await screen.findByRole('option', { name: 'December' }));
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  const day = screen.getByLabelText('Day');
  await vi.waitFor(() => expect(day).toHaveAttribute('aria-invalid', 'true'));
  expect(day).toHaveAccessibleDescription(
    'An occasion anchor date requires both a month and a day',
  );
  await vi.waitFor(() => expect(day).toHaveFocus());
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

  const lead = screen.getByLabelText('Lead time (days)');
  await vi.waitFor(() => expect(lead).toHaveAttribute('aria-invalid', 'true'));
  expect(lead).toHaveAccessibleDescription(/Enter a valid number of lead-time days\./);
  expect(onSave).not.toHaveBeenCalled();
});

test('shows an error and stays open when onSave rejects', async () => {
  const onSave = vi.fn().mockRejectedValue(new Error('boom'));
  const onClose = vi.fn();
  renderDialog({ onSave, onClose });

  await fillValidForm();
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(await screen.findByRole('alert')).toHaveTextContent('Failed to save occasion.');
  expect(onClose).not.toHaveBeenCalled();
});

async function fillValidForm() {
  fireEvent.change(screen.getByLabelText('Label *'), { target: { value: 'Christmas card' } });
  fireEvent.mouseDown(screen.getByLabelText('Month'));
  fireEvent.click(await screen.findByRole('option', { name: 'December' }));
  fireEvent.mouseDown(screen.getByLabelText('Day'));
  fireEvent.click(await screen.findByRole('option', { name: '25' }));
  await vi.waitFor(() => expect(screen.queryByRole('listbox')).not.toBeInTheDocument());
}

function reasonError(reason: string) {
  return new ApiError('English fallback', 'VALIDATION_ERROR', 400, undefined, undefined, reason);
}

test.each([
  [
    'occasion_anchor_date_required',
    'Month',
    'An occasion needs a month and a day so it can remind you',
  ],
  [
    'occasion_anchor_date_incomplete',
    'Day',
    'An occasion anchor date requires both a month and a day',
  ],
  [
    'occasion_anchor_date_invalid',
    'Day',
    'That month and day do not exist together, for example February 30',
  ],
])(
  'shows the localized server reason %s on the %s field instead of the generic failure',
  async (reason, field, text) => {
    const onSave = vi.fn().mockRejectedValue(reasonError(reason));
    renderDialog({ onSave });
    await fillValidForm();
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));

    const control = screen.getByRole('combobox', { name: field });
    await vi.waitFor(() => expect(control).toHaveAttribute('aria-invalid', 'true'));
    expect(control).toHaveAccessibleDescription(text);
    // Focus moves in an effect after the error state renders; wait for it
    // rather than racing it under CI load.
    await vi.waitFor(() => expect(control).toHaveFocus());
    expect(screen.queryByText('Failed to save occasion.')).not.toBeInTheDocument();
  },
);

test('shows another localized server reason in the form-level alert', async () => {
  const onSave = vi
    .fn()
    .mockRejectedValue(
      new ApiError(
        'English',
        'VALIDATION_ERROR',
        400,
        undefined,
        undefined,
        'occasion_kind_invalid',
      ),
    );
  renderDialog({ onSave });
  await fillValidForm();
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'kind must not contain control characters',
  );
});

test('saves a custom kind, inactive, secret, and notes', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  fireEvent.change(screen.getByLabelText('Kind'), { target: { value: 'thank-you note' } });
  fireEvent.change(screen.getByLabelText('Label *'), { target: { value: 'Thank-you note' } });
  fireEvent.mouseDown(screen.getByLabelText('Month'));
  fireEvent.click(await screen.findByRole('option', { name: 'March' }));
  fireEvent.mouseDown(screen.getByLabelText('Day'));
  fireEvent.click(await screen.findByRole('option', { name: '3' }));
  fireEvent.click(screen.getByLabelText('Active'));
  fireEvent.mouseDown(screen.getByLabelText('Sensitivity'));
  fireEvent.click(await screen.findByRole('option', { name: 'Secret' }));
  fireEvent.change(screen.getByLabelText('Notes'), { target: { value: 'Keep this quiet' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave).toHaveBeenCalledWith({
    kind: 'thank-you note',
    label: 'Thank-you note',
    anchorMonth: 3,
    anchorDay: 3,
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

test('typing in an invalid field clears its error', async () => {
  renderDialog();
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  const label = screen.getByLabelText('Label *');
  await vi.waitFor(() => expect(label).toHaveAttribute('aria-invalid', 'true'));
  fireEvent.change(label, { target: { value: 'C' } });
  expect(label).toHaveAttribute('aria-invalid', 'false');
});

test('rejects a day set with no month, naming the Month field', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });
  fireEvent.change(screen.getByLabelText('Label *'), { target: { value: 'Card' } });
  fireEvent.mouseDown(screen.getByLabelText('Day'));
  fireEvent.click(await screen.findByRole('option', { name: '5' }));
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  const month = screen.getByRole('combobox', { name: 'Month' });
  await vi.waitFor(() => expect(month).toHaveAttribute('aria-invalid', 'true'));
  expect(month).toHaveAccessibleDescription(
    'An occasion anchor date requires both a month and a day',
  );
  expect(onSave).not.toHaveBeenCalled();
});

test('a stored impossible date (Feb 31) is flagged on the Day field instead of being resubmitted', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({
    onSave,
    obligation: {
      id: 'o1',
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
      entity_id: 'alice-uid',
      kind: 'gift',
      label: 'Feb31 test',
      anchor_month: 2,
      anchor_day: 31,
      lead_time_days: 0,
      active: true,
      sensitivity: 'normal',
    },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  const day = screen.getByRole('combobox', { name: 'Day' });
  await vi.waitFor(() => expect(day).toHaveAttribute('aria-invalid', 'true'));
  expect(day).toHaveAccessibleDescription(
    'That month and day do not exist together, for example February 30',
  );
  expect(onSave).not.toHaveBeenCalled();
});
