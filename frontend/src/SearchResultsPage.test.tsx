import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, useNavigate } from 'react-router';
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
  entities: [
    { id: 'e-1', kind: 'place', name: 'Alice Springs House', sensitivity: 'normal' },
    { id: 'e-2', kind: 'pet', name: 'Alice', sensitivity: 'private' },
  ],
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

test('entities are the fourth tab: counted, linked to the entity page, with a kind chip', async () => {
  vi.mocked(searchAll).mockResolvedValue(data);
  renderPage('/search?q=ali');
  fireEvent.click(await screen.findByRole('tab', { name: 'Organisations, places & pets (2)' }));
  const house = screen.getByRole('link', { name: /Alice Springs House/ });
  expect(house).toHaveAttribute('href', '/entities/e-1');
  expect(house).toHaveTextContent('Place');
  expect(screen.getByRole('link', { name: /Pet/ })).toHaveAttribute('href', '/entities/e-2');
  expect(screen.getByText('Private')).toBeInTheDocument();
});

test('?tab=entities deep-links to the entities tab', async () => {
  vi.mocked(searchAll).mockResolvedValue(data);
  renderPage('/search?q=ali&tab=entities');
  expect(await screen.findByText('Alice Springs House')).toBeInTheDocument();
  expect(screen.getByRole('tab', { name: 'Organisations, places & pets (2)' })).toHaveAttribute(
    'aria-selected',
    'true',
  );
});

test('an empty entities tab explains itself, and a full one says it is capped', async () => {
  vi.mocked(searchAll).mockResolvedValue({ ...data, entities: [] });
  renderPage('/search?q=ali&tab=entities');
  expect(
    await screen.findByText('No organisations, places or pets match "ali".'),
  ).toBeInTheDocument();
  cleanup();
  const many = Array.from({ length: 50 }, (_, i) => ({
    id: `e${i}`,
    kind: 'organisation' as const,
    name: `Org ${i}`,
    sensitivity: 'normal' as const,
  }));
  vi.mocked(searchAll).mockResolvedValue({ ...data, entities: many });
  renderPage('/search?q=org&tab=entities');
  expect(await screen.findByText(/Showing the first 50 matches/)).toBeInTheDocument();
});

function Navigator({ to }: { to: string }) {
  const navigate = useNavigate();
  return (
    <button type="button" onClick={() => void navigate(to)}>
      go to {to}
    </button>
  );
}

// issue #2570: the requestRef guard. A slow response for an abandoned query
// must not overwrite the newer query's results (mirrors GlobalSearch #960).
test('a stale response for an abandoned query does not clobber the newer one', async () => {
  let resolveSlow!: (r: SearchResult) => void;
  vi.mocked(searchAll)
    .mockReturnValueOnce(new Promise<SearchResult>((r) => (resolveSlow = r)))
    .mockResolvedValueOnce({
      ...data,
      query: 'john',
      contacts: [{ id: 2, uid: 'u2', firstname: 'John', lastname: 'Smith' }],
      notes: [],
      entities: [],
    });
  render(
    <MemoryRouter initialEntries={['/search?q=jo']}>
      <DateFormatProvider>
        <Navigator to="/search?q=john" />
        <SearchResultsPage />
      </DateFormatProvider>
    </MemoryRouter>,
  );
  fireEvent.click(screen.getByRole('button', { name: 'go to /search?q=john' }));
  expect(await screen.findByRole('link', { name: /John Smith/ })).toBeInTheDocument();

  await act(async () => {
    resolveSlow({
      ...data,
      query: 'jo',
      contacts: [{ id: 3, uid: 'u3', firstname: 'Joanna', lastname: 'Doe' }],
      notes: [],
      entities: [],
    });
  });
  expect(screen.queryByRole('link', { name: /Joanna Doe/ })).not.toBeInTheDocument();
  expect(screen.getByRole('link', { name: /John Smith/ })).toBeInTheDocument();
});

test('a stale failure for an abandoned query does not replace the newer results with an error', async () => {
  let rejectSlow!: (e: Error) => void;
  vi.mocked(searchAll)
    .mockReturnValueOnce(new Promise<SearchResult>((_, rej) => (rejectSlow = rej)))
    .mockResolvedValueOnce({ ...data, query: 'john', notes: [], entities: [] });
  render(
    <MemoryRouter initialEntries={['/search?q=jo']}>
      <DateFormatProvider>
        <Navigator to="/search?q=john" />
        <SearchResultsPage />
      </DateFormatProvider>
    </MemoryRouter>,
  );
  fireEvent.click(screen.getByRole('button', { name: 'go to /search?q=john' }));
  await screen.findByRole('tab', { name: 'Contacts (1)' });
  await act(async () => {
    rejectSlow(new Error('slow boom'));
  });
  expect(screen.queryByText(/slow boom/)).not.toBeInTheDocument();
  expect(screen.getByRole('tab', { name: 'Contacts (1)' })).toBeInTheDocument();
});

