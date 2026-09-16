import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import './i18n/config';
import ActivitiesPage from './ActivitiesPage';
import { AnnouncerProvider } from './context/AnnouncerContext';
import { DateFormatProvider } from './DateFormatProvider';

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

interface RouteHandler {
  method?: string;
  status?: number;
  body?: unknown;
}

interface Call {
  url: string;
  method: string;
  body?: unknown;
}

// Route-based fetch mock: unlike a flat url->response map, this page needs
// per-method responses on the same URL (GET /activities for the list vs.
// PUT/DELETE /activities/:id for edit/delete) and a recorded call log to
// assert on query params (cursor, search, fromDate/toDate) and request bodies.
function mockFetchRoutes(
  routes: Array<{ pattern: string; handler: RouteHandler | RouteHandler[] }>,
) {
  const calls: Call[] = [];
  const callCounts = new Map<string, number>();
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, options?: RequestInit) => {
      const method = (options?.method || 'GET').toUpperCase();
      calls.push({
        url,
        method,
        body: options?.body ? JSON.parse(options.body as string) : undefined,
      });
      for (const { pattern, handler } of routes) {
        if (!url.includes(pattern)) continue;
        const handlers = Array.isArray(handler) ? handler : [handler];
        const matching = handlers.filter((h) => !h.method || h.method === method);
        if (matching.length === 0) continue;
        const key = `${method} ${pattern}`;
        const idx = callCounts.get(key) || 0;
        callCounts.set(key, idx + 1);
        const h = matching[Math.min(idx, matching.length - 1)];
        const status = h.status ?? 200;
        return {
          ok: status >= 200 && status < 300,
          status,
          statusText: `status ${status}`,
          json: async () => h.body ?? {},
        };
      }
      throw new Error(`unexpected fetch: ${method} ${url}`);
    }),
  );
  return calls;
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

const emptyContactsPage = () => ({ contacts: [], next_cursor: '', limit: 25, total: 0 });

function activity(overrides: Record<string, unknown> = {}) {
  return {
    ID: 1,
    title: 'Coffee catch-up',
    description: 'Long overdue',
    location: 'Downtown Cafe',
    date: '2026-01-05T00:00:00Z',
    CreatedAt: '2026-01-01T00:00:00Z',
    UpdatedAt: '2026-01-01T00:00:00Z',
    contacts: [{ ID: 5, firstname: 'Bob', lastname: 'Jones' }],
    ...overrides,
  };
}

test('the initial fetch requests contacts included at the page size', async () => {
  const calls = mockFetchRoutes([
    { pattern: '/activities?', handler: { body: { activities: [], next_cursor: '', limit: 25 } } },
  ]);

  renderPage();

  await waitFor(() => {
    expect(screen.getByText('No activities found')).toBeDefined();
  });

  const listCall = calls.find((c) => c.method === 'GET' && c.url.includes('/activities?'));
  expect(listCall?.url).toContain('limit=25');
  expect(listCall?.url).toContain('include=contacts');
});

test('typing a search term debounces before issuing a new request', async () => {
  const calls = mockFetchRoutes([
    {
      pattern: '/activities?',
      handler: { body: { activities: [activity()], next_cursor: '', limit: 25 } },
    },
  ]);

  renderPage();
  await waitFor(() => {
    expect(screen.getByText('Coffee catch-up')).toBeDefined();
  });
  const countBeforeTyping = calls.length;

  fireEvent.change(screen.getByLabelText('Search activities...'), {
    target: { value: 'Coffee' },
  });

  // The 400ms debounce hasn't elapsed yet -- no new request should have gone
  // out synchronously off the keystroke.
  expect(calls.length).toBe(countBeforeTyping);

  await waitFor(
    () => {
      expect(calls.some((c) => c.url.includes('search=Coffee'))).toBe(true);
    },
    { timeout: 2000 },
  );
});

test('from/to date filters are sent as fromDate/toDate query params', async () => {
  const calls = mockFetchRoutes([
    { pattern: '/activities?', handler: { body: { activities: [], next_cursor: '', limit: 25 } } },
  ]);

  renderPage();
  await waitFor(() => expect(screen.getByText('No activities found')).toBeDefined());

  fireEvent.change(screen.getByLabelText('From'), { target: { value: '2026-01-01' } });
  fireEvent.change(screen.getByLabelText('To'), { target: { value: '2026-01-31' } });

  await waitFor(() => {
    expect(
      calls.some(
        (c) => c.url.includes('fromDate=2026-01-01') && c.url.includes('toDate=2026-01-31'),
      ),
    ).toBe(true);
  });

  // hasFilters is now true, so an empty result renders the filtered-empty copy.
  await waitFor(() => {
    expect(screen.getByText('No activities match your filters')).toBeDefined();
  });
});

test('Load more appends the next cursor page without dropping the first page', async () => {
  const calls = mockFetchRoutes([
    {
      pattern: '/activities?',
      handler: [
        {
          body: {
            activities: [activity({ ID: 1, title: 'First' })],
            next_cursor: 'cursor-abc',
            limit: 25,
          },
        },
        {
          body: {
            activities: [activity({ ID: 2, title: 'Second' })],
            next_cursor: '',
            limit: 25,
          },
        },
      ],
    },
  ]);

  renderPage();
  await waitFor(() => expect(screen.getByText('First')).toBeDefined());

  fireEvent.click(screen.getByRole('button', { name: 'Load more' }));

  await waitFor(() => {
    expect(screen.getByText('Second')).toBeDefined();
  });
  // Both pages are visible -- loadMore appends rather than replaces.
  expect(screen.getByText('First')).toBeDefined();

  const loadMoreCall = calls.filter((c) => c.method === 'GET').at(-1);
  expect(loadMoreCall?.url).toContain('cursor=cursor-abc');

  // The button disappears once next_cursor comes back empty.
  expect(screen.queryByRole('button', { name: 'Load more' })).toBeNull();
});

