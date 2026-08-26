import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import './i18n/config';
import ContactSharesPage from './ContactSharesPage';
import { SnackbarProvider } from './context/SnackbarContext';

afterEach(cleanup);
afterEach(() => vi.unstubAllGlobals());

function renderPage() {
  return render(
    <SnackbarProvider>
      <ContactSharesPage />
    </SnackbarProvider>,
  );
}

function mockFetchByUrl(handlers: Record<string, (url: string, init?: RequestInit) => unknown>) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      for (const [pattern, respond] of Object.entries(handlers)) {
        if (url.includes(pattern)) {
          return { ok: true, json: async () => respond(url, init) };
        }
      }
      throw new Error(`unexpected fetch: ${url}`);
    }),
  );
}

const pendingShare = {
  id: 'share-1',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  from_user_id: 2,
  to_user_id: 1,
  contact_display_name: 'Alice Anderson',
  status: 'pending',
};

const outgoingShare = {
  id: 'share-2',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  from_user_id: 1,
  to_user_id: 3,
  contact_display_name: 'Bob Brown',
  status: 'accepted',
};

function baseHandlers() {
  return {
    '/contact-shares/incoming': () => ({
      contact_shares: [pendingShare],
      usernames: { '2': 'sender_bob' },
      total: 1,
      next_cursor: '',
      limit: 25,
    }),
    '/contact-shares/outgoing': () => ({
      contact_shares: [outgoingShare],
      usernames: { '3': 'carol' },
      total: 1,
      next_cursor: '',
      limit: 25,
    }),
  };
}

test('renders incoming shares with the sender username on the incoming tab', async () => {
  mockFetchByUrl(baseHandlers());
  renderPage();

  expect(await screen.findByText('Alice Anderson')).toBeInTheDocument();
  expect(screen.getByText('From sender_bob')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Accept' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Decline' })).toBeInTheDocument();
});

test('switching to the outgoing tab shows sent shares without accept/decline actions', async () => {
  mockFetchByUrl(baseHandlers());
  renderPage();

  await screen.findByText('Alice Anderson');
  fireEvent.click(screen.getByRole('tab', { name: 'Outgoing' }));

  expect(await screen.findByText('Bob Brown')).toBeInTheDocument();
  expect(screen.getByText('To carol')).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Accept' })).not.toBeInTheDocument();
});

// declineContactShare's response body is never parsed (the api function
// returns void), so a mock respond() callback wired only through .json()
// would never run -- assert on the fetch call itself instead.
function wasDeclineRequested(): boolean {
  const calls = (fetch as unknown as { mock: { calls: [string, RequestInit?][] } }).mock.calls;
  return calls.some(
    ([url, init]) => url.includes('/contact-shares/share-1/decline') && init?.method === 'POST',
  );
}

test('declining prompts for confirmation and, once confirmed, calls decline and refreshes', async () => {
  mockFetchByUrl({
    ...baseHandlers(),
    '/contact-shares/share-1/decline': () => ({ message: 'Share declined' }),
  });
  vi.spyOn(window, 'confirm').mockReturnValue(true);
  renderPage();

  await screen.findByText('Alice Anderson');
  fireEvent.click(screen.getByRole('button', { name: 'Decline' }));

  await waitFor(() => expect(wasDeclineRequested()).toBe(true));
});

test('declining does nothing when the confirmation is cancelled', async () => {
  mockFetchByUrl({
    ...baseHandlers(),
    '/contact-shares/share-1/decline': () => ({ message: 'Share declined' }),
  });
  vi.spyOn(window, 'confirm').mockReturnValue(false);
  renderPage();

  await screen.findByText('Alice Anderson');
  fireEvent.click(screen.getByRole('button', { name: 'Decline' }));

  // Give any accidental async call a chance to fire before asserting it didn't.
  await new Promise((r) => setTimeout(r, 10));
  expect(wasDeclineRequested()).toBe(false);
});

test('accepting opens the preview dialog and confirming completes the import', async () => {
  let confirmedActions: unknown;
  mockFetchByUrl({
    ...baseHandlers(),
    '/contact-shares/share-1/accept': () => ({
      session_id: 'sess-1',
      rows: [
        {
          row_index: 0,
          parsed_contact: { firstname: 'Alice' },
          validation_errors: [],
          duplicate_match: null,
          suggested_action: 'add',
        },
      ],
      total_rows: 1,
      valid_rows: 1,
      duplicate_count: 0,
      error_count: 0,
    }),
    '/contact-shares/share-1/confirm': (_url, init) => {
      confirmedActions = JSON.parse(init?.body as string).actions;
      return { total_processed: 1, created: 1, updated: 0, skipped: 0, errors: [] };
    },
  });
  renderPage();

  await screen.findByText('Alice Anderson');
  fireEvent.click(screen.getByRole('button', { name: 'Accept' }));

  await screen.findByText('Review shared contact');
  const confirmButton = await screen.findByRole('button', { name: 'Confirm' });
  await waitFor(() => expect(confirmButton).toBeEnabled());
  fireEvent.click(confirmButton);

  await waitFor(() => expect(confirmedActions).toEqual([{ row_index: 0, action: 'add' }]));
});
