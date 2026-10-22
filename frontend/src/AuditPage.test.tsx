import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import './i18n/config';
import AuditPage from './AuditPage';
import { type AuditEvent, getAuditEvents, undoAuditEvent } from './api/audit';
import { ApiError } from './api/client';
import { type Contact, getContactsByUid } from './api/contacts';
import { SnackbarProvider } from './context/SnackbarContext';
import { DateFormatProvider, setDateFormatPreference } from './DateFormatProvider';
import { setTimeFormat } from './timeFormat';

// T60. This codebase's vitest
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
    </MemoryRouter>,
  );
}

test('renders the empty state when there are no events', async () => {
  auditGetMock.mockResolvedValue({ audit_events: [], total: 0 });
  renderPage();

  expect(
    await screen.findByText(
      'No audit events yet. Changes you make to your contacts and other records will appear here.',
    ),
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

// Issue #1830: audit timestamps follow the user's 12/24-hour preference.
test('audit timestamps render 24-hour or 12-hour per the time-format preference', async () => {
  auditGetMock.mockResolvedValue({
    audit_events: [ev({ id: 7, created_at: '2026-08-02T15:30:00Z' })],
    total: 1,
  });
  const stamp = (container: HTMLElement) =>
    container.querySelector('tbody tr td p')?.textContent ?? '';
  try {
    setTimeFormat('24h');
    const first = renderPage();
    await screen.findAllByRole('button', { name: 'Undo' });
    expect(stamp(first.container)).toMatch(/\d{1,2}:\d{2}/);
    expect(stamp(first.container)).not.toMatch(/AM|PM/i);
    first.unmount();

    setTimeFormat('12h');
    const second = renderPage();
    await screen.findAllByRole('button', { name: 'Undo' });
    expect(stamp(second.container)).toMatch(/AM|PM/i);
  } finally {
    setTimeFormat('auto');
  }
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

// #243: the contact link is the sole content of its <td> (not inline prose),
// so it needs its own 24px target rather than relying on body2's 18px line-height.
test('the resolved-contact link meets the 24px minimum target size', async () => {
  auditGetMock.mockResolvedValue({
    audit_events: [ev({ id: 3, entity_type: 'contact', operation: 'update' })],
    total: 1,
  });
  renderPage();

  const link = await screen.findByRole('link', { name: 'Alice Johnson' });
  expect(getComputedStyle(link).minHeight).toBe('24px');
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
    { timeout: 2000 },
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
    await screen.findByText('This event is past its retention window and can no longer be undone'),
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

// --- #1792 / #1788: keyboard-reachable scroll region, phone card list ---------

function setPhoneViewport(phone: boolean) {
  window.matchMedia = ((query: string) => ({
    matches: phone && query.includes('max-width'),
    media: query,
    onchange: null,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
  })) as unknown as typeof window.matchMedia;
}

afterEach(() => {
  // @ts-expect-error -- restore jsdom's "no matchMedia" default
  delete window.matchMedia;
});

test('the table scroll container is a focusable, labelled region (#1792)', async () => {
  auditGetMock.mockResolvedValue({ audit_events: [ev({})], total: 1 });
  renderPage();

  // A horizontally scrolling container with nothing focusable inside cannot be
  // scrolled from the keyboard (WCAG 2.1.1), so the region itself takes focus.
  const region = await screen.findByRole('region', { name: 'Audit log' });
  expect(region).toHaveAttribute('tabindex', '0');
  expect(within(region).getByRole('table')).toBeInTheDocument();
});

test('below sm the audit log is a card list, not a 5-column table (#1788)', async () => {
  setPhoneViewport(true);
  auditGetMock.mockResolvedValue({
    audit_events: [ev({ id: 1 }), ev({ id: 2, operation: 'delete', entity_type: 'note' })],
    total: 2,
  });
  renderPage();

  const list = await screen.findByRole('list', { name: 'Audit log' });
  expect(screen.queryByRole('table')).not.toBeInTheDocument();
  const items = within(list).getAllByRole('listitem');
  expect(items).toHaveLength(2);
  // Each card carries what the columns did: entity type, operation, entity,
  // and (on a contact update) the Undo action.
  expect(within(items[0]).getByText('Contact')).toBeInTheDocument();
  expect(within(items[0]).getByText('Updated')).toBeInTheDocument();
  expect(await within(items[0]).findByText('Alice Johnson')).toBeInTheDocument();
  fireEvent.click(within(items[0]).getByRole('button', { name: /undo/i }));
  expect(await screen.findByRole('dialog')).toBeInTheDocument();
  // A delete event has no Undo.
  expect(within(items[1]).queryByRole('button', { name: /undo/i })).not.toBeInTheDocument();
});

// Issue #1789: the date half follows the Date format preference (a European
// user must not see mm/dd/yyyy), and seconds are never shown.
test('audit timestamps follow the date preference and omit seconds', async () => {
  auditGetMock.mockResolvedValue({
    audit_events: [ev({ id: 7, created_at: '2026-08-02T15:30:00' })],
    total: 1,
  });
  setDateFormatPreference('eu');
  setTimeFormat('24h');
  try {
    const { container } = renderPage();
    await screen.findAllByRole('button', { name: 'Undo' });
    const stamp = container.querySelector('tbody tr td p')?.textContent ?? '';
    expect(stamp).toBe('02.08.2026 15:30');
    expect(stamp).not.toMatch(/\d{2}:\d{2}:\d{2}/);
  } finally {
    setDateFormatPreference('eu');
    setTimeFormat('auto');
  }
});
