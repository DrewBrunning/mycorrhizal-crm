import { test, expect, vi, afterEach, beforeEach } from 'vitest';
import { render, screen, cleanup, fireEvent, waitFor, within } from '@testing-library/react';
import { MemoryRouter, Routes, Route } from 'react-router';
import './i18n/config';
import ContactsPage from './ContactsPage';
import { SnackbarProvider } from './context/SnackbarContext';
import { DateFormatProvider } from './DateFormatProvider';
import { getContacts, Contact } from './api/contacts';
import { listCircles } from './api/circles';
import { listTags } from './api/tags';
import { getFieldDefinitions } from './api/fieldDefinitions';
import { getCurrentUser } from './api/admin';
import { runBulkOperation, BulkOperationResult } from './api/bulkOperations';
import { searchAll } from './api/search';
import { previewContactMerge, commitContactMerge } from './api/contactMerge';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(cleanup);

vi.mock('./api/contacts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/contacts')>();
  return { ...actual, getContacts: vi.fn() };
});
vi.mock('./api/circles', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/circles')>();
  return { ...actual, listCircles: vi.fn() };
});
vi.mock('./api/tags', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/tags')>();
  return { ...actual, listTags: vi.fn() };
});
vi.mock('./api/fieldDefinitions', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/fieldDefinitions')>();
  return { ...actual, getFieldDefinitions: vi.fn() };
});
vi.mock('./api/admin', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/admin')>();
  return { ...actual, getCurrentUser: vi.fn() };
});
vi.mock('./api/bulkOperations', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/bulkOperations')>();
  return { ...actual, runBulkOperation: vi.fn() };
});
vi.mock('./api/search', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/search')>();
  return {
    ...actual,
    searchAll: vi.fn(async (q: string) => ({
      query: q,
      resolved_relation: '',
      contacts: [],
      notes: [],
      activities: [],
    })),
  };
});
vi.mock('./api/contactMerge', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/contactMerge')>();
  return { ...actual, previewContactMerge: vi.fn(), commitContactMerge: vi.fn() };
});
vi.mock('./auth', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./auth')>();
  return { ...actual, isAuthenticated: vi.fn(() => true) };
});

beforeEach(() => {
  vi.mocked(getContacts).mockReset();
  vi.mocked(listCircles).mockReset();
  vi.mocked(listTags).mockReset();
  vi.mocked(getFieldDefinitions).mockReset();
  vi.mocked(getCurrentUser).mockReset();
  vi.mocked(runBulkOperation).mockReset();
  vi.mocked(searchAll).mockReset();
  vi.mocked(previewContactMerge).mockReset();
  vi.mocked(commitContactMerge).mockReset();

  vi.mocked(getCurrentUser).mockResolvedValue({ enabled_contact_fields: null } as never);
  vi.mocked(listCircles).mockResolvedValue({ circles: [], members: [], next_cursor: '', limit: 100 } as never);
  vi.mocked(listTags).mockResolvedValue({ tags: [], contacts: [], next_cursor: '', limit: 100 } as never);
  vi.mocked(getFieldDefinitions).mockResolvedValue({ field_definitions: [] } as never);
  vi.mocked(searchAll).mockResolvedValue({ query: '', resolved_relation: '', contacts: [], notes: [], activities: [] } as never);
});

function contact(id: number, uid: string, firstname: string): Contact {
  return { ID: id, uid, firstname, lastname: '', archived: false };
}

// Two pages: page one has Alice + Bob, page two (loaded via "load more") has
// Carol. Selection must survive the pagination boundary.
function mockTwoPages() {
  vi.mocked(getContacts).mockImplementation((params) => {
    if (params?.cursor) {
      return Promise.resolve({ contacts: [contact(3, 'uid-3', 'Carol')], next_cursor: '', limit: 10 });
    }
    return Promise.resolve({ contacts: [contact(1, 'uid-1', 'Alice'), contact(2, 'uid-2', 'Bob')], next_cursor: 'cursor-1', limit: 10 });
  });
}

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/contacts']}>
      <DateFormatProvider>
        <SnackbarProvider>
          <Routes>
            <Route path="/contacts" element={<ContactsPage />} />
            <Route path="/contacts/:id" element={<div>CONTACT DETAIL PAGE</div>} />
          </Routes>
        </SnackbarProvider>
      </DateFormatProvider>
    </MemoryRouter>
  );
}

