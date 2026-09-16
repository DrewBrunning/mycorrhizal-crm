import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import './i18n/config';
import CircleTagTriagePage from './CircleTagTriagePage';
import { AnnouncerProvider } from './context/AnnouncerContext';
import { SnackbarProvider } from './context/SnackbarContext';

afterEach(() => {
  cleanup();
  localStorage.clear();
  vi.unstubAllGlobals();
});

beforeEach(() => {
  localStorage.setItem(
    'user_info',
    JSON.stringify({ user_id: 1, username: 'test', is_admin: false }),
  );
});

interface RouteHandler {
  method?: string;
  status?: number;
  body?: unknown;
}

// A richer fetch mock than the simple substring-match helper used elsewhere:
// this page needs per-method (GET vs POST) and per-status (409 vs generic
// failure) responses against the *same* URL pattern (e.g. POST /circles
// succeeding for one name and 409-ing for another), which a single
// url-keyed map can't express.
function mockFetchRoutes(
  routes: Array<{ pattern: string; handler: RouteHandler | RouteHandler[] }>,
) {
  const callCounts = new Map<string, number>();
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, options?: RequestInit) => {
      const method = (options?.method || 'GET').toUpperCase();
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
}

function renderPage() {
  return render(
    <SnackbarProvider>
      <AnnouncerProvider>
        <CircleTagTriagePage />
      </AnnouncerProvider>
    </SnackbarProvider>,
  );
}

test('collect sorts legacy circles by contact count descending', async () => {
  mockFetchRoutes([
    { pattern: '/contacts/circles?legacy=true', handler: { body: ['Small', 'Big'] } },
    {
      pattern: 'circle_legacy=Small',
      handler: { body: { contacts: [{ uid: 'c1' }], total: 1 } },
    },
    {
      pattern: 'circle_legacy=Big',
      handler: {
        body: { contacts: [{ uid: 'c2' }, { uid: 'c3' }, { uid: 'c4' }], total: 3 },
      },
    },
  ]);

  renderPage();

  await waitFor(() => {
    expect(screen.getAllByDisplayValue(/Small|Big/)).toHaveLength(2);
  });

  const nameInputs = screen.getAllByDisplayValue(/Small|Big/) as HTMLInputElement[];
  // Big (3 contacts) sorts before Small (1 contact).
  expect(nameInputs.map((i) => i.value)).toEqual(['Big', 'Small']);
});

test('shows the empty state when there are no legacy circles', async () => {
  mockFetchRoutes([{ pattern: '/contacts/circles?legacy=true', handler: { body: [] } }]);

  renderPage();

  await waitFor(() => {
    expect(
      screen.getByText('No legacy circles found. You may not need this migration.'),
    ).toBeDefined();
  });
});

test('surfaces a collect-level failure as an error alert', async () => {
  mockFetchRoutes([
    {
      pattern: '/contacts/circles?legacy=true',
      handler: { status: 500, body: { error: { code: 'INTERNAL', message: 'boom' } } },
    },
  ]);

  renderPage();

  await waitFor(() => {
    expect(screen.getByText('boom')).toBeDefined();
  });
});

test('a per-item contact-count lookup failure zeroes that item instead of failing collect', async () => {
  mockFetchRoutes([
    { pattern: '/contacts/circles?legacy=true', handler: { body: ['Family', 'Broken'] } },
    {
      pattern: 'circle_legacy=Family',
      handler: { body: { contacts: [{ uid: 'c1' }], total: 1 } },
    },
    {
      pattern: 'circle_legacy=Broken',
      handler: { status: 500, body: { error: { code: 'INTERNAL', message: 'boom' } } },
    },
  ]);

  renderPage();

  await waitFor(() => {
    expect(screen.getAllByDisplayValue(/Family|Broken/)).toHaveLength(2);
  });

  // Broken has 0 contacts (from the catch branch) and Family has 1, so
  // Family (higher count) sorts first.
  const nameInputs = screen.getAllByDisplayValue(/Family|Broken/) as HTMLInputElement[];
  expect(nameInputs.map((i) => i.value)).toEqual(['Family', 'Broken']);
  // No page-level error banner -- the failure was absorbed per-item.
  expect(screen.queryByText('boom')).toBeNull();
});

test('handleApply treats 409 as already-exists and reports other failures, tracking progress', async () => {
  mockFetchRoutes([
    { pattern: '/contacts/circles?legacy=true', handler: { body: ['Family', 'Coworkers'] } },
    {
      pattern: 'circle_legacy=Family',
      handler: { body: { contacts: [], total: 0 } },
    },
    {
      pattern: 'circle_legacy=Coworkers',
      handler: { body: { contacts: [], total: 0 } },
    },
    {
      pattern: '/circles',
      handler: [
        {
          method: 'POST',
          status: 201,
          body: { message: 'ok', circle: { id: 'circle-1', name: 'Family' } },
        },
        {
          method: 'POST',
          status: 409,
          body: { error: { code: 'ALREADY_EXISTS', message: 'circle exists' } },
        },
      ],
    },
  ]);

  renderPage();

  await waitFor(() => {
    expect(screen.getAllByDisplayValue(/Family|Coworkers/)).toHaveLength(2);
  });

  // Classify "Coworkers" (second, alphabetically-tied so both sort by count 0
  // in original collect order: Family then Coworkers) as a Tag isn't needed --
  // both default to "circle", which is exactly the branch under test (two
  // circle creations: one succeeds, one 409s).
  fireEvent.click(screen.getByRole('button', { name: 'Next' }));
  await waitFor(() => {
    expect(screen.getByRole('button', { name: 'Apply' })).toBeDefined();
  });
  fireEvent.click(screen.getByRole('button', { name: 'Apply' }));

  await waitFor(() => {
    expect(screen.getByText('Create Entities')).toBeDefined();
  });
  fireEvent.click(screen.getByRole('button', { name: 'Create Entities' }));

  await waitFor(() => {
    expect(screen.getByText('Created Circle: Family')).toBeDefined();
  });
  expect(screen.getByText('Circle "Coworkers" already exists, skipping')).toBeDefined();
});

