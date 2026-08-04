import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter, Routes, Route, useNavigate } from 'react-router-dom';
import './i18n/config';
import SearchPage from './SearchPage';
import { DateFormatProvider } from './DateFormatProvider';

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const searchResponse = () => ({
  query: 'symphony',
  resolved_relation: '',
  contacts: [
    { id: 7, uid: 'c-7', firstname: 'Wolfgang', lastname: 'Symphony', primary_email: 'w@example.com' },
  ],
  notes: [
    { id: 3, content: 'review the symphony score', date: '2026-08-03T10:00:00Z', contact_id: 7, contact_name: 'Wolfgang Symphony', snippet: '…symphony…' },
  ],
  activities: [
    { id: 9, title: 'Symphony concert', date: '2026-08-01T19:00:00Z', snippet: '…symphony…' },
  ],
});

function mockSearch(respond: () => unknown) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      if (url.includes('/search?')) {
        return { ok: true, json: async () => respond() };
      }
      throw new Error(`unexpected fetch: ${url}`);
    })
  );
}

function renderSearchPage(initial = '') {
  return render(
    <DateFormatProvider>
      <MemoryRouter initialEntries={[`/search${initial ? `?q=${initial}` : ''}`]}>
        <Routes>
          <Route path="/search" element={<SearchPage />} />
          <Route path="/contacts/:id" element={<div>CONTACT PAGE</div>} />
        </Routes>
      </MemoryRouter>
    </DateFormatProvider>
  );
}

test('searches and surfaces all three groups', async () => {
  mockSearch(searchResponse);
  renderSearchPage('symphony');

  await waitFor(() => {
    expect(screen.getAllByText('Wolfgang Symphony').length).toBeGreaterThanOrEqual(1);
  });
  expect(screen.getByText('review the symphony score')).toBeInTheDocument();
  expect(screen.getByText('Symphony concert')).toBeInTheDocument();
  expect(screen.getByText('Contacts (1)')).toBeInTheDocument();
  expect(screen.getByText('Notes (1)')).toBeInTheDocument();
  expect(screen.getByText('Interactions (1)')).toBeInTheDocument();
});

test('shows no-results when everything is empty', async () => {
  mockSearch(() => ({ query: 'zzz', resolved_relation: '', contacts: [], notes: [], activities: [] }));
  renderSearchPage('zzz');

  await waitFor(() => {
    expect(screen.getByText(/No results for "zzz"/)).toBeInTheDocument();
  });
});

test('typing a query triggers a search and updates the URL', async () => {
  mockSearch(searchResponse);
  renderSearchPage();

  fireEvent.change(screen.getByLabelText(/Search contacts, notes, interactions/), {
    target: { value: 'symphony' },
  });
  await waitFor(() => {
    expect(screen.getAllByText('Wolfgang Symphony').length).toBeGreaterThanOrEqual(1);
  });
});

test('clicking a contact navigates to its page', async () => {
  mockSearch(searchResponse);
  renderSearchPage('symphony');

  await waitFor(() => {
    expect(screen.getAllByText('Wolfgang Symphony').length).toBeGreaterThanOrEqual(1);
  });
  fireEvent.click(screen.getAllByText('Wolfgang Symphony')[0]);
  await waitFor(() => expect(screen.getByText('CONTACT PAGE')).toBeInTheDocument());
});

// Regression test: the top-nav search bar submits a new query by navigating
// to /search?q=X. React Router does not remount SearchPage for a same-route
// param change, so this only exercises the real bug (input state seeded
// from the URL once at mount, never resynced) if the URL changes while the
// page is already mounted — a plain `renderSearchPage(initial)` call can't
// reach that path. Confirmed live in a real browser before the fix: it left
// the old query's results on screen and reverted the URL back to it.
function ExternalNavButton({ to }: { to: string }) {
  const navigate = useNavigate();
  return <button onClick={() => navigate(to)}>external-nav</button>;
}

test('a query submitted externally (e.g. the top-nav search bar) while already on the page replaces the old search', async () => {
  const responses: Record<string, unknown> = {
    Alice: { query: 'Alice', resolved_relation: '', contacts: [{ id: 1, uid: 'u1', firstname: 'Alice', lastname: 'Johnson' }], notes: [], activities: [] },
    Bob: { query: 'Bob', resolved_relation: '', contacts: [{ id: 2, uid: 'u2', firstname: 'Bob', lastname: 'Smith' }], notes: [], activities: [] },
  };
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      const q = new URL(url, 'http://x').searchParams.get('q') || '';
      return { ok: true, json: async () => responses[q] ?? { query: q, resolved_relation: '', contacts: [], notes: [], activities: [] } };
    })
  );

  render(
    <DateFormatProvider>
      <MemoryRouter initialEntries={['/search?q=Alice']}>
        <ExternalNavButton to="/search?q=Bob" />
        <Routes>
          <Route path="/search" element={<SearchPage />} />
        </Routes>
      </MemoryRouter>
    </DateFormatProvider>
  );

  await waitFor(() => expect(screen.getByText('Alice Johnson')).toBeInTheDocument());

  fireEvent.click(screen.getByText('external-nav'));

  await waitFor(() => expect(screen.getByText('Bob Smith')).toBeInTheDocument());
  expect(screen.queryByText('Alice Johnson')).not.toBeInTheDocument();
});
