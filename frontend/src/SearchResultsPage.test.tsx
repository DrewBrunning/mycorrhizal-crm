import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import './i18n/config';
import type { SearchResult } from './api/search';
import { DateFormatProvider } from './DateFormatProvider';
import SearchResultsPage from './SearchResultsPage';

vi.mock('./api/search', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/search')>();
  return { ...actual, searchAll: vi.fn() };
});

import { searchAll } from './api/search';

const data: SearchResult = {
  query: 'ali',
  contacts: [{ id: 1, uid: 'u1', firstname: 'Alice', lastname: 'Smith', org: 'Acme' }],
  notes: [
    {
      id: 5,
      content: 'Met Alice at the fair',
      date: '2026-01-01T00:00:00Z',
      contact_id: 1,
      contact_name: 'Alice Smith',
    },
    { id: 6, content: 'Unfiled alice idea', date: '2026-01-02T00:00:00Z', contact_id: null },
  ],
  activities: [],
};

function renderPage(url: string) {
  return render(
    <MemoryRouter initialEntries={[url]}>
      <DateFormatProvider>
        <SearchResultsPage />
      </DateFormatProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => vi.mocked(searchAll).mockReset());
afterEach(cleanup);

test('asks for the maximum page per category and shows one tab per category with counts', async () => {
  vi.mocked(searchAll).mockResolvedValue(data);
  renderPage('/search?q=ali');
  await screen.findByRole('tab', { name: 'Contacts (1)' });
  expect(searchAll).toHaveBeenCalledWith('ali', 50);
  expect(screen.getByRole('tab', { name: 'Notes (2)' })).toBeInTheDocument();
  expect(screen.getByRole('tab', { name: 'Activities (0)' })).toBeInTheDocument();
  expect(screen.getByRole('heading', { level: 1, name: 'Search results' })).toBeInTheDocument();
  // Contacts is the default tab.
  expect(screen.getByRole('link', { name: /Alice Smith/ })).toHaveAttribute('href', '/contacts/1');
  expect(document.title).toBe('Search results · Mycorrhizal CRM');
});

test('switching tabs shows that category, with links to where each hit lives', async () => {
  vi.mocked(searchAll).mockResolvedValue(data);
  renderPage('/search?q=ali');
  fireEvent.click(await screen.findByRole('tab', { name: 'Notes (2)' }));
  expect(screen.getByRole('link', { name: /Met Alice at the fair/ })).toHaveAttribute(
    'href',
    '/contacts/1',
  );
  expect(screen.getByRole('link', { name: /Unfiled alice idea/ })).toHaveAttribute(
    'href',
    '/notes?search=ali',
  );
});

test('the tab query parameter selects the initial tab', async () => {
  vi.mocked(searchAll).mockResolvedValue(data);
  renderPage('/search?q=ali&tab=notes');
  expect(await screen.findByText('Met Alice at the fair')).toBeInTheDocument();
  expect(screen.getByRole('tab', { name: 'Notes (2)' })).toHaveAttribute('aria-selected', 'true');
});

test('an empty category explains itself', async () => {
  vi.mocked(searchAll).mockResolvedValue(data);
  renderPage('/search?q=ali&tab=activities');
  expect(await screen.findByText('No activities match "ali".')).toBeInTheDocument();
});

test('a missing or one-character query prompts instead of searching', () => {
  renderPage('/search');
  expect(screen.getByText('Type at least two characters to search.')).toBeInTheDocument();
  expect(searchAll).not.toHaveBeenCalled();
});

test('a failed search shows a retryable error', async () => {
  vi.mocked(searchAll).mockRejectedValueOnce(new Error('boom')).mockResolvedValueOnce(data);
  renderPage('/search?q=ali');
  fireEvent.click(await screen.findByRole('button', { name: /retry/i }));
  await waitFor(() =>
    expect(screen.getByRole('tab', { name: 'Contacts (1)' })).toBeInTheDocument(),
  );
});