test('selection survives pagination and selects across both pages', async () => {
  mockTwoPages();
  renderPage();

  // Page one loads: Alice and Bob.
  const aliceBox = await screen.findByLabelText('Select Alice');
  expect(screen.getByLabelText('Select Bob')).toBeInTheDocument();

  // Select Alice.
  fireEvent.click(aliceBox);
  expect(screen.getByText('1 selected')).toBeInTheDocument();

  // Load page two; the selection must survive the boundary.
  fireEvent.click(screen.getByText('Load more'));
  const carolBox = await screen.findByLabelText('Select Carol');
  expect(screen.getByText('1 selected')).toBeInTheDocument();
  expect(carolBox).not.toBeChecked();
  expect(aliceBox).toBeChecked();

  // Select Carol across the page boundary → 2 selected.
  fireEvent.click(carolBox);
  expect(screen.getByText('2 selected')).toBeInTheDocument();
});

test('select-all selects every loaded contact and clear empties the selection', async () => {
  mockTwoPages();
  renderPage();
  await screen.findByLabelText('Select Alice');

  fireEvent.click(screen.getByLabelText('Select all'));
  expect(screen.getByText('2 selected')).toBeInTheDocument();

  fireEvent.click(screen.getByText('Load more'));
  await screen.findByLabelText('Select Carol');

  // Select-all adds the newly loaded page too.
  fireEvent.click(screen.getByLabelText('Select all'));
  expect(screen.getByText('3 selected')).toBeInTheDocument();

  fireEvent.click(screen.getByText('Clear'));
  expect(screen.queryByText('3 selected')).not.toBeInTheDocument();
});

test('a bulk tag add sends the selected VCardUIDs and nothing else', async () => {
  mockTwoPages();
  vi.mocked(runBulkOperation).mockResolvedValue({
    action: 'add_tag',
    total: 1,
    succeeded: 1,
    failed: 0,
    failures: [],
  } as BulkOperationResult);
  vi.mocked(listTags).mockResolvedValue({
    tags: [{ id: 't1', created_at: '', updated_at: '', name: 'vip' }],
    contacts: [],
    next_cursor: '',
    limit: 100,
  } as never);

  renderPage();
  await screen.findByLabelText('Select Alice');

  fireEvent.click(screen.getByLabelText('Select Alice'));
  fireEvent.mouseDown(screen.getByLabelText('Tag…'));
  fireEvent.click(await screen.findByRole('option', { name: 'vip' }));
  fireEvent.click(screen.getByText('Add tag'));

  await waitFor(() =>
    expect(runBulkOperation).toHaveBeenCalledWith({
      action: 'add_tag',
      vcard_uids: ['uid-1'],
      tag_id: 't1',
    })
  );
});

test('bulk delete asks for confirmation naming the count before running', async () => {
  mockTwoPages();
  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false);
  renderPage();
  await screen.findByLabelText('Select Alice');

  fireEvent.click(screen.getByLabelText('Select Alice'));
  fireEvent.click(screen.getByText('Delete'));

  expect(confirmSpy).toHaveBeenCalledWith('Delete 1 contacts? This permanently removes them and all of their data. This cannot be undone.');
  expect(runBulkOperation).not.toHaveBeenCalled();
});

// Regression test: the row Checkbox sits inside a Card whose own onClick
// navigates to the contact's detail page. stopPropagation() on the
// Checkbox's onChange does nothing for the native click event that bubbles
// to the Card, so clicking the checkbox used to navigate away instead of
// selecting — confirmed in a real browser, not just here.
test("clicking a contact's checkbox selects it without navigating to its detail page", async () => {
  mockTwoPages();
  renderPage();
  const aliceBox = await screen.findByLabelText('Select Alice');

  fireEvent.click(aliceBox);

  expect(screen.getByText('1 selected')).toBeInTheDocument();
  expect(aliceBox).toBeChecked();
  expect(screen.queryByText('CONTACT DETAIL PAGE')).not.toBeInTheDocument();
});