test('handleApply reports a non-409 tag-creation failure verbatim', async () => {
  mockFetchRoutes([
    { pattern: '/contacts/circles?legacy=true', handler: { body: ['Diet'] } },
    { pattern: 'circle_legacy=Diet', handler: { body: { contacts: [], total: 0 } } },
    {
      pattern: '/tags',
      handler: {
        method: 'POST',
        status: 500,
        body: { error: { code: 'INTERNAL', message: 'db down' } },
      },
    },
  ]);

  renderPage();

  await waitFor(() => {
    expect(screen.getByDisplayValue('Diet')).toBeDefined();
  });

  // Reclassify the only item from Circle to Tag.
  fireEvent.mouseDown(screen.getByLabelText('Type'));
  fireEvent.click(screen.getByRole('option', { name: 'Tag' }));

  fireEvent.click(screen.getByRole('button', { name: 'Next' }));
  await waitFor(() => expect(screen.getByRole('button', { name: 'Apply' })).toBeDefined());
  fireEvent.click(screen.getByRole('button', { name: 'Apply' }));

  await waitFor(() => expect(screen.getByText('Create Entities')).toBeDefined());
  fireEvent.click(screen.getByRole('button', { name: 'Create Entities' }));

  await waitFor(() => {
    expect(screen.getByText(/Failed to create tag "Diet"/)).toBeDefined();
  });
});

test('handleAddMembers resolves entities, tolerates 409 on membership, and reports a missing entity', async () => {
  mockFetchRoutes([
    { pattern: '/contacts/circles?legacy=true', handler: { body: ['Family', 'Ghost'] } },
    {
      pattern: 'circle_legacy=Family',
      handler: { body: { contacts: [{ uid: 'u1' }, { uid: 'u2' }], total: 2 } },
    },
    {
      pattern: 'circle_legacy=Ghost',
      handler: { body: { contacts: [{ uid: 'u3' }], total: 1 } },
    },
    {
      pattern: '/circles?',
      handler: {
        method: 'GET',
        body: {
          circles: [{ id: 'circle-1', name: 'Family' }],
          total: 1,
          next_cursor: '',
          limit: 200,
        },
      },
    },
    {
      pattern: '/tags?',
      handler: { method: 'GET', body: { tags: [], total: 0, next_cursor: '', limit: 200 } },
    },
    {
      pattern: '/circles/circle-1/members',
      handler: [
        {
          method: 'POST',
          status: 201,
          body: { id: 1, circle_id: 'circle-1', member_vcard_uid: 'u1' },
        },
        {
          method: 'POST',
          status: 409,
          body: { error: { code: 'ALREADY_EXISTS', message: 'already a member' } },
        },
      ],
    },
  ]);

  renderPage();

  await waitFor(() => {
    expect(screen.getAllByDisplayValue(/Family|Ghost/)).toHaveLength(2);
  });

  fireEvent.click(screen.getByRole('button', { name: 'Next' }));
  await waitFor(() => expect(screen.getByRole('button', { name: 'Apply' })).toBeDefined());
  fireEvent.click(screen.getByRole('button', { name: 'Apply' }));

  await waitFor(() => expect(screen.getByText('Add Members')).toBeDefined());
  fireEvent.click(screen.getByRole('button', { name: 'Add Members' }));

  await waitFor(() => {
    expect(screen.getByText('Added 2 members to Circle "Family"')).toBeDefined();
  });
  // "Ghost" was classified as a circle but never created, so no matching
  // entity is found by name.
  expect(screen.getByText('Circle "Ghost" not found — create it first')).toBeDefined();
});

test('handleAddMembers reports a failure resolving circles/tags and stops before any membership calls', async () => {
  mockFetchRoutes([
    { pattern: '/contacts/circles?legacy=true', handler: { body: ['Family'] } },
    { pattern: 'circle_legacy=Family', handler: { body: { contacts: [], total: 0 } } },
    {
      pattern: '/circles?',
      handler: {
        method: 'GET',
        status: 500,
        body: { error: { code: 'INTERNAL', message: 'listCircles boom' } },
      },
    },
    {
      pattern: '/tags?',
      handler: { method: 'GET', body: { tags: [], total: 0, next_cursor: '', limit: 200 } },
    },
  ]);

  renderPage();

  await waitFor(() => {
    expect(screen.getByDisplayValue('Family')).toBeDefined();
  });

  fireEvent.click(screen.getByRole('button', { name: 'Next' }));
  await waitFor(() => expect(screen.getByRole('button', { name: 'Apply' })).toBeDefined());
  fireEvent.click(screen.getByRole('button', { name: 'Apply' }));

  await waitFor(() => expect(screen.getByText('Add Members')).toBeDefined());
  fireEvent.click(screen.getByRole('button', { name: 'Add Members' }));

  await waitFor(() => {
    expect(screen.getByText(/Failed to fetch circles\/tags/)).toBeDefined();
  });
});
