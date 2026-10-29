import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import './i18n/config';
import ActivitiesPage from './ActivitiesPage';
import { AnnouncerProvider } from './context/AnnouncerContext';
import { DateFormatProvider } from './DateFormatProvider';
import { pickDate } from './testSupport/dateField';

// Issue #2006: the Activities page lists a range, edits the end date, sends
// null to clear it, and shows the server's end_date refusal on the end field.

beforeEach(() => {
  localStorage.setItem(
    'user_info',
    JSON.stringify({ user_id: 1, username: 'test', is_admin: false }),
  );
});

afterEach(() => {
  cleanup();
  localStorage.clear();
  vi.unstubAllGlobals();
});

interface Call {
  url: string;
  method: string;
  body?: Record<string, unknown>;
}

function mockApi(put: { status: number; body: unknown }) {
  const calls: Call[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, options?: RequestInit) => {
      const method = (options?.method || 'GET').toUpperCase();
      calls.push({
        url,
        method,
        body: options?.body ? JSON.parse(options.body as string) : undefined,
      });
      let status = 200;
      let body: unknown = {};
      if (url.includes('/activities?')) {
        body = { activities: [rangedActivity()], next_cursor: '', limit: 25 };
      } else if (url.includes('/contacts?')) {
        body = { contacts: [], next_cursor: '', limit: 25, total: 0 };
      } else if (url.includes('/activities/1') && method === 'PUT') {
        status = put.status;
        body = put.body;
      } else {
        throw new Error(`unexpected fetch: ${method} ${url}`);
      }
      return { ok: status < 300, status, statusText: `status ${status}`, json: async () => body };
    }),
  );
  return calls;
}

function rangedActivity() {
  return {
    ID: 1,
    revision: 5,
    title: 'Family trip',
    description: '',
    location: '',
    date: '2026-05-12T00:00:00Z',
    end_date: '2026-05-15T00:00:00Z',
    CreatedAt: '2026-01-01T00:00:00Z',
    UpdatedAt: '2026-01-01T00:00:00Z',
    contacts: [],
  };
}

function renderPage() {
  return render(
    <MemoryRouter>
      <AnnouncerProvider>
        <DateFormatProvider>
          <ActivitiesPage />
        </DateFormatProvider>
      </AnnouncerProvider>
    </MemoryRouter>,
  );
}

test('lists a multi-day activity as a range', async () => {
  mockApi({ status: 200, body: rangedActivity() });
  renderPage();
  await waitFor(() => expect(screen.getAllByText('12.05.2026 – 15.05.2026')).toHaveLength(2));
});

test('editing keeps the end date and sends it as an instant', async () => {
  const calls = mockApi({ status: 200, body: rangedActivity() });
  renderPage();
  await waitFor(() => expect(screen.getByText('Family trip')).toBeDefined());

  fireEvent.click(screen.getByRole('button', { name: 'Edit Activity' }));
  await waitFor(() =>
    expect(screen.getByLabelText('End date (optional)')).toHaveValue('15.05.2026'),
  );
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await waitFor(() => {
    const put = calls.find((c) => c.method === 'PUT');
    expect(put?.body).toMatchObject({
      date: '2026-05-12T00:00:00.000Z',
      end_date: '2026-05-15T00:00:00.000Z',
    });
  });
});

test('clearing the end date sends end_date null', async () => {
  const calls = mockApi({ status: 200, body: { ...rangedActivity(), end_date: undefined } });
  renderPage();
  await waitFor(() => expect(screen.getByText('Family trip')).toBeDefined());

  fireEvent.click(screen.getByRole('button', { name: 'Edit Activity' }));
  await waitFor(() =>
    expect(screen.getByLabelText('End date (optional)')).toHaveValue('15.05.2026'),
  );
  fireEvent.change(screen.getByLabelText('End date (optional)'), { target: { value: '' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await waitFor(() => {
    const put = calls.find((c) => c.method === 'PUT');
    expect(put?.body).toBeDefined();
    expect(put?.body).toHaveProperty('end_date', null);
  });
});

test("the server's 400 on end_date shows on the end field and keeps the editor open", async () => {
  mockApi({
    status: 400,
    body: {
      error: {
        code: 'INVALID_INPUT',
        message: "Invalid value for field 'end_date'",
        details: { field: 'end_date', reason: 'end date precedes the start date' },
        reason_code: 'activity_end_before_start',
      },
    },
  });
  renderPage();
  await waitFor(() => expect(screen.getByText('Family trip')).toBeDefined());

  fireEvent.click(screen.getByRole('button', { name: 'Edit Activity' }));
  await waitFor(() => expect(screen.getByLabelText('End date (optional)')).toBeDefined());
  await pickDate('Date', '2026-05-12', '2026-05-12').catch(() => undefined);
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(await screen.findByText('The end date cannot be before the start date')).toBeDefined();
  expect(screen.getByLabelText('End date (optional)')).toBeDefined();
});
