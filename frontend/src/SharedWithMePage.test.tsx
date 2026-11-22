import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, describe, expect, test, vi } from 'vitest';
import './i18n/config';
import { AnnouncerProvider } from './context/AnnouncerContext';
import { PendingSharesContext } from './context/PendingSharesContext';
import { DateFormatProvider } from './DateFormatProvider';
import SharedWithMePage from './SharedWithMePage';
import { errorEnvelope, mockApi, server, setupMswServer } from './test/mswServer';

setupMswServer();
afterEach(cleanup);

const pending = {
  id: 'p1',
  from_username: 'bob',
  entity_type: 'contact',
  label: 'Alice Anderson',
  sections: ['emails', 'phones'],
  status: 'pending',
  expires_at: null,
  responded_at: null,
  created_at: '2026-01-01T00:00:00Z',
};
const active = {
  id: 'a1',
  from_username: 'carol',
  entity_type: 'circle',
  label: 'Book Club',
  sections: [],
  status: 'active',
  expires_at: '2099-02-03T10:00:00Z',
  responded_at: '2026-01-02T00:00:00Z',
  created_at: '2026-01-01T00:00:00Z',
};

function page(grants: unknown[], next = '') {
  return { share_grants: grants, next_cursor: next, limit: 25 };
}

function renderPage() {
  return render(
    <MemoryRouter>
      <AnnouncerProvider>
        <DateFormatProvider>
          <SharedWithMePage />
        </DateFormatProvider>
      </AnnouncerProvider>
    </MemoryRouter>,
  );
}

async function rowFor(label: string): Promise<HTMLElement> {
  const li = (await screen.findByText(label)).closest('li');
  if (!li) throw new Error(`no list row for ${label}`);
  return li;
}

function setup(grants: unknown[] = [pending, active], blocks: unknown[] = []) {
  mockApi('get', '/share-grants/incoming', page(grants));
  mockApi('get', '/share-grants/blocks', { blocks });
}

