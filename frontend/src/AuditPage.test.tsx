import { test, expect, vi, afterEach, beforeEach } from 'vitest';
import { render, screen, cleanup, fireEvent, waitFor, within } from '@testing-library/react';
import { MemoryRouter, Routes, Route } from 'react-router';
import './i18n/config';
import AuditPage from './AuditPage';
import { SnackbarProvider } from './context/SnackbarContext';
import { DateFormatProvider } from './DateFormatProvider';
import { getAuditEvents, undoAuditEvent, AuditEvent } from './api/audit';
import { getContactsByUid, Contact } from './api/contacts';
import { ApiError } from './api/client';

// T60 (docs/fork-plan/tickets/79-T60-audit-trail-ui.md). This codebase's vitest
// has no auto-cleanup, so it must be done explicitly (CLAUDE.md frontend trap #1).
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

vi.mock('./api/audit', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/audit')>();
  return { ...actual, getAuditEvents: vi.fn(), undoAuditEvent: vi.fn() };
});
vi.mock('./api/contacts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/contacts')>();
  return { ...actual, getContactsByUid: vi.fn() };
});

const auditGetMock = vi.mocked(getAuditEvents);
const undoMock = vi.mocked(undoAuditEvent);
const contactsByUidMock = vi.mocked(getContactsByUid);

function ev(overrides: Partial<AuditEvent>): AuditEvent {
  return {
    id: 1,
    created_at: '2026-08-02T12:00:00Z',
    entity_type: 'contact',
    entity_id: 'uid-1',
    operation: 'update',
    ...overrides,
  };
}

function contact(uid: string, ID: number, firstname: string, lastname: string): Contact {
  return { ID, uid, firstname, lastname, archived: false };
}

beforeEach(() => {
  auditGetMock.mockReset();
  undoMock.mockReset();
  contactsByUidMock.mockReset();
  const alice = contact('uid-1', 5, 'Alice', 'Johnson');
  contactsByUidMock.mockResolvedValue(new Map<string, Contact>([['uid-1', alice]]));
});

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/audit']}>
      <DateFormatProvider>
        <SnackbarProvider>
          <Routes>
            <Route path="/audit" element={<AuditPage />} />
            <Route path="/contacts/:id" element={<div>CONTACT DETAIL PAGE</div>} />
          </Routes>
        </SnackbarProvider>
      </DateFormatProvider>
    </MemoryRouter>
  );
}

test('renders the empty state when there are no events', async () => {
  auditGetMock.mockResolvedValue({ audit_events: [], total: 0 });
  renderPage();

  expect(
    await screen.findByText('No audit events yet. Changes you make to your contacts and other records will appear here.')
  ).toBeDefined();
});

test('renders events with the Undo button only on contact update rows', async () => {
  auditGetMock.mockResolvedValue({
    audit_events: [
      ev({ id: 3, entity_type: 'contact', operation: 'update' }),
      ev({ id: 2, entity_type: 'note', operation: 'create' }),
      ev({ id: 1, entity_type: 'contact', operation: 'delete' }),
    ],
    total: 3,
  });
  renderPage();

  // Only the contact-update row offers undo -- the backend 400s a delete or a
  // non-contact undo, so the button gate must match the server's gate.
  const undoButtons = await screen.findAllByRole('button', { name: 'Undo' });
  expect(undoButtons).toHaveLength(1);

  expect(screen.getByText('Created')).toBeDefined();
  expect(screen.getByText('Deleted')).toBeDefined();
  expect(screen.getAllByText('Contact')).toHaveLength(2);
  expect(screen.getByText('Note')).toBeDefined();
});

test('resolves a contact uid to a link to the contact detail page', async () => {
  auditGetMock.mockResolvedValue({
    audit_events: [ev({ id: 3, entity_type: 'contact', operation: 'update' })],
    total: 1,
  });
  renderPage();

  const link = await screen.findByRole('link', { name: 'Alice Johnson' });
  expect(link.getAttribute('href')).toBe('/contacts/5');
});

