import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { getContacts } from '../api/contacts';
import { SnackbarProvider } from '../context/SnackbarContext';
import { DateFormatProvider, formatDateWithFormat, getDateFormat } from '../DateFormatProvider';
import { useTimeZone } from '../testSupport/timeZone';
import AddActivityDialog from './AddActivityDialog';
import AddNoteDialog from './AddNoteDialog';
import ReminderDialog from './ReminderDialog';

// Issue #1727: the "today" default of every add dialog is the user's LOCAL
// date, not the UTC date. 22:30 in Chicago (CDT) on 11 March is 03:30Z on
// 12 March, so the old `toISOString().split('T')[0]` defaulted to tomorrow.

vi.mock('../api/contacts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/contacts')>();
  return { ...actual, getContacts: vi.fn() };
});

beforeEach(() => {
  // See src/testSupport/timeZone.ts: runtime TZ mutation is ignored in the
  // worker threads Stryker runs tests in (issue #1843).
  useTimeZone('America/Chicago');
  // Only Date is faked: RTL's waitFor and MUI transitions need real timers.
  vi.useFakeTimers({ toFake: ['Date'] });
  vi.setSystemTime(new Date('2026-03-12T03:30:00Z'));
  vi.mocked(getContacts).mockReset();
  vi.mocked(getContacts).mockResolvedValue({ contacts: [], next_cursor: '' } as never);
});

afterEach(() => {
  cleanup();
  sessionStorage.clear();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

test('AddActivityDialog defaults the date to the local date', async () => {
  render(
    <DateFormatProvider>
      <AddActivityDialog open onClose={vi.fn()} onSave={vi.fn().mockResolvedValue(undefined)} />
    </DateFormatProvider>,
  );
  await waitFor(() => expect(getContacts).toHaveBeenCalled());
  expect(screen.getByLabelText('Date *')).toHaveValue(
    formatDateWithFormat('2026-03-11', getDateFormat()),
  );
});

test('AddNoteDialog defaults the date to the local date', async () => {
  render(
    <SnackbarProvider>
      <DateFormatProvider>
        <AddNoteDialog open onClose={vi.fn()} onSave={vi.fn().mockResolvedValue(undefined)} />
      </DateFormatProvider>
    </SnackbarProvider>,
  );
  await waitFor(() => expect(getContacts).toHaveBeenCalled());
  expect(screen.getByLabelText('Date')).toHaveValue(
    formatDateWithFormat('2026-03-11', getDateFormat()),
  );
});

test('ReminderDialog defaults the date, and its min, to the local date', () => {
  render(
    <DateFormatProvider>
      <ReminderDialog
        open
        contactId={3}
        onClose={vi.fn()}
        onSave={vi.fn().mockResolvedValue(undefined)}
      />
    </DateFormatProvider>,
  );
  expect(screen.getByLabelText('Date *')).toHaveValue(
    formatDateWithFormat('2026-03-11', getDateFormat()),
  );

  // The min is enforced by the calendar: the day before "today" is disabled.
  fireEvent.click(screen.getByLabelText('Date *'));
  expect(screen.getByRole('button', { name: '10.03.2026' })).toBeDisabled();
  expect(screen.getByRole('button', { name: '11.03.2026' })).not.toBeDisabled();
});