describe('SharedWithMePage', () => {
  test('a pending grant shows only sender, type and label: no sections, no content, no view button', async () => {
    const content = mockApi('get', '/share-grants/p1/content', {});
    setup([pending]);
    renderPage();
    const row = await rowFor('Alice Anderson');
    expect(within(row).getByText('From bob')).toBeInTheDocument();
    expect(within(row).getByText('Contact')).toBeInTheDocument();
    expect(within(row).getByText(/once you accept/i)).toBeInTheDocument();
    expect(within(row).queryByText(/Sections:/)).not.toBeInTheDocument();
    expect(within(row).queryByText(/emails/i)).not.toBeInTheDocument();
    expect(within(row).queryByRole('button', { name: /^View/ })).not.toBeInTheDocument();
    expect(within(row).getByRole('button', { name: /^Accept/ })).toBeInTheDocument();
    expect(within(row).getByRole('button', { name: /^Decline/ })).toBeInTheDocument();
    expect(content).toHaveLength(0);
  });

  test('pending and active grants sit under separate headings; active offers view and stop receiving', async () => {
    setup();
    renderPage();
    expect(await screen.findByRole('heading', { name: 'Waiting for your answer' })).toBeVisible();
    expect(screen.getByRole('heading', { name: 'Shared with you' })).toBeVisible();
    const row = await rowFor('Book Club');
    expect(within(row).getByRole('button', { name: /^View/ })).toBeInTheDocument();
    expect(within(row).getByRole('button', { name: /^Stop receiving/ })).toBeInTheDocument();
    expect(within(row).queryByRole('button', { name: /^Accept/ })).not.toBeInTheDocument();
    expect(within(row).queryByRole('button', { name: /^Block/ })).not.toBeInTheDocument();
  });

  // WCAG 2.5.3 (#2700): voice control says the visible text, so every accessible
  // name must contain it (axe label-content-name-mismatch).
  test('every action button name contains its visible text', async () => {
    setup(
      [pending, active],
      [{ user_id: 7, username: 'dave', created_at: '2026-01-01T00:00:00Z' }],
    );
    renderPage();
    await rowFor('Alice Anderson');
    await screen.findByRole('button', { name: 'Unblock dave' });
    const buttons = screen.getAllByRole('button').filter((b) => b.hasAttribute('aria-label'));
    expect(buttons.length).toBeGreaterThan(4);
    for (const b of buttons) {
      const visible = (b.textContent ?? '').trim().toLowerCase();
      const name = (b.getAttribute('aria-label') ?? '').toLowerCase();
      expect(name, `aria-label "${name}" must contain "${visible}"`).toContain(visible);
    }
  });

  test('accepting posts accept, moves the row to active and announces it', async () => {
    setup([pending]);
    const accept = mockApi('post', '/share-grants/p1/accept', {
      share_grant: { ...pending, status: 'active' },
    });
    renderPage();
    fireEvent.click(await screen.findByRole('button', { name: /^Accept Alice Anderson/ }));
    await waitFor(() => expect(accept).toHaveLength(1));
    expect(await screen.findByRole('button', { name: /^View Alice Anderson/ })).toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Waiting for your answer' })).toBeNull();
    await waitFor(() =>
      expect(screen.getByRole('status')).toHaveTextContent('Accepted Alice Anderson from bob.'),
    );
  });

  test('declining posts block:false, removes the row and announces it', async () => {
    setup([pending]);
    const decline = mockApi('post', '/share-grants/p1/decline', {
      share_grant: { ...pending, status: 'declined' },
    });
    renderPage();
    fireEvent.click(await screen.findByRole('button', { name: /^Decline Alice Anderson/ }));
    await waitFor(() => expect(decline).toHaveLength(1));
    expect(decline[0].body).toEqual({ block: false });
    await waitFor(() => expect(screen.queryByText('Alice Anderson')).not.toBeInTheDocument());
    await waitFor(() =>
      expect(screen.getByRole('status')).toHaveTextContent('Declined Alice Anderson from bob.'),
    );
  });

  test('blocking a sender needs confirmation, declines with block:true and lists them as blocked', async () => {
    mockApi('get', '/share-grants/incoming', page([pending]));
    let blocks: unknown[] = [];
    const { http, HttpResponse } = await import('msw');
    const { API_BASE_URL } = await import('./api/client');
    server.use(
      http.get(`${API_BASE_URL}/share-grants/blocks`, () => HttpResponse.json({ blocks })),
    );
    const decline = mockApi('post', '/share-grants/p1/decline', {
      share_grant: { ...pending, status: 'declined' },
    });
    renderPage();
    fireEvent.click(await screen.findByRole('button', { name: 'Block sender bob' }));
    const dialog = await screen.findByRole('dialog', { name: 'Block this sender?' });
    expect(within(dialog).getByText(/not told that you blocked them/i)).toBeInTheDocument();
    expect(decline).toHaveLength(0); // nothing sent until confirmed
    blocks = [{ user_id: 7, username: 'bob', created_at: '2026-01-01T00:00:00Z' }];
    fireEvent.click(within(dialog).getByRole('button', { name: 'Block bob' }));
    await waitFor(() => expect(decline).toHaveLength(1));
    expect(decline[0].body).toEqual({ block: true });
    expect(await screen.findByRole('button', { name: 'Unblock bob' })).toBeInTheDocument();
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('Blocked bob.'));
  });

  test('cancelling the block confirmation sends nothing', async () => {
    setup([pending]);
    const decline = mockApi('post', '/share-grants/p1/decline', {});
    renderPage();
    fireEvent.click(await screen.findByRole('button', { name: 'Block sender bob' }));
    const dialog = await screen.findByRole('dialog', { name: 'Block this sender?' });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(decline).toHaveLength(0);
  });

  test('unblocking deletes the block and announces it', async () => {
    setup([], [{ user_id: 7, username: 'bob', created_at: '2026-01-01T00:00:00Z' }]);
    const del = mockApi('delete', '/share-grants/blocks/7', new Response(null, { status: 204 }));
    renderPage();
    fireEvent.click(await screen.findByRole('button', { name: 'Unblock bob' }));
    await waitFor(() => expect(del).toHaveLength(1));
    await waitFor(() => expect(screen.queryByRole('button', { name: 'Unblock bob' })).toBeNull());
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('Unblocked bob.'));
  });

  test('leaving an active grant confirms, posts leave and removes the row', async () => {
    setup([active]);
    const leave = mockApi('post', '/share-grants/a1/leave', {
      share_grant: { ...active, status: 'declined' },
    });
    renderPage();
    fireEvent.click(await screen.findByRole('button', { name: /^Stop receiving Book Club/ }));
    const dialog = await screen.findByRole('dialog', { name: 'Stop receiving this share?' });
    expect(leave).toHaveLength(0);
    fireEvent.click(within(dialog).getByRole('button', { name: 'Stop receiving' }));
    await waitFor(() => expect(leave).toHaveLength(1));
    await waitFor(() => expect(screen.queryByText('Book Club')).not.toBeInTheDocument());
    await waitFor(() =>
      expect(screen.getByRole('status')).toHaveTextContent(
        'Stopped receiving Book Club from carol.',
      ),
    );
  });

  test('an action error is shown and the row stays', async () => {
    setup([pending]);
    mockApi(
      'post',
      '/share-grants/p1/accept',
      errorEnvelope(409, 'CONFLICT', 'no longer possible'),
    );
    renderPage();
    fireEvent.click(await screen.findByRole('button', { name: /^Accept Alice Anderson/ }));
    expect(await screen.findByRole('alert')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^Accept Alice Anderson/ })).toBeInTheDocument();
  });

  test('a load failure shows a retry instead of the empty state', async () => {
    mockApi('get', '/share-grants/incoming', errorEnvelope(500, 'INTERNAL', 'boom'));
    mockApi('get', '/share-grants/blocks', { blocks: [] });
    renderPage();
    expect(await screen.findByRole('button', { name: 'Retry' })).toBeInTheDocument();
    expect(screen.queryByText('Nobody is sharing anything with you.')).toBeNull();
  });

  test('shows the empty state', async () => {
    setup([], []);
    renderPage();
    expect(await screen.findByText('Nobody is sharing anything with you.')).toBeInTheDocument();
    expect(screen.getByText('You have not blocked anyone.')).toBeInTheDocument();
  });

  test('load more appends the next cursor page', async () => {
    const calls = mockApi('get', '/share-grants/incoming', page([active], 'c2'));
    mockApi('get', '/share-grants/blocks', { blocks: [] });
    renderPage();
    await screen.findByText('Book Club');
    mockApi('get', '/share-grants/incoming', page([{ ...active, id: 'a2', label: 'Hikers' }], ''));
    fireEvent.click(screen.getByRole('button', { name: 'Load more' }));
    expect(await screen.findByText('Hikers')).toBeInTheDocument();
    expect(screen.getByText('Book Club')).toBeInTheDocument();
    expect(calls).toHaveLength(1);
  });
});