test('a contact uid that no longer resolves renders as plain text, not a broken link', async () => {
  contactsByUidMock.mockResolvedValue(new Map());
  auditGetMock.mockResolvedValue({
    audit_events: [ev({ id: 3, entity_type: 'contact', operation: 'update' })],
    total: 1,
  });
  renderPage();

  expect(await screen.findByText('uid-1')).toBeDefined();
  expect(screen.queryByRole('link')).toBeNull();
});

test('the entity type dropdown filters server-side', async () => {
  auditGetMock.mockResolvedValue({ audit_events: [], total: 0 });
  renderPage();
  await screen.findByText('Audit log');

  fireEvent.mouseDown(screen.getByLabelText('Entity type'));
  const option = await screen.findByRole('option', { name: 'Note' });
  fireEvent.click(option);

  await waitFor(() => {
    expect(auditGetMock).toHaveBeenCalledWith(expect.objectContaining({ entity_type: 'note' }));
  });
});

test('the clear filters button resets both filters', async () => {
  auditGetMock.mockResolvedValue({ audit_events: [], total: 0 });
  renderPage();
  await screen.findByText('Audit log');

  const idInput = screen.getByLabelText('Entity ID');
  fireEvent.change(idInput, { target: { value: 'abc' } });

  // The entity_id field is debounced, so the request lags the keystroke.
  await waitFor(
    () => {
      expect(auditGetMock).toHaveBeenCalledWith(expect.objectContaining({ entity_id: 'abc' }));
    },
    { timeout: 2000 }
  );

  fireEvent.click(screen.getByRole('button', { name: 'Clear filters' }));

  await waitFor(() => {
    const lastCall = auditGetMock.mock.calls[auditGetMock.mock.calls.length - 1];
    expect(lastCall[0]).toEqual(expect.objectContaining({ entity_id: undefined }));
  });
});

test('undo asks for confirmation, POSTs, shows a success toast and refreshes', async () => {
  auditGetMock.mockResolvedValue({
    audit_events: [ev({ id: 3, entity_type: 'contact', operation: 'update' })],
    total: 1,
  });
  undoMock.mockResolvedValue({ message: 'Contact restored to its previous state' });
  renderPage();

  fireEvent.click(await screen.findByRole('button', { name: 'Undo' }));

  const dialog = screen.getByRole('dialog');
  expect(within(dialog).getByText('Undo this change?')).toBeDefined();
  fireEvent.click(within(dialog).getByRole('button', { name: 'Undo' }));

  await waitFor(() => {
    expect(undoMock).toHaveBeenCalledWith(3);
  });
  expect(await screen.findByText('Contact restored to its previous state')).toBeDefined();
  // A successful undo refreshes the list.
  await waitFor(() => {
    expect(auditGetMock.mock.calls.length).toBeGreaterThan(1);
  });
});

test('a 410 from undo shows the retention-window message', async () => {
  auditGetMock.mockResolvedValue({
    audit_events: [ev({ id: 3, entity_type: 'contact', operation: 'update' })],
    total: 1,
  });
  undoMock.mockRejectedValue(new ApiError('past retention', 'GONE', 410));
  renderPage();

  fireEvent.click(await screen.findByRole('button', { name: 'Undo' }));
  const dialog = screen.getByRole('dialog');
  fireEvent.click(within(dialog).getByRole('button', { name: 'Undo' }));

  expect(
    await screen.findByText('This event is past its retention window and can no longer be undone')
  ).toBeDefined();
});

test('a 400 from undo surfaces the server error message', async () => {
  auditGetMock.mockResolvedValue({
    audit_events: [ev({ id: 3, entity_type: 'contact', operation: 'update' })],
    total: 1,
  });
  undoMock.mockRejectedValue(new ApiError('undo is not supported for note', 'INVALID_INPUT', 400));
  renderPage();

  fireEvent.click(await screen.findByRole('button', { name: 'Undo' }));
  const dialog = screen.getByRole('dialog');
  fireEvent.click(within(dialog).getByRole('button', { name: 'Undo' }));

  expect(await screen.findByText('undo is not supported for note')).toBeDefined();
});