test('clicking the contact row itself (not the checkbox) still navigates to its detail page', async () => {
  mockTwoPages();
  renderPage();
  await screen.findByLabelText('Select Alice');

  fireEvent.click(screen.getByText('Alice'));

  expect(await screen.findByText('CONTACT DETAIL PAGE')).toBeInTheDocument();
});

test('changing the circle filter clears an in-progress selection', async () => {
  mockTwoPages();
  vi.mocked(listCircles).mockResolvedValue({
    circles: [{ id: 'c1', created_at: '', updated_at: '', name: 'Friends' }],
    members: [],
    next_cursor: '',
    limit: 100,
  } as never);

  renderPage();
  const aliceBox = await screen.findByLabelText('Select Alice');
  fireEvent.click(aliceBox);
  expect(screen.getByText('1 selected')).toBeInTheDocument();

  // The filter swaps the visible contacts out from under the selection —
  // a stale "N selected" would let a bulk action (including delete) run
  // against contacts no longer on screen.
  fireEvent.mouseDown(screen.getByLabelText('Filter by Circle'));
  fireEvent.click(await screen.findByRole('option', { name: 'Friends' }));

  await waitFor(() => expect(screen.queryByText('1 selected')).not.toBeInTheDocument());
});

test('typing a search term filters the list through the debounced URL param', async () => {
  mockTwoPages();
  renderPage();
  await screen.findByLabelText('Select Alice');

  fireEvent.change(screen.getByLabelText(/search contacts/i), {
    target: { value: 'ali' },
  });

  await waitFor(() =>
    expect(getContacts).toHaveBeenCalledWith(
      expect.objectContaining({ search: 'ali' })
    )
  );
});

test('a single character does not trigger a filtered search', async () => {
  mockTwoPages();
  renderPage();
  await screen.findByLabelText('Select Alice');

  fireEvent.change(screen.getByLabelText(/search contacts/i), {
    target: { value: 'a' },
  });

  // The two-character minimum means the single rune never becomes the URL
  // param, so the list keeps its unfiltered query — no refetch, no search arg.
  await new Promise((r) => setTimeout(r, 400));
  expect(getContacts).toHaveBeenCalledTimes(1);
  expect(getContacts).not.toHaveBeenCalledWith(expect.objectContaining({ search: 'a' }));
});

test('clearing the search field restores the unfiltered list', async () => {
  mockTwoPages();
  renderPage();
  await screen.findByLabelText('Select Alice');

  const field = screen.getByLabelText(/search contacts/i);
  fireEvent.change(field, { target: { value: 'ali' } });
  await waitFor(() =>
    expect(getContacts).toHaveBeenCalledWith(
      expect.objectContaining({ search: 'ali' })
    )
  );

  fireEvent.change(field, { target: { value: '' } });

  await waitFor(() =>
    expect(getContacts).toHaveBeenLastCalledWith(
      expect.objectContaining({ search: '' })
    )
  );
});

test('a search change clears an in-progress selection', async () => {
  mockTwoPages();
  renderPage();
  await screen.findByLabelText('Select Alice');

  fireEvent.click(screen.getByLabelText('Select Alice'));
  expect(screen.getByText('1 selected')).toBeInTheDocument();

  fireEvent.change(screen.getByLabelText(/search contacts/i), {
    target: { value: 'bob' },
  });

  await waitFor(() => expect(screen.queryByText('1 selected')).not.toBeInTheDocument());
});

test('defaults to name (alphabetical) sort', async () => {
  mockTwoPages();
  renderPage();
  await screen.findByLabelText('Select Alice');

  expect(getContacts).toHaveBeenCalledWith(
    expect.objectContaining({ sort: 'name', order: 'asc' })
  );
});

