import { cleanup, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { afterEach, expect, test, vi } from 'vitest';
import './i18n/config';
import { SnackbarProvider } from './context/SnackbarContext';
import { DateFormatProvider } from './DateFormatProvider';
import PrepViewPage from './PrepViewPage';

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function mockBriefingFetch(data: unknown) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      if (url.includes('/briefing')) {
        return { ok: true, json: async () => data };
      }
      throw new Error(`unexpected fetch: ${url}`);
    }),
  );
}

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/contacts/1/prep']}>
      <SnackbarProvider>
        <DateFormatProvider>
          <Routes>
            <Route path="/contacts/:id/prep" element={<PrepViewPage />} />
          </Routes>
        </DateFormatProvider>
      </SnackbarProvider>
    </MemoryRouter>,
  );
}

const fullBriefing = {
  contact_id: 1,
  uid: 'alice-uid',
  name: 'Alice Wonder',
  kind: 'human',
  photo_thumbnail: '',
  last_activity: {
    id: 9,
    title: 'Coffee',
    type: 'visit',
    description: 'Talked about her garden plans',
    date: '2026-08-02T10:00:00Z',
  },
  recent_notes: [{ ID: 3, content: 'Talks about her garden', date: '2026-07-30T00:00:00Z' }],
  open_agenda_items: [{ id: 'a1', entity_id: 'alice-uid', content: 'Ask about the surgery' }],
  relationships: [
    {
      edge: { id: 'e1', source_id: 'bob-uid', target_id: 'alice-uid', type: 'spouse_of' },
      other_party_contact_id: 2,
      other_party_name: 'Bob Marley',
      display_token: 'spouse_of',
    },
  ],
  life_events: [{ id: 'le1', entity_id: 'alice-uid', type: 'graduated', description: 'PhD' }],
  upcoming_reminders: [{ ID: 5, message: 'Send card', remind_at: '2026-08-10T09:00:00Z' }],
  upcoming_dates: [{ label: 'birthday', date: '--01-15', days_until: 10 }],
  cadence: {
    policy: { id: 'p1', entity_id: 'alice-uid', target_interval_days: 30 },
    health: {
      has_qualifying_interaction: true,
      last_interaction: '2026-08-02T10:00:00Z',
      next_due: '2026-09-01T00:00:00Z',
      overdue_by: 3,
    },
  },
};

test('renders every briefing block from the composition endpoint', async () => {
  mockBriefingFetch(fullBriefing);
  renderPage();

  await waitFor(() => expect(screen.getByText('Alice Wonder')).toBeInTheDocument());

  expect(screen.getByText('Relationship health')).toBeInTheDocument();
  expect(screen.getByText('3 days overdue')).toBeInTheDocument();

  expect(screen.getByText('Things to bring up')).toBeInTheDocument();
  expect(screen.getByText(/Ask about the surgery/)).toBeInTheDocument();

  expect(screen.getByText('Last interaction')).toBeInTheDocument();
  expect(screen.getByText(/Coffee/)).toBeInTheDocument();
  expect(screen.getByText(/Talked about her garden plans/)).toBeInTheDocument();

  expect(screen.getByText('Recent notes')).toBeInTheDocument();
  expect(screen.getByText(/Talks about her garden/)).toBeInTheDocument();

  expect(screen.getByText('People around them')).toBeInTheDocument();
  expect(screen.getByText(/Bob Marley/)).toBeInTheDocument();

  expect(screen.getByText('Life events')).toBeInTheDocument();
  expect(screen.getByText(/Graduated/)).toBeInTheDocument();

  expect(screen.getByText('Upcoming reminders')).toBeInTheDocument();
  expect(screen.getByText(/Send card/)).toBeInTheDocument();

  expect(screen.getByText('Upcoming dates')).toBeInTheDocument();
});

test('degrades gracefully when every block is empty', async () => {
  mockBriefingFetch({
    contact_id: 1,
    uid: 'empty-uid',
    name: 'Empty',
    kind: '',
    photo_thumbnail: '',
    recent_notes: [],
    open_agenda_items: [],
    relationships: [],
    life_events: [],
    upcoming_reminders: [],
    upcoming_dates: [],
  });
  renderPage();

  await waitFor(() => expect(screen.getByText('Empty')).toBeInTheDocument());

  // Header renders, but no cadence card, no agenda, no interaction blocks.
  expect(screen.getByText('No interactions recorded yet')).toBeInTheDocument();
  expect(screen.queryByText('Things to bring up')).toBeNull();
  expect(screen.queryByText('Relationship health')).toBeNull();
  expect(screen.queryByText('People around them')).toBeNull();
});