test('handleAddActivity creates the activity, closes the dialog, and refetches', async () => {
  const calls = mockFetchRoutes([
    {
      pattern: '/activities?',
      handler: { body: { activities: [], next_cursor: '', limit: 25 } },
    },
    {
      pattern: '/activities',
      handler: {
        method: 'POST',
        status: 201,
        body: activity({ ID: 9, title: 'New Meetup' }),
      },
    },
    { pattern: '/contacts?', handler: { body: emptyContactsPage() } },
  ]);

  renderPage();
  await waitFor(() => expect(screen.getByText('No activities found')).toBeDefined());
  const fetchCountBeforeCreate = calls.filter((c) => c.method === 'GET').length;

  fireEvent.click(screen.getByRole('button', { name: 'Add Activity' }));
  await waitFor(() => expect(screen.getByLabelText('Title *')).toBeDefined());

  fireEvent.change(screen.getByLabelText('Title *'), { target: { value: 'New Meetup' } });
  fireEvent.change(screen.getByLabelText('Date *'), { target: { value: '2026-02-01' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await waitFor(() => {
    expect(calls.some((c) => c.method === 'POST' && c.url.includes('/activities'))).toBe(true);
  });
  const createCall = calls.find((c) => c.method === 'POST');
  expect(createCall?.body).toMatchObject({ title: 'New Meetup' });

  // Dialog closes.
  await waitFor(() => {
    expect(screen.queryByLabelText('Title *')).toBeNull();
  });

  // refetch() ran: a new GET beyond the initial load.
  const fetchCountAfterCreate = calls.filter((c) => c.method === 'GET').length;
  expect(fetchCountAfterCreate).toBeGreaterThan(fetchCountBeforeCreate);
});

test('editing an activity prefills the dialog, fetches all contacts once, and saves the update', async () => {
  const calls = mockFetchRoutes([
    {
      pattern: '/activities?',
      handler: { body: { activities: [activity()], next_cursor: '', limit: 25 } },
    },
    { pattern: '/contacts?', handler: { body: emptyContactsPage() } },
    {
      pattern: '/activities/1',
      handler: { method: 'PUT', body: activity({ title: 'Coffee catch-up (updated)' }) },
    },
  ]);

  renderPage();
  await waitFor(() => expect(screen.getByText('Coffee catch-up')).toBeDefined());

  fireEvent.click(screen.getByRole('button', { name: 'Edit Activity' }));

  await waitFor(() => {
    expect(screen.getByDisplayValue('Coffee catch-up')).toBeDefined();
  });
  // getAllContacts was called exactly once (allContacts started empty).
  expect(calls.filter((c) => c.url.includes('/contacts?')).length).toBe(1);

  const titleInput = screen.getByDisplayValue('Coffee catch-up');
  fireEvent.change(titleInput, { target: { value: 'Coffee catch-up (updated)' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await waitFor(() => {
    const putCall = calls.find((c) => c.method === 'PUT');
    expect(putCall).toBeDefined();
    expect(putCall?.body).toMatchObject({ title: 'Coffee catch-up (updated)' });
  });

  await waitFor(() => {
    expect(screen.queryByDisplayValue('Coffee catch-up (updated)')).toBeNull();
  });
});

test('saving an edit with a blank title is a no-op guard, not a request', async () => {
  const calls = mockFetchRoutes([
    {
      pattern: '/activities?',
      handler: { body: { activities: [activity()], next_cursor: '', limit: 25 } },
    },
    { pattern: '/contacts?', handler: { body: emptyContactsPage() } },
  ]);

  renderPage();
  await waitFor(() => expect(screen.getByText('Coffee catch-up')).toBeDefined());

  fireEvent.click(screen.getByRole('button', { name: 'Edit Activity' }));
  await waitFor(() => expect(screen.getByDisplayValue('Coffee catch-up')).toBeDefined());

  fireEvent.change(screen.getByDisplayValue('Coffee catch-up'), { target: { value: '   ' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(calls.some((c) => c.method === 'PUT')).toBe(false);
  // Dialog remains open.
  expect(screen.getByRole('button', { name: 'Save' })).toBeDefined();
});

test('deleting an activity confirms, calls DELETE, and refetches', async () => {
  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
  const calls = mockFetchRoutes([
    {
      pattern: '/activities?',
      handler: { body: { activities: [activity()], next_cursor: '', limit: 25 } },
    },
    { pattern: '/contacts?', handler: { body: emptyContactsPage() } },
    { pattern: '/activities/1', handler: { method: 'DELETE', status: 204, body: undefined } },
  ]);

  renderPage();
  await waitFor(() => expect(screen.getByText('Coffee catch-up')).toBeDefined());

  fireEvent.click(screen.getByRole('button', { name: 'Edit Activity' }));
  await waitFor(() => expect(screen.getByDisplayValue('Coffee catch-up')).toBeDefined());

  fireEvent.click(screen.getByRole('button', { name: 'Delete' }));

  await waitFor(() => {
    expect(calls.some((c) => c.method === 'DELETE' && c.url.includes('/activities/1'))).toBe(true);
  });
  expect(confirmSpy).toHaveBeenCalled();

  await waitFor(() => {
    expect(screen.queryByDisplayValue('Coffee catch-up')).toBeNull();
  });
  confirmSpy.mockRestore();
});
