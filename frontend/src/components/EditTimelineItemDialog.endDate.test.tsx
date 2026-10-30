import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { SnackbarProvider } from '../context/SnackbarContext';
import { DateFormatProvider } from '../DateFormatProvider';
import EditTimelineItemDialog, { type TimelineItemValues } from './EditTimelineItemDialog';

// Issue #2006: the activity editor carries both date fields; the server's
// end_date refusal is shown on the end field.

afterEach(cleanup);

function renderEditor(
  values: TimelineItemValues,
  extra: Partial<React.ComponentProps<typeof EditTimelineItemDialog>> = {},
) {
  const onChange = vi.fn();
  render(
    <SnackbarProvider>
      <DateFormatProvider>
        <EditTimelineItemDialog
          open
          onClose={vi.fn()}
          onSave={vi.fn()}
          onDelete={vi.fn()}
          type="activity"
          values={values}
          onChange={onChange}
          allContacts={[]}
          {...extra}
        />
      </DateFormatProvider>
    </SnackbarProvider>,
  );
  return onChange;
}

test('shows the start and the optional end date', () => {
  renderEditor({
    activityTitle: 'Trip',
    activityDate: '2026-05-12',
    activityEndDate: '2026-05-15',
  });
  expect(screen.getByLabelText('Date')).toHaveValue('12.05.2026');
  expect(screen.getByLabelText('End date (optional)')).toHaveValue('15.05.2026');
});

test('an activity without an end date shows an empty end field; clearing it reports an empty value', () => {
  const onChange = renderEditor({
    activityTitle: 'Trip',
    activityDate: '2026-05-12',
    activityEndDate: '2026-05-15',
  });
  fireEvent.change(screen.getByLabelText('End date (optional)'), { target: { value: '' } });
  expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ activityEndDate: '' }));
});

test('a server end_date error shows on the end field', () => {
  renderEditor(
    { activityTitle: 'Trip', activityDate: '2026-05-25', activityEndDate: '2026-05-15' },
    { activityEndDateError: 'Server says no' },
  );
  expect(screen.getByText('Server says no')).toBeInTheDocument();
});

test('a start moved past the end is flagged without a server round trip', () => {
  renderEditor({
    activityTitle: 'Trip',
    activityDate: '2026-05-25',
    activityEndDate: '2026-05-15',
  });
  expect(screen.getByText('The end date cannot be before the start date')).toBeInTheDocument();
});

test('a valid range shows no error', () => {
  renderEditor({
    activityTitle: 'Trip',
    activityDate: '2026-05-12',
    activityEndDate: '2026-05-12',
  });
  expect(screen.queryByText('The end date cannot be before the start date')).toBeNull();
});