// #2698: the default tab must not be an empty one when another has hits.
test('with no tab parameter the first non-empty tab is selected (#2698)', async () => {
  vi.mocked(searchAll).mockResolvedValue({ ...data, contacts: [], notes: [], activities: [] });
  renderPage('/search?q=lakeside');
  expect(await screen.findByText('Alice Springs House')).toBeInTheDocument();
  expect(screen.getByRole('tab', { name: 'Organisations, places & pets (2)' })).toHaveAttribute(
    'aria-selected',
    'true',
  );
  expect(screen.queryByText('No contacts match "lakeside".')).not.toBeInTheDocument();
});

test('an explicit tab parameter is honoured even when that tab is empty (#2698)', async () => {
  vi.mocked(searchAll).mockResolvedValue({ ...data, contacts: [], notes: [], activities: [] });
  renderPage('/search?q=lakeside&tab=contacts');
  expect(await screen.findByText('No contacts match "lakeside".')).toBeInTheDocument();
  expect(screen.getByRole('tab', { name: 'Contacts (0)' })).toHaveAttribute(
    'aria-selected',
    'true',
  );
});

test('the user can pick the empty Contacts tab over a non-empty default (#2698)', async () => {
  vi.mocked(searchAll).mockResolvedValue({ ...data, contacts: [], notes: [], activities: [] });
  renderPage('/search?q=lakeside');
  fireEvent.click(await screen.findByRole('tab', { name: 'Contacts (0)' }));
  expect(await screen.findByText('No contacts match "lakeside".')).toBeInTheDocument();
});

test('when every category is empty the Contacts tab stays selected (#2698)', async () => {
  vi.mocked(searchAll).mockResolvedValue({ ...data, contacts: [], notes: [], entities: [] });
  renderPage('/search?q=zzzz');
  expect(await screen.findByRole('tab', { name: 'Contacts (0)' })).toHaveAttribute(
    'aria-selected',
    'true',
  );
});

// #2692: a phone has no AppBar search, so the page carries its own field.
test('the results page has a search field prefilled with the query; submitting replaces it (#2692)', async () => {
  vi.mocked(searchAll).mockResolvedValue(data);
  renderPage('/search?q=ali&tab=notes');
  const field = await screen.findByRole('searchbox', { name: 'Search everything' });
  expect(field).toHaveValue('ali');
  fireEvent.change(field, { target: { value: 'bob' } });
  fireEvent.submit(field.closest('form') as HTMLFormElement);
  await waitFor(() => expect(searchAll).toHaveBeenLastCalledWith('bob', 50));
  expect(screen.getByRole('searchbox', { name: 'Search everything' })).toHaveValue('bob');
});

test('the search field is reachable when no query has been typed yet (#2692)', () => {
  renderPage('/search');
  expect(screen.getByRole('searchbox', { name: 'Search everything' })).toHaveValue('');
});

// axe `list` (serious): a <ul> may only contain <li>; each result link sits in one.
test('every result row is a list item wrapping its link', async () => {
  vi.mocked(searchAll).mockResolvedValue(data);
  renderPage('/search?q=ali');
  const link = await screen.findByRole('link', { name: /Alice Smith/ });
  const list = link.closest('ul') as HTMLElement;
  for (const child of Array.from(list.children)) expect(child.tagName).toBe('LI');
});

// #2744: the results page says why a contact matched when it was not (only) the name.
test('a contact row shows why it matched; name-only and absent show nothing (#2744)', async () => {
  vi.mocked(searchAll).mockResolvedValue({
    ...data,
    contacts: [
      {
        id: 1,
        uid: 'u1',
        firstname: 'Beatrix',
        lastname: 'Zebra',
        matched_on: ['email', 'address'],
      },
      { id: 2, uid: 'u2', firstname: 'Bob', lastname: 'Name', matched_on: ['name'] },
      { id: 3, uid: 'u3', firstname: 'Cy', lastname: 'Plain' },
    ],
  });
  renderPage('/search?q=zebra');
  const row = await screen.findByRole('link', { name: /Beatrix Zebra/ });
  expect(row).toHaveTextContent('Matches: email, address');
  expect(screen.getAllByText(/^Matches: /)).toHaveLength(1);
});

test('entity rows never show a match reason (name is the only one) (#2744)', async () => {
  vi.mocked(searchAll).mockResolvedValue({
    ...data,
    entities: [
      {
        id: 'e-1',
        kind: 'place',
        name: 'Alice Springs House',
        sensitivity: 'normal',
        matched_on: ['name'],
      },
    ],
  });
  renderPage('/search?q=ali&tab=entities');
  await screen.findByRole('link', { name: /Alice Springs House/ });
  expect(screen.queryByText(/^Matches: /)).toBeNull();
});