describe('SharedWithMePage failure states', () => {
  test('a blocked-senders load failure shows a retry that re-reads the list', async () => {
    mockApi('get', '/share-grants/incoming', page([]));
    const failing = mockApi('get', '/share-grants/blocks', errorEnvelope(500, 'INTERNAL', 'boom'));
    renderPage();
    fireEvent.click(await screen.findByRole('button', { name: 'Retry' }));
    await waitFor(() => expect(failing.length).toBeGreaterThan(1));
  });

  test('a failed unblock shows a dismissible error and keeps the sender listed', async () => {
    setup([], [{ user_id: 7, username: 'bob', created_at: '2026-01-01T00:00:00Z' }]);
    mockApi('delete', '/share-grants/blocks/7', errorEnvelope(500, 'INTERNAL', 'nope'));
    renderPage();
    fireEvent.click(await screen.findByRole('button', { name: 'Unblock bob' }));
    const alert = await screen.findByRole('alert');
    expect(screen.getByRole('button', { name: 'Unblock bob' })).toBeInTheDocument();
    fireEvent.click(within(alert).getByRole('button', { name: /close/i }));
    await waitFor(() => expect(screen.queryByRole('alert')).toBeNull());
  });

  test('retrying a failed grant load re-reads the grants', async () => {
    const failing = mockApi(
      'get',
      '/share-grants/incoming',
      errorEnvelope(500, 'INTERNAL', 'boom'),
    );
    mockApi('get', '/share-grants/blocks', { blocks: [] });
    renderPage();
    fireEvent.click(await screen.findByRole('button', { name: 'Retry' }));
    await waitFor(() => expect(failing.length).toBeGreaterThan(1));
  });
});

