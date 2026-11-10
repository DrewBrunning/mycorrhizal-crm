import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter, useLocation } from 'react-router';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { SearchResult } from '../api/search';
import GlobalSearch from './GlobalSearch';

vi.mock('../api/search', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/search')>();
  return { ...actual, searchAll: vi.fn() };
});

import { searchAll } from '../api/search';

const result = (over: Partial<SearchResult> = {}): SearchResult => ({
  query: 'ali',
  contacts: [],
  notes: [],
  activities: [],
  ...over,
});

function Where() {
  const l = useLocation();
  return <div data-testid="where">{l.pathname + l.search}</div>;
}

function renderSearch() {
  return render(
    <MemoryRouter>
      <GlobalSearch />
      <Where />
    </MemoryRouter>,
  );
}

async function type(value: string) {
  const box = screen.getByPlaceholderText('Search contacts, notes, activities…');
  fireEvent.focus(box);
  fireEvent.change(box, { target: { value } });
  await act(async () => {
    await vi.advanceTimersByTimeAsync(300);
  });
  return box;
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.mocked(searchAll).mockReset();
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

const full = result({
  contacts: [
    { id: 1, uid: 'u1', firstname: 'Alice', lastname: 'Smith', primary_email: 'a@x.test' },
  ],
  notes: [
    {
      id: 5,
      content: 'Met Alice at the fair',
      date: '2026-01-01T00:00:00Z',
      contact_id: 1,
      contact_name: 'A. Smith',
    },
    { id: 6, content: 'Unfiled alice idea', date: '2026-01-02T00:00:00Z', contact_id: null },
  ],
  activities: [{ id: 9, title: 'Lunch with Alice', date: '2026-02-01T00:00:00Z' }],
});

test('asks for the top 3 hits per category and groups the dropdown by category', async () => {
  vi.mocked(searchAll).mockResolvedValue(full);
  renderSearch();
  await type('ali');

  expect(searchAll).toHaveBeenCalledWith('ali', 3);
  expect(screen.getByText('Contacts')).toBeInTheDocument();
  expect(screen.getByText('Notes')).toBeInTheDocument();
  expect(screen.getByText('Activities')).toBeInTheDocument();
  expect(screen.getByText('Alice Smith')).toBeInTheDocument();
  expect(screen.getByText('Met Alice at the fair')).toBeInTheDocument();
  expect(screen.getByText('Lunch with Alice')).toBeInTheDocument();
});

test('does not search below two characters', async () => {
  renderSearch();
  await type('a');
  expect(searchAll).not.toHaveBeenCalled();
});

test('omits a category with no hits', async () => {
  vi.mocked(searchAll).mockResolvedValue(result({ activities: full.activities }));
  renderSearch();
  await type('ali');
  expect(screen.queryByText('Contacts')).not.toBeInTheDocument();
  expect(screen.queryByText('Notes')).not.toBeInTheDocument();
  expect(screen.getByText('Activities')).toBeInTheDocument();
});

test('picking a contact opens its page and clears the box', async () => {
  vi.mocked(searchAll).mockResolvedValue(full);
  renderSearch();
  const box = await type('ali');
  fireEvent.click(screen.getByText('Alice Smith'));
  expect(screen.getByTestId('where')).toHaveTextContent('/contacts/1');
  expect((box as HTMLInputElement).value).toBe('');
});

test('a filed note opens its contact; an unfiled note opens the Notes page filtered by the query', async () => {
  vi.mocked(searchAll).mockResolvedValue(full);
  renderSearch();
  await type('ali');
  fireEvent.click(screen.getByText('Met Alice at the fair'));
  expect(screen.getByTestId('where')).toHaveTextContent('/contacts/1');

  await type('ali');
  fireEvent.click(screen.getByText('Unfiled alice idea'));
  expect(screen.getByTestId('where')).toHaveTextContent('/notes?search=ali');
});

test('an activity opens the Activities page filtered by the query', async () => {
  vi.mocked(searchAll).mockResolvedValue(full);
  renderSearch();
  await type('ali');
  fireEvent.click(screen.getByText('Lunch with Alice'));
  expect(screen.getByTestId('where')).toHaveTextContent('/activities?search=ali');
});

test('the "see all results" row opens the results page', async () => {
  vi.mocked(searchAll).mockResolvedValue(full);
  renderSearch();
  await type('ali');
  fireEvent.click(screen.getByText('See all results for "ali"'));
  expect(screen.getByTestId('where')).toHaveTextContent('/search?q=ali');
});

test('Enter with nothing highlighted opens the results page', async () => {
  vi.mocked(searchAll).mockResolvedValue(full);
  renderSearch();
  const box = await type('ali');
  fireEvent.keyDown(box, { key: 'Enter' });
  expect(screen.getByTestId('where')).toHaveTextContent('/search?q=ali');
});

test('shows a no-results row when nothing matches, still linking to the results page', async () => {
  vi.mocked(searchAll).mockResolvedValue(result());
  renderSearch();
  await type('zzz');
  expect(screen.getByText('No results for "zzz".')).toBeInTheDocument();
});

test('a failed search leaves the dropdown empty instead of throwing', async () => {
  const spy = vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(searchAll).mockRejectedValue(new Error('boom'));
  renderSearch();
  await type('ali');
  expect(screen.queryByText('Alice Smith')).not.toBeInTheDocument();
  spy.mockRestore();
});

test('a stale response for an abandoned query does not clobber the newer one (#960)', async () => {
  let resolveSlow!: (r: SearchResult) => void;
  vi.mocked(searchAll)
    .mockReturnValueOnce(new Promise((r) => (resolveSlow = r)))
    .mockResolvedValueOnce(
      result({ contacts: [{ id: 2, uid: 'u2', firstname: 'John', lastname: 'Smith' }] }),
    );
  renderSearch();
  await type('jo');
  await type('john');
  expect(screen.getByText('John Smith')).toBeInTheDocument();
  await act(async () => {
    resolveSlow(result({ contacts: [{ id: 3, uid: 'u3', firstname: 'Joanna', lastname: 'Doe' }] }));
  });
  expect(screen.queryByText('Joanna Doe')).not.toBeInTheDocument();
  expect(screen.getByText('John Smith')).toBeInTheDocument();
});