test('changing the sort control refetches with the chosen sort and direction', async () => {
  mockTwoPages();
  renderPage();
  await screen.findByLabelText('Select Alice');

  fireEvent.mouseDown(screen.getByLabelText('Sort'));
  fireEvent.click(await screen.findByRole('option', { name: 'Recently edited (newest first)' }));

  await waitFor(() =>
    expect(getContacts).toHaveBeenCalledWith(
      expect.objectContaining({ sort: 'updated_at', order: 'desc' })
    )
  );
});

test('changing the sort does not clear an in-progress selection', async () => {
  mockTwoPages();
  renderPage();
  await screen.findByLabelText('Select Alice');

  fireEvent.click(screen.getByLabelText('Select Alice'));
  expect(screen.getByText('1 selected')).toBeInTheDocument();

  fireEvent.mouseDown(screen.getByLabelText('Sort'));
  fireEvent.click(await screen.findByRole('option', { name: 'Recently edited (newest first)' }));

  await waitFor(() =>
    expect(getContacts).toHaveBeenCalledWith(
      expect.objectContaining({ sort: 'updated_at', order: 'desc' })
    )
  );
  // Sorting reorders the list but not which contacts are visible — the
  // selection is still valid and must survive (T77 trap).
  expect(screen.getByText('1 selected')).toBeInTheDocument();
});

// --- T103 contact-info filter -----------------------------------------------

// T103: the filter defaults ON — a fresh /contacts load asks the server for
// contactable contacts only (hasContactInfo: true), even though the URL has no
// param (absence of ?has_contact_info= means the default, which is filtered).
test('defaults to the contactable-only filter on first load', async () => {
  mockTwoPages();
  renderPage();
  await screen.findByLabelText('Select Alice');

  expect(getContacts).toHaveBeenCalledWith(
    expect.objectContaining({ hasContactInfo: true })
  );
  // The "Show all" switch is present and off (the filter is on).
  expect(screen.getByLabelText('Show all')).not.toBeChecked();
});

// T103: the switch is the URL-persisted inverse of the filter. Flipping it on
// writes has_contact_info=false (show everything) and refetches accordingly.
test('toggling Show all off the filter and writing the URL param', async () => {
  mockTwoPages();
  renderPage();
  await screen.findByLabelText('Select Alice');

  fireEvent.click(screen.getByLabelText('Show all'));

  await waitFor(() =>
    expect(getContacts).toHaveBeenCalledWith(
      expect.objectContaining({ hasContactInfo: false })
    )
  );
  expect(screen.getByLabelText('Show all')).toBeChecked();
});

// T103 URL round trip: a shared/reloaded link carrying has_contact_info=false
// must reproduce the "show all" state — the switch is checked and the server
// is asked for everything.
test('a has_contact_info=false URL is honoured on load', async () => {
  vi.mocked(getContacts).mockImplementation((params) => {
    expect(params?.hasContactInfo).toBe(false);
    return Promise.resolve({ contacts: [contact(1, 'uid-1', 'Alice')], next_cursor: '', limit: 10 });
  });
  render(
    <MemoryRouter initialEntries={['/contacts?has_contact_info=false']}>
      <DateFormatProvider>
        <SnackbarProvider>
          <Routes>
            <Route path="/contacts" element={<ContactsPage />} />
          </Routes>
        </SnackbarProvider>
      </DateFormatProvider>
    </MemoryRouter>
  );
  await screen.findByLabelText('Select Alice');
  expect(screen.getByLabelText('Show all')).toBeChecked();
});

// T103: the hidden count is disclosed when the filter is on and the response
// says contacts were hidden — the user who sees 340 must know the other 160
// were hidden, not lost.
test('renders the hidden count while the filter is active', async () => {
  vi.mocked(getContacts).mockResolvedValue({
    contacts: [contact(1, 'uid-1', 'Alice')],
    next_cursor: '',
    limit: 10,
    hidden_count: 2,
  });
  renderPage();
  await screen.findByLabelText('Select Alice');
  expect(screen.getByText('2 contacts without contact info are hidden')).toBeInTheDocument();

  // Toggling Show all reveals them; the line disappears (no filter, no count).
  fireEvent.click(screen.getByLabelText('Show all'));
  await waitFor(() => expect(screen.queryByText('2 contacts without contact info are hidden')).not.toBeInTheDocument());
});