describe('SharedWithMePage read view', () => {
  async function openView(label: RegExp) {
    fireEvent.click(await screen.findByRole('button', { name: label }));
    return screen.findByRole('dialog');
  }

  test('a contact grant renders only the allowlisted display fields', async () => {
    mockApi(
      'get',
      '/share-grants/incoming',
      page([{ ...active, id: 'c1', entity_type: 'contact', label: 'Alice Anderson' }]),
    );
    mockApi('get', '/share-grants/blocks', { blocks: [] });
    mockApi('get', '/share-grants/c1/content', {
      share_grant_content: {
        grant_id: 'c1',
        entity_type: 'contact',
        owner_username: 'carol',
        contact: {
          uid: 'hidden-uid',
          name: { full: 'Alice Anderson' },
          emails: { e1: { address: 'alice@example.com' } },
          'crm:internal': { note: 'must not show' },
        },
      },
    });
    renderPage();
    const dialog = await openView(/^View Alice Anderson/);
    expect(await within(dialog).findByText('alice@example.com')).toBeInTheDocument();
    expect(within(dialog).getByText(/Read-only/)).toBeInTheDocument();
    expect(dialog).not.toHaveTextContent('hidden-uid');
    expect(dialog).not.toHaveTextContent('must not show');
  });

  test('a note grant shows its text and date', async () => {
    mockApi(
      'get',
      '/share-grants/incoming',
      page([{ ...active, id: 'n1', entity_type: 'note', label: 'Note' }]),
    );
    mockApi('get', '/share-grants/blocks', { blocks: [] });
    mockApi('get', '/share-grants/n1/content', {
      share_grant_content: {
        grant_id: 'n1',
        entity_type: 'note',
        owner_username: 'carol',
        note: { content: 'Remember the cake', date: '2026-03-04T00:00:00Z' },
      },
    });
    renderPage();
    const dialog = await openView(/^View Note/);
    expect(await within(dialog).findByText('Remember the cake')).toBeInTheDocument();
  });

  test('an activity grant shows title, description, location and type, never participants', async () => {
    mockApi(
      'get',
      '/share-grants/incoming',
      page([{ ...active, id: 'x1', entity_type: 'activity', label: 'Picnic' }]),
    );
    mockApi('get', '/share-grants/blocks', { blocks: [] });
    mockApi('get', '/share-grants/x1/content', {
      share_grant_content: {
        grant_id: 'x1',
        entity_type: 'activity',
        owner_username: 'carol',
        activity: {
          title: 'Picnic',
          description: 'Bring a blanket',
          location: 'The park',
          date: '2026-03-04T00:00:00Z',
          end_date: null,
          type: 'visit',
        },
      },
    });
    renderPage();
    const dialog = await openView(/^View Picnic/);
    expect(await within(dialog).findByText('Bring a blanket')).toBeInTheDocument();
    expect(within(dialog).getByText('The park')).toBeInTheDocument();
    expect(within(dialog).getByText('visit')).toBeInTheDocument();
  });

  test('a circle grant lists live member names and says when the list is truncated', async () => {
    setup([active]);
    mockApi('get', '/share-grants/a1/content', {
      share_grant_content: {
        grant_id: 'a1',
        entity_type: 'circle',
        owner_username: 'carol',
        circle: {
          name: 'Book Club',
          description: 'Monthly',
          color: 'blue',
          member_count: 120,
          members: [{ display_name: 'Ann Lee' }, { display_name: 'Ben Ray' }],
          members_truncated: true,
        },
      },
    });
    renderPage();
    const dialog = await openView(/^View Book Club/);
    expect(await within(dialog).findByText('Ann Lee')).toBeInTheDocument();
    expect(within(dialog).getByText('Ben Ray')).toBeInTheDocument();
    expect(within(dialog).getByText('120 members')).toBeInTheDocument();
    expect(within(dialog).getByText('Showing the first 2 of 120 members.')).toBeInTheDocument();
  });

  test('a grant that ended in the meantime reads as unavailable (the identical 404)', async () => {
    setup([active]);
    mockApi('get', '/share-grants/a1/content', errorEnvelope(404, 'NOT_FOUND', 'gone'));
    renderPage();
    const dialog = await openView(/^View Book Club/);
    expect(
      await within(dialog).findByText('This share is no longer available.'),
    ).toBeInTheDocument();
  });
});

