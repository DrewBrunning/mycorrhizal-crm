import { cleanup, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { getContacts } from '../api/contacts';
import { SnackbarProvider } from '../context/SnackbarContext';
import { DateFormatProvider } from '../DateFormatProvider';
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
    <AddActivityDialog open onClose={vi.fn()} onSave={vi.fn().mockResolvedValue(undefined)} />,
  );
  await waitFor(() => expect(getContacts).toHaveBeenCalled());
  expect(screen.getByLabelText('Date *')).toHaveValue('2026-03-11');
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
  expect(screen.getByLabelText('Date')).toHaveValue('2026-03-11');
});

test('ReminderDialog defaults the date, and its min, to the local date', () => {
  render(
    <ReminderDialog
      open
      contactId={3}
      onClose={vi.fn()}
      onSave={vi.fn().mockResolvedValue(undefined)}
    />,
  );
  const input = screen.getByLabelText('Date *');
  expect(input).toHaveValue('2026-03-11');
  expect(input).toHaveAttribute('min', '2026-03-11');
});