test('a hidden count of zero renders nothing', async () => {
  vi.mocked(getContacts).mockResolvedValue({
    contacts: [contact(1, 'uid-1', 'Alice')],
    next_cursor: '',
    limit: 10,
    hidden_count: 0,
  });
  renderPage();
  await screen.findByLabelText('Select Alice');
  expect(screen.queryByText(/hidden/i)).not.toBeInTheDocument();
});

// T103 trap: toggling the contact-info filter swaps the visible set, so an
// in-progress bulk selection must be cleared — a delete/archive applied to
// rows the user can no longer see is a real hazard, not a cosmetic one.
test('toggling the contact-info filter clears an in-progress selection', async () => {
  mockTwoPages();
  renderPage();
  await screen.findByLabelText('Select Alice');

  fireEvent.click(screen.getByLabelText('Select Alice'));
  expect(screen.getByText('1 selected')).toBeInTheDocument();

  fireEvent.click(screen.getByLabelText('Show all'));

  await waitFor(() => expect(screen.queryByText('1 selected')).not.toBeInTheDocument());
});

// --- T92 bulk merge ----------------------------------------------------------
// Merge is a pairwise flow, not a one-verb-over-N-rows action: selecting
// exactly two contacts enables the bulk bar's Merge, which opens
// MergeContactsDialog in pair mode (neither selected row is privileged —
// the user picks the keeper). A successful merge clears the selection and
// refetches so the dead loser row can't linger.

const noConflictPreview = {
  keep_id: 1,
  merge_id: 2,
  resolution: {
    emails: [],
    phones: [],
    addresses: [],
    urls: [],
    impps: [],
    resolved_scalars: {},
    conflicts: [],
    field_value_conflicts: [],
  },
  association_counts: {
    notes: 0, activities: 0, reminders: 0, reminder_completions: 0,
    relationship_edges: 0, household_memberships: 0, circle_memberships: 0,
    tags: 0, life_events: 0, life_event_references: 0, field_values: 0,
    contact_sync_links: 0, attachments: 0, preferences: 0,
    external_identities: 0, external_activities: 0, cadence_policies: 0,
  },
};

test('selecting exactly two contacts and merging opens the pair-mode dialog, then refreshes and clears', async () => {
  mockTwoPages();
  vi.mocked(previewContactMerge).mockImplementation(async (keepId, mergeId) => ({
    ...noConflictPreview,
    keep_id: keepId,
    merge_id: mergeId,
  } as never));
  vi.mocked(commitContactMerge).mockResolvedValue({ message: 'merged', contact: {} } as never);
  renderPage();
  await screen.findByLabelText('Select Alice');

  fireEvent.click(screen.getByLabelText('Select Alice'));
  fireEvent.click(screen.getByLabelText('Select Bob'));
  expect(screen.getByText('2 selected')).toBeInTheDocument();

  // The bulk bar's Merge (only one matches — the dialog is not open yet).
  fireEvent.click(screen.getByRole('button', { name: 'Merge' }));

  // Pair mode: both contacts are offered as keeper candidates, neither
  // privileged (T92 step 2 — the whole point of the lift).
  const dialog = screen.getByRole('dialog');
  expect(within(dialog).getByText('Keep Alice')).toBeInTheDocument();
  expect(within(dialog).getByText('Keep Bob')).toBeInTheDocument();

  // No conflicts → the dialog's own Merge button enables once the preview
  // lands. Scope inside the dialog: the bulk bar's Merge button is still in
  // the DOM behind it.
  const dialogMerge = within(dialog).getByRole('button', { name: 'Merge' });
  await waitFor(() => expect(dialogMerge).not.toBeDisabled());
  fireEvent.click(dialogMerge);

  // Committed as Alice (the pair default keeper) swallowing Bob, then the
  // selection clears and the list refetches.
  await waitFor(() => expect(commitContactMerge).toHaveBeenCalledWith(1, 2, {}));
  await waitFor(() => expect(screen.queryByText('2 selected')).not.toBeInTheDocument());
  expect(getContacts).toHaveBeenCalledTimes(2);
  expect(within(dialog).queryByText('Keep Alice')).not.toBeInTheDocument();
});

