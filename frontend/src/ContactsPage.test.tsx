import { test, expect, vi, afterEach, beforeEach } from 'vitest';
import { render, screen, cleanup, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
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

  vi.mocked(getCurrentUser).mockResolvedValue({ enabled_contact_fields: null } as never);
  vi.mocked(listCircles).mockResolvedValue({ circles: [], members: [], next_cursor: '', limit: 100 } as never);
  vi.mocked(listTags).mockResolvedValue({ tags: [], contacts: [], next_cursor: '', limit: 100 } as never);
  vi.mocked(getFieldDefinitions).mockResolvedValue({ field_definitions: [] } as never);
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
    <MemoryRouter>
      <DateFormatProvider>
        <SnackbarProvider>
          <ContactsPage />
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
