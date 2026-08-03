import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
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