// Selection is keyed by VCardUID specifically so it survives pagination, and
// the merge resolves the pair from the *loaded* `contacts` array — so it must
// work across the page boundary too (Alice on page one, Carol on page two).
test('merging a pair selected across the pagination boundary resolves both rows from the loaded pages', async () => {
  mockTwoPages();
  vi.mocked(previewContactMerge).mockImplementation(async (keepId, mergeId) => ({
    ...noConflictPreview,
    keep_id: keepId,
    merge_id: mergeId,
  } as never));
  vi.mocked(commitContactMerge).mockResolvedValue({ message: 'merged', contact: {} } as never);
  renderPage();
  await screen.findByLabelText('Select Alice');

  fireEvent.click(screen.getByLabelText('Select Alice'));
  fireEvent.click(screen.getByText('Load more'));
  await screen.findByLabelText('Select Carol');
  fireEvent.click(screen.getByLabelText('Select Carol'));
  expect(screen.getByText('2 selected')).toBeInTheDocument();

  fireEvent.click(screen.getByRole('button', { name: 'Merge' }));

  const dialog = screen.getByRole('dialog');
  expect(within(dialog).getByText('Keep Alice')).toBeInTheDocument();
  expect(within(dialog).getByText('Keep Carol')).toBeInTheDocument();

  const dialogMerge = within(dialog).getByRole('button', { name: 'Merge' });
  await waitFor(() => expect(dialogMerge).not.toBeDisabled());
  fireEvent.click(dialogMerge);

  // Alice (page one) resolves as the pair default keeper, Carol (page two) as
  // the loser — the IDs come from the loaded rows, not a fresh lookup.
  await waitFor(() => expect(commitContactMerge).toHaveBeenCalledWith(1, 3, {}));
  await waitFor(() => expect(screen.queryByText('2 selected')).not.toBeInTheDocument());
});

// T77 deliberately keeps the selection across a sort change while the sort's
// refetch may drop a selected row off the loaded page. That leaves the Merge
// button enabled (selectedCount === 2) pointing at rows `contacts` no longer
// carries — resolving must surface it, not silently do nothing.
test('a merge whose selected rows left the loaded page alerts and clears the stale selection', async () => {
  vi.mocked(getContacts).mockImplementation((params) => {
    if (params?.sort === 'updated_at') {
      return Promise.resolve({ contacts: [contact(1, 'uid-1', 'Alice')], next_cursor: '', limit: 10 });
    }
    if (params?.cursor) {
      return Promise.resolve({ contacts: [contact(3, 'uid-3', 'Carol')], next_cursor: '', limit: 10 });
    }
    return Promise.resolve({ contacts: [contact(1, 'uid-1', 'Alice'), contact(2, 'uid-2', 'Bob')], next_cursor: 'cursor-1', limit: 10 });
  });
  const alertSpy = vi.spyOn(window, 'alert').mockImplementation(() => {});
  renderPage();
  await screen.findByLabelText('Select Alice');

  fireEvent.click(screen.getByLabelText('Select Alice'));
  fireEvent.click(screen.getByText('Load more'));
  await screen.findByLabelText('Select Carol');
  fireEvent.click(screen.getByLabelText('Select Carol'));
  expect(screen.getByText('2 selected')).toBeInTheDocument();

  // The sort refetch replaces the loaded page with just Alice; the selection
  // survives it (T77), so Merge is enabled but Carol can't be resolved.
  fireEvent.mouseDown(screen.getByLabelText('Sort'));
  fireEvent.click(await screen.findByRole('option', { name: 'Recently edited (newest first)' }));
  await waitFor(() => expect(screen.getByText('2 selected')).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: 'Merge' }));

  expect(alertSpy).toHaveBeenCalledWith(expect.stringContaining('no longer visible'));
  await waitFor(() => expect(screen.queryByText('2 selected')).not.toBeInTheDocument());
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  alertSpy.mockRestore();
});
