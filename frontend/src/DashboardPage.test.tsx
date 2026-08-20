import { test, expect, vi, afterEach, beforeEach } from 'vitest';
import { render, screen, cleanup, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter, Routes, Route } from 'react-router';
import './i18n/config';
import DashboardPage from './DashboardPage';
import { DateFormatProvider } from './DateFormatProvider';
import { getDashboard, DashboardResponse } from './api/dashboard';
import { getUpcomingReminders, completeReminder, skipReminder } from './api/reminders';
import { listCircles } from './api/circles';

// M3: the page
// now fetches one GET /dashboard composite instead of fanning out to four
// endpoints plus a per-reminder contact lookup. This codebase's vitest has
// no auto-cleanup (CLAUDE.md frontend trap #1).
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

vi.mock('./api/dashboard', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/dashboard')>();
  return { ...actual, getDashboard: vi.fn() };
});
vi.mock('./api/reminders', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/reminders')>();
  return { ...actual, getUpcomingReminders: vi.fn(), completeReminder: vi.fn(), skipReminder: vi.fn() };
});
vi.mock('./api/circles', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/circles')>();
  return { ...actual, listCircles: vi.fn() };
});

const getDashboardMock = vi.mocked(getDashboard);
const getUpcomingRemindersMock = vi.mocked(getUpcomingReminders);
const completeReminderMock = vi.mocked(completeReminder);
const skipReminderMock = vi.mocked(skipReminder);
const listCirclesMock = vi.mocked(listCircles);

function emptyDashboard(): DashboardResponse {
  return { birthdays: [], random_contacts: [], upcoming_reminders: [], overdue: [], favorites: [], reach_out_suggestions: [] };
}

beforeEach(() => {
  getDashboardMock.mockReset();
  getUpcomingRemindersMock.mockReset();
  completeReminderMock.mockReset();
  skipReminderMock.mockReset();
  listCirclesMock.mockReset();
  listCirclesMock.mockResolvedValue({ circles: [], total: 0, next_cursor: '', limit: 200, members: [] });
});

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/dashboard']}>
      <DateFormatProvider>
        <Routes>
          <Route path="/dashboard" element={<DashboardPage />} />
          <Route path="/contacts/:id" element={<div>CONTACT DETAIL PAGE</div>} />
        </Routes>
      </DateFormatProvider>
    </MemoryRouter>
  );
}

test('fetches the dashboard composite once and renders all four blocks', async () => {
  getDashboardMock.mockResolvedValue({
    birthdays: [{ type: 'contact', name: 'Bea Birthday', birthday: '--08-20', contact_id: 1 }],
    random_contacts: [{ ID: 2, firstname: 'Randy', lastname: 'Contact', archived: false }],
    favorites: [{ ID: 4, firstname: 'Fay', lastname: 'Vorite', is_favorite: true }],
    upcoming_reminders: [
      { ID: 9, message: 'Call Nicky', by_mail: false, remind_at: '2026-08-12T00:00:00Z', recurrence: 'once', reoccur_from_completion: true, completed: false, email_sent: false, contact_id: 3, contact_name: 'Nicky Name' },
    ],
    overdue: [],
    reach_out_suggestions: [],
  });

  renderPage();

  await waitFor(() => expect(screen.getByText('Bea Birthday')).toBeInTheDocument());
  expect(screen.getByText('Randy Contact')).toBeInTheDocument();
  // Issue #173: the favorites block renders its contacts.
  expect(screen.getByText('Fay Vorite')).toBeInTheDocument();
  expect(screen.getByText('Call Nicky')).toBeInTheDocument();
  // The reminder's contact_name is embedded server-side (M3 design decision
  // 2) -- no separate per-reminder contact fetch happens.
  expect(screen.getByText('Nicky Name')).toBeInTheDocument();
  expect(getDashboardMock).toHaveBeenCalledTimes(1);
});

test('empty dashboard renders each column\'s empty state without crashing', async () => {
  getDashboardMock.mockResolvedValue(emptyDashboard());

  renderPage();

  await waitFor(() => expect(screen.getByText('No upcoming birthdays')).toBeInTheDocument());
  expect(screen.getByText('No upcoming reminders')).toBeInTheDocument();
  // Issue #173: the favorites block's empty state.
  expect(screen.getByText('No favorites yet')).toBeInTheDocument();
});

test('completing a reminder refetches via the plain upcoming-reminders endpoint and keeps the known contact name', async () => {
  getDashboardMock.mockResolvedValue({
    ...emptyDashboard(),
    upcoming_reminders: [
      { ID: 9, message: 'Call Nicky', by_mail: false, remind_at: '2026-08-12T00:00:00Z', recurrence: 'once', reoccur_from_completion: true, completed: false, email_sent: false, contact_id: 3, contact_name: 'Nicky Name' },
    ],
  });
  completeReminderMock.mockResolvedValue({ message: 'ok' });
  getUpcomingRemindersMock.mockResolvedValue([
    { ID: 9, message: 'Call Nicky', by_mail: false, remind_at: '2026-08-19T00:00:00Z', recurrence: 'weekly', reoccur_from_completion: true, completed: false, email_sent: false, contact_id: 3 },
  ]);

  renderPage();

  await waitFor(() => expect(screen.getByText('Call Nicky')).toBeInTheDocument());
  fireEvent.click(screen.getByRole('button', { name: 'Complete' }));

  await waitFor(() => expect(completeReminderMock).toHaveBeenCalledWith(9));
  await waitFor(() => expect(getUpcomingRemindersMock).toHaveBeenCalledTimes(1));
  // The refetched reminder carries no contact_name from the plain endpoint;
  // the page must carry the previously-known name forward.
  await waitFor(() => expect(screen.getByText('Nicky Name')).toBeInTheDocument());
});