// #2691: answering a share does not navigate, so the app shell's nav badge is
// told to re-read the pending count after every mutation that can change it.
describe('SharedWithMePage pending-count refresh signal', () => {
  function renderWithSignal() {
    const refreshPendingShares = vi.fn();
    render(
      <MemoryRouter>
        <PendingSharesContext.Provider value={{ refreshPendingShares }}>
          <AnnouncerProvider>
            <DateFormatProvider>
              <SharedWithMePage />
            </DateFormatProvider>
          </AnnouncerProvider>
        </PendingSharesContext.Provider>
      </MemoryRouter>,
    );
    return refreshPendingShares;
  }

  test('accept refreshes once', async () => {
    setup([pending]);
    mockApi('post', '/share-grants/p1/accept', { share_grant: { ...pending, status: 'active' } });
    const refresh = renderWithSignal();
    fireEvent.click(await screen.findByRole('button', { name: /^Accept Alice Anderson/ }));
    await waitFor(() => expect(refresh).toHaveBeenCalledTimes(1));
  });

  test('decline refreshes once', async () => {
    setup([pending]);
    mockApi('post', '/share-grants/p1/decline', {
      share_grant: { ...pending, status: 'declined' },
    });
    const refresh = renderWithSignal();
    fireEvent.click(await screen.findByRole('button', { name: /^Decline Alice Anderson/ }));
    await waitFor(() => expect(refresh).toHaveBeenCalledTimes(1));
  });

  test('leave refreshes once', async () => {
    setup([active]);
    mockApi('post', '/share-grants/a1/leave', { share_grant: { ...active, status: 'declined' } });
    const refresh = renderWithSignal();
    fireEvent.click(await screen.findByRole('button', { name: /^Stop receiving Book Club/ }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Stop receiving' }));
    await waitFor(() => expect(refresh).toHaveBeenCalledTimes(1));
  });

  test('block refreshes once', async () => {
    setup([pending]);
    mockApi('post', '/share-grants/p1/decline', {
      share_grant: { ...pending, status: 'declined' },
    });
    const refresh = renderWithSignal();
    fireEvent.click(await screen.findByRole('button', { name: 'Block sender bob' }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Block bob' }));
    await waitFor(() => expect(refresh).toHaveBeenCalledTimes(1));
  });

  test('unblock refreshes once', async () => {
    setup([], [{ user_id: 7, username: 'bob', created_at: '2026-01-01T00:00:00Z' }]);
    mockApi('delete', '/share-grants/blocks/7', new Response(null, { status: 204 }));
    const refresh = renderWithSignal();
    fireEvent.click(await screen.findByRole('button', { name: 'Unblock bob' }));
    await waitFor(() => expect(refresh).toHaveBeenCalledTimes(1));
  });

  test('a failed accept does not refresh', async () => {
    setup([pending]);
    mockApi('post', '/share-grants/p1/accept', errorEnvelope(409, 'CONFLICT', 'nope'));
    const refresh = renderWithSignal();
    fireEvent.click(await screen.findByRole('button', { name: /^Accept Alice Anderson/ }));
    await screen.findByRole('alert');
    expect(refresh).not.toHaveBeenCalled();
  });
});

// #2744: the server labels a note grant with the note's first line once it is active; a
// pending one stays the generic "Note" (the recipient must accept before seeing content).
// The row title and every action's accessible name come straight from `label`.
describe('SharedWithMePage note grant labels (#2744)', () => {
  const pendingNote = { ...pending, id: 'n1', entity_type: 'note', label: 'Note', sections: [] };
  const activeNote = {
    ...active,
    id: 'n2',
    entity_type: 'note',
    label: 'Call the plumber about the leak\u2026',
    sections: [],
  };

  test('a pending note shows the generic label, an active note shows its first line', async () => {
    setup([pendingNote, activeNote]);
    renderPage();
    const acceptBtn = await screen.findByRole('button', { name: /^Accept Note from bob/ });
    const pendingRow = acceptBtn.closest('li') as HTMLElement;
    expect(within(pendingRow).getAllByText('Note')).toHaveLength(2); // title + type chip
    const activeRow = await rowFor('Call the plumber about the leak\u2026');
    expect(within(activeRow).getByText('Note')).toBeInTheDocument(); // the type chip only
    expect(
      within(activeRow).getByRole('button', {
        name: /^View Call the plumber about the leak\u2026/,
      }),
    ).toBeVisible();
  });
});