// The test above passes `[]` for every block — a shape the server does NOT
// send. The blocks were tagged `omitempty` in Go, so a contact with no history
// came back as `{contact_id, uid, name, kind}` with all six blocks *absent*,
// and `briefing.open_agenda_items.length` threw, taking the whole page into the
// ErrorBoundary. Every freshly-created contact was in that state, so the prep
// view was broken on first use while both this suite and the Go suite stayed
// green.
//
// The server now always emits `[]` and getContactBriefing normalises anyway.
// This test pins the response shape that actually shipped, so a regression on
// either side is caught here rather than by a user.
test('survives a response that omits every collection block entirely', async () => {
  mockBriefingFetch({
    contact_id: 1,
    uid: 'bare-uid',
    name: 'Bare Minimum',
    kind: 'human',
  });
  renderPage();

  await waitFor(() => expect(screen.getByText('Bare Minimum')).toBeInTheDocument());

  // Rendered the page rather than the ErrorBoundary's failure surface.
  expect(screen.getByText('No interactions recorded yet')).toBeInTheDocument();
  expect(screen.queryByText('Things to bring up')).toBeNull();
  expect(screen.queryByText('Upcoming dates')).toBeNull();
});

test('cadence card shows on-track when overdue_by is zero', async () => {
  mockBriefingFetch({
    ...fullBriefing,
    cadence: {
      policy: { id: 'p1', entity_id: 'alice-uid', target_interval_days: 30 },
      health: {
        has_qualifying_interaction: true,
        last_interaction: '2026-08-01T10:00:00Z',
        next_due: '2026-08-31T00:00:00Z',
        overdue_by: 0,
      },
    },
  });
  renderPage();

  await waitFor(() => expect(screen.getByText('Alice Wonder')).toBeInTheDocument());
  expect(screen.getByText('Relationship health')).toBeInTheDocument();
  expect(screen.getByText('On track')).toBeInTheDocument();
  expect(screen.queryByText(/days overdue/)).toBeNull();
});

test('cadence card shows no-interactions state without a qualifying interaction ever', async () => {
  mockBriefingFetch({
    ...fullBriefing,
    cadence: {
      policy: { id: 'p1', entity_id: 'alice-uid', target_interval_days: 30 },
      health: { has_qualifying_interaction: false, overdue_by: 0 },
    },
  });
  renderPage();

  await waitFor(() => expect(screen.getByText('Alice Wonder')).toBeInTheDocument());
  expect(screen.getByText('Relationship health')).toBeInTheDocument();
  expect(screen.getByText(/No qualifying interactions yet/)).toBeInTheDocument();
});

test('relationship row renders a link chip pointing at the other contact', async () => {
  mockBriefingFetch({
    ...fullBriefing,
    relationships: [
      {
        edge: { id: 'e1', source_id: 'bob-uid', target_id: 'alice-uid', type: 'friend_of' },
        other_party_contact_id: 42,
        other_party_name: 'Bob Marley',
        display_token: 'friend_of',
      },
    ],
  });
  renderPage();

  await waitFor(() => expect(screen.getByText('Alice Wonder')).toBeInTheDocument());
  expect(screen.getByText(/Bob Marley/)).toBeInTheDocument();

  // The "View" chip is an <a> wrapping the chip; assert the link resolves
  // to the other party's numeric contact route.
  const viewChip = screen.getByText('View');
  expect(viewChip.closest('a')).toHaveAttribute('href', '/contacts/42');
});

test('shows an error state when the fetch fails', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => ({
      ok: false,
      status: 500,
      statusText: 'Internal Server Error',
      json: async () => ({ error: 'boom' }),
    })),
  );
  renderPage();

  // The error path surfaces via the Snackbar + the inline error alert; assert
  // on the inline alert (any text) rather than a specific message, since
  // handleFetchError/ApiError formatting is a shared utility with its own tests.
  await waitFor(() => {
    const alert = document.querySelector('.MuiAlert-message');
    expect(alert).not.toBeNull();
  });
});
