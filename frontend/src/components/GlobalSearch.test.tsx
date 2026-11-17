import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react';
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
  entities: [],
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
  const box = screen.getByPlaceholderText('Search contacts, notes, activities, places…');
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
  entities: [
    { id: 'e-1', kind: 'organisation', name: 'Alice Holdings', sensitivity: 'normal' },
    { id: 'e-2', kind: 'pet', name: 'Alice the cat', sensitivity: 'secret' },
  ],
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
  expect(
    within(screen.getByRole('listbox')).getByText('No results for "zzz".'),
  ).toBeInTheDocument();
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

test('entities get their own group with the kind (and a non-normal sensitivity) as the detail line', async () => {
  vi.mocked(searchAll).mockResolvedValue(full);
  renderSearch();
  await type('ali');
  expect(screen.getByText('Organisations, places & pets')).toBeInTheDocument();
  expect(screen.getByText('Alice Holdings')).toBeInTheDocument();
  expect(screen.getByText('Organisation')).toBeInTheDocument();
  expect(screen.getByText('Alice the cat')).toBeInTheDocument();
  expect(screen.getByText(/^Pet · /)).toBeInTheDocument();
});

test('picking an entity opens its page and clears the box', async () => {
  vi.mocked(searchAll).mockResolvedValue(full);
  renderSearch();
  const box = await type('ali');
  fireEvent.click(screen.getByText('Alice Holdings'));
  expect(screen.getByTestId('where')).toHaveTextContent('/entities/e-1');
  expect((box as HTMLInputElement).value).toBe('');
});

test('an entities-only result is a result: no "no results" row, and see-all is offered', async () => {
  vi.mocked(searchAll).mockResolvedValue(result({ entities: full.entities }));
  renderSearch();
  await type('ali');
  expect(screen.getByText('Organisations, places & pets')).toBeInTheDocument();
  expect(screen.queryByText(/No results for/)).not.toBeInTheDocument();
  expect(screen.getByText('See all results for "ali"')).toBeInTheDocument();
});

// #2695: valid combobox/listbox ARIA with a named input and announced counts.
test('the input is a named combobox (#2695)', () => {
  renderSearch();
  expect(screen.getByRole('combobox', { name: 'Search everything' })).toBeInTheDocument();
});

test('the open dropdown is a valid listbox: presentation wrappers, named groups, options inside groups (#2695)', async () => {
  vi.mocked(searchAll).mockResolvedValue(full);
  renderSearch();
  await type('ali');
  const listbox = screen.getByRole('listbox', { name: 'Search everything' });
  expect(listbox).not.toHaveAttribute('aria-labelledby');
  // aria-required-children: a listbox may only own options and groups.
  for (const child of Array.from(listbox.children)) {
    expect(child.getAttribute('role')).toBe('presentation');
  }
  for (const name of ['Contacts', 'Notes', 'Activities', 'Entities']) {
    expect(within(listbox).getByRole('group', { name })).toBeInTheDocument();
  }
  // aria-required-parent: every option sits in a group (or the listbox itself).
  for (const option of within(listbox).getAllByRole('option')) {
    expect(option.parentElement?.getAttribute('role')).toBe('group');
  }
  // No bare <ul> left between the wrappers and the options.
  expect(listbox.querySelectorAll('ul:not([role])')).toHaveLength(0);
});

test('announces the number of results after each response (#2695)', async () => {
  vi.mocked(searchAll).mockResolvedValue(full);
  renderSearch();
  await type('ali');
  expect(screen.getByText('6 results').closest('[role="status"]')).toBeInTheDocument();
  vi.mocked(searchAll).mockResolvedValue(result());
  await type('zzz');
  // The live region and the "no results" row both carry the text.
  const none = screen.getAllByText('No results for "zzz".');
  expect(none.some((el) => el.closest('[role="status"]'))).toBe(true);
});

// #2697: Escape leaves a stale highlight that made Enter a no-op.
test('Enter submits again after a highlight was dismissed with Escape (#2697)', async () => {
  vi.mocked(searchAll).mockResolvedValue(full);
  renderSearch();
  const box = await type('gre');
  fireEvent.keyDown(box, { key: 'ArrowDown' });
  fireEvent.keyDown(box, { key: 'Escape' });
  await type('');
  await type('zzqqxx');
  fireEvent.keyDown(box, { key: 'Enter' });
  expect(screen.getByTestId('where')).toHaveTextContent('/search?q=zzqqxx');
});

// #2710: a failed request used to leave an empty, silent popup.
test('a failed search shows an error row that opens the results page (#2710)', async () => {
  const spy = vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(searchAll).mockRejectedValue(new Error('boom'));
  renderSearch();
  await type('ali');
  const row = within(screen.getByRole('listbox')).getByText(
    "Couldn't search right now. Press Enter to open the results page.",
  );
  fireEvent.click(row);
  expect(screen.getByTestId('where')).toHaveTextContent('/search?q=ali');
  spy.mockRestore();
});

test('falls back to the formatted name and the description for sparse hits', async () => {
  vi.mocked(searchAll).mockResolvedValue(
    result({
      contacts: [{ id: 4, uid: 'u4', firstname: '', lastname: '', fn: 'Dr Who' }],
      activities: [{ id: 8, title: '', description: 'Quiet walk', date: '2026-02-01T00:00:00Z' }],
    }),
  );
  renderSearch();
  await type('wh');
  expect(screen.getByText('Dr Who')).toBeInTheDocument();
  expect(screen.getByText('Quiet walk')).toBeInTheDocument();
});

test('a failure for an abandoned query does not replace the newer results (#960)', async () => {
  const spy = vi.spyOn(console, 'error').mockImplementation(() => {});
  let rejectSlow!: (e: Error) => void;
  vi.mocked(searchAll)
    .mockReturnValueOnce(new Promise((_, rej) => (rejectSlow = rej)))
    .mockResolvedValueOnce(
      result({ contacts: [{ id: 2, uid: 'u2', firstname: 'John', lastname: 'Smith' }] }),
    );
  renderSearch();
  await type('jo');
  await type('john');
  await act(async () => {
    rejectSlow(new Error('late'));
  });
  expect(screen.getByText('John Smith')).toBeInTheDocument();
  expect(screen.queryByText(/Couldn't search right now/)).not.toBeInTheDocument();
  spy.mockRestore();
});

test('Enter on a blank box does nothing', async () => {
  renderSearch();
  const box = screen.getByRole('combobox', { name: 'Search everything' });
  fireEvent.change(box, { target: { value: '   ' } });
  fireEvent.keyDown(box, { key: 'Enter' });
  expect(screen.getByTestId('where')).toHaveTextContent(/^\/$/);
});
