import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { afterEach, expect, test, vi } from 'vitest';
import './i18n/config';
import type { ContactRecordResponse } from './api/contacts';
import type { OccasionObligation } from './api/occasionObligations';
import ContactDetailPage from './ContactDetailPage';
import { SnackbarProvider } from './context/SnackbarContext';
import { DateFormatProvider } from './DateFormatProvider';

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const contactRecord: ContactRecordResponse = {
  id: 1,
  uid: 'alice-uid',
  etag: '',
  revision: 1,
  card: {
    name: {
      components: [
        { kind: 'given', value: 'Alice' },
        { kind: 'surname', value: 'Wonder' },
      ],
    },
  },
  crm: { kind: 'human' },
};

interface Call {
  url: string;
  method: string;
  body?: unknown;
}

// Issue #958: one failed auxiliary fetch (notes, activities, or reminder
// completions) must not make an existing contact look deleted. Only the
// core `getContactRecord` call is allowed to gate the not-found branch.
// `notesOk: false` reproduces the reported scenario -- everything else on
// the page still succeeds (or 404s and is caught internally by its own
// hook, same as the rest of this page's many auxiliary calls).
//
// `record` seeds the GET response (and PUT echoes back a merge of it with
// the request body, so a save flows through to a real re-render). The
// per-endpoint `fail` flags let a test force one specific mutation to error
// without hand-rolling a whole new fetch mock.
function mockFetch({
  notesOk = true,
  record = contactRecord,
  fail = {},
  enabledFields,
  occasionObligations,
}: {
  notesOk?: boolean;
  record?: ContactRecordResponse;
  fail?: Partial<
    Record<'update' | 'delete' | 'archive' | 'unarchive' | 'favorite' | 'unfavorite', boolean>
  >;
  // Overrides GET /users/me's enabled_contact_fields -- some fields exercised
  // below (organization/department) are not in DEFAULT_ENABLED_CONTACT_FIELDS.
  enabledFields?: string[];
  // Seeds GET /occasion-obligations and backs POST/PUT with an in-memory
  // list, so a save flows through to a real re-render. Omit to exercise the
  // catch-all 404 path other tests rely on (useOccasionObligations catches
  // it silently, same as every other per-contact hook on this page).
  occasionObligations?: OccasionObligation[];
} = {}) {
  const calls: Call[] = [];
  let current = record;
  const obligations = occasionObligations ? [...occasionObligations] : undefined;
  const errorResponse = (message: string) => ({
    ok: false,
    status: 500,
    statusText: 'Internal Server Error',
    json: async () => ({ error: { code: 'INTERNAL_ERROR', message } }),
  });

  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, options?: RequestInit) => {
      const method = (options?.method || 'GET').toUpperCase();
      const body = options?.body ? JSON.parse(options.body as string) : undefined;
      calls.push({ url, method, body });

      if (url.endsWith('/contacts/1') && method === 'GET') {
        return { ok: true, json: async () => current };
      }
      if (url.endsWith('/contacts/1') && method === 'PUT') {
        if (fail.update) return errorResponse('update failed');
        current = {
          ...current,
          ...body,
          card: { ...current.card, ...body.card },
          crm: { ...current.crm, ...body.crm },
        };
        return { ok: true, json: async () => current };
      }
      if (url.endsWith('/contacts/1') && method === 'DELETE') {
        if (fail.delete) return errorResponse('delete failed');
        return { ok: true, json: async () => ({}) };
      }
      if (url.endsWith('/contacts/1/archive') && method === 'POST') {
        if (fail.archive) return errorResponse('archive failed');
        return { ok: true, json: async () => ({ ID: 1, archived: true }) };
      }
      if (url.endsWith('/contacts/1/unarchive') && method === 'POST') {
        if (fail.unarchive) return errorResponse('unarchive failed');
        return { ok: true, json: async () => ({ ID: 1, archived: false }) };
      }
      if (url.endsWith('/contacts/1/favorite') && method === 'POST') {
        if (fail.favorite) return errorResponse('favorite failed');
        return { ok: true, json: async () => ({ ID: 1, is_favorite: true }) };
      }
      if (url.endsWith('/contacts/1/unfavorite') && method === 'POST') {
        if (fail.unfavorite) return errorResponse('unfavorite failed');
        return { ok: true, json: async () => ({ ID: 1, is_favorite: false }) };
      }
      if (url.endsWith('/users/me') && method === 'GET') {
        if (!enabledFields)
          return { ok: false, status: 404, statusText: 'Not Found', json: async () => ({}) };
        return { ok: true, json: async () => ({ enabled_contact_fields: enabledFields }) };
      }
      if (url.includes('/contacts/1/notes')) {
        if (!notesOk) {
          return {
            ok: false,
            status: 500,
            statusText: 'Internal Server Error',
            json: async () => ({ error: { code: 'INTERNAL_ERROR', message: 'boom' } }),
          };
        }
        return { ok: true, json: async () => ({ notes: [] }) };
      }
      if (url.includes('/contacts/1/activities')) {
        return { ok: true, json: async () => ({ activities: [] }) };
      }
      if (url.includes('/contacts/1/reminder-completions')) {
        return { ok: true, json: async () => ({ completions: [] }) };
      }
      // useContactFieldValues is the one auxiliary hook on this page whose
      // notifier is wired to a fetch failure (not just its own mutations),
      // so it must succeed here or it clobbers the single shared Snackbar
      // with its own "Not Found" toast right after ours.
      if (url.includes('/contacts/1/field-values')) {
        return { ok: true, json: async () => ({ field_values: [] }) };
      }
      if (obligations && url.includes('/occasion-obligations?') && method === 'GET') {
        return { ok: true, json: async () => ({ occasion_obligations: obligations }) };
      }
      if (obligations && url.endsWith('/occasion-obligations') && method === 'POST') {
        const created: OccasionObligation = {
          id: 'new-ob',
          created_at: '',
          updated_at: '',
          ...body,
        };
        obligations.push(created);
        return { ok: true, json: async () => ({ occasion_obligation: created }) };
      }
      if (obligations && url.includes('/occasion-obligations/') && method === 'PUT') {
        const id = url.split('/occasion-obligations/')[1];
        const idx = obligations.findIndex((o) => o.id === id);
        if (idx >= 0) obligations[idx] = { ...obligations[idx], ...body };
        return { ok: true, json: async () => obligations[idx] };
      }
      // Every other endpoint this page touches on mount (current user,
      // reminders, relationship edges, life events, agenda, gifts, field
      // definitions, external links, immich, paperless/seafile/nextcloud
      // configs, circles, tags, cadence policies) is fetched by a hook that
      // already catches its own errors without notifying -- 404 exercises
      // that path rather than papering over it with a full mock of every
      // endpoint.
      return { ok: false, status: 404, statusText: 'Not Found', json: async () => ({}) };
    }),
  );
  return calls;
}

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/contacts/1']}>
      <SnackbarProvider>
        <DateFormatProvider>
          <Routes>
            <Route path="/contacts/:id" element={<ContactDetailPage />} />
            <Route path="/contacts" element={<div>CONTACTS LIST PAGE</div>} />
          </Routes>
        </DateFormatProvider>
      </SnackbarProvider>
    </MemoryRouter>,
  );
}

// EditableField's caption label and its "Edit" pencil sit in the same row
// Box; scoping to it disambiguates from every other field's identical
// "Edit" aria-label on this page.
function fieldRow(label: string): HTMLElement {
  return screen.getByText(label).parentElement as HTMLElement;
}
// The row's parent also holds the editing TextField + Save/Cancel icons
// once edit mode opens (a sibling of the row, not inside it).
function fieldContent(label: string): HTMLElement {
  return fieldRow(label).parentElement as HTMLElement;
}

test('renders the contact when every auxiliary fetch succeeds', async () => {
  mockFetch();
  renderPage();

  await waitFor(() => expect(screen.getByText('Alice Wonder')).toBeInTheDocument());
  expect(screen.queryByText('Contact not found')).not.toBeInTheDocument();
});

test('a failed notes fetch does not turn an existing contact into "not found" (#958)', async () => {
  mockFetch({ notesOk: false });
  renderPage();

  // The contact record loaded fine -- the page must still render it, not
  // fall into the notFound branch just because the notes fetch 500'd.
  await waitFor(() => expect(screen.getByText('Alice Wonder')).toBeInTheDocument());
  expect(screen.queryByText('Contact not found')).not.toBeInTheDocument();

  // The failure is surfaced as a non-fatal notification instead of being
  // silently swallowed.
  await waitFor(() =>
    expect(
      screen.getByText('Some timeline data failed to load. Try refreshing the page.'),
    ).toBeInTheDocument(),
  );
});

// --- handleToggleFavorite: optimistic rollback on error ---------------------

test('a failed favorite toggle rolls back the optimistic star and shows an error', async () => {
  mockFetch({ fail: { favorite: true } });
  renderPage();
  await waitFor(() => expect(screen.getByText('Alice Wonder')).toBeInTheDocument());

  fireEvent.click(screen.getByLabelText('Mark as favorite'));

  // Rolled back to the non-favorite star, not left on the optimistic "filled" state.
  await waitFor(() => expect(screen.getByLabelText('Mark as favorite')).toBeInTheDocument());
  // The thrown ApiError's own message surfaces verbatim (not the generic
  // fallback, which only applies to a non-ApiError failure).
  expect(screen.getByText('favorite failed')).toBeInTheDocument();
});

test('a successful favorite toggle flips the star and persists', async () => {
  mockFetch();
  renderPage();
  await waitFor(() => expect(screen.getByText('Alice Wonder')).toBeInTheDocument());

  fireEvent.click(screen.getByLabelText('Mark as favorite'));

  await waitFor(() => expect(screen.getByLabelText('Unmark as favorite')).toBeInTheDocument());
});

// --- Occasions (ADR 0024, issue #387) ---------------------------------------

test('adding an occasion opens the create dialog and saves it via POST', async () => {
  const calls = mockFetch({ occasionObligations: [] });
  renderPage();
  await waitFor(() => expect(screen.getByText('Alice Wonder')).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: 'Add occasion' }));
  expect(screen.getByText('Add an occasion')).toBeInTheDocument();

  fireEvent.change(screen.getByLabelText('Label *'), {
    target: { value: 'Christmas card' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await waitFor(() => {
    const postCall = calls.find(
      (c) => c.method === 'POST' && c.url.endsWith('/occasion-obligations'),
    );
    expect(postCall?.body).toMatchObject({ entity_id: 'alice-uid', label: 'Christmas card' });
  });
  await waitFor(() => expect(screen.queryByText('Add an occasion')).not.toBeInTheDocument());
});

test('editing an existing occasion opens the edit dialog pre-filled, and saves via PUT', async () => {
  const existing: OccasionObligation = {
    id: 'ob-1',
    created_at: '',
    updated_at: '',
    entity_id: 'alice-uid',
    kind: 'card',
    label: 'Christmas card',
    lead_time_days: 14,
    active: true,
    sensitivity: 'normal',
  };
  const calls = mockFetch({ occasionObligations: [existing] });
  renderPage();
  await waitFor(() => expect(screen.getByText('Alice Wonder')).toBeInTheDocument());
  await waitFor(() => expect(screen.getByText('Christmas card')).toBeInTheDocument());

  const item = screen.getByText('Christmas card').closest('.MuiPaper-root') as HTMLElement;
  fireEvent.click(within(item).getByLabelText('Edit'));
  expect(screen.getByText('Edit occasion')).toBeInTheDocument();
  expect(screen.getByDisplayValue('Christmas card')).toBeInTheDocument();

  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await waitFor(() => {
    const putCall = calls.find(
      (c) => c.method === 'PUT' && c.url.endsWith('/occasion-obligations/ob-1'),
    );
    expect(putCall?.body).toMatchObject({ label: 'Christmas card' });
  });
});

test('cancelling the occasion dialog closes it without saving', async () => {
  const calls = mockFetch({ occasionObligations: [] });
  renderPage();
  await waitFor(() => expect(screen.getByText('Alice Wonder')).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: 'Add occasion' }));
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

  await waitFor(() => expect(screen.queryByText('Add an occasion')).not.toBeInTheDocument());
  expect(calls.some((c) => c.method === 'POST' && c.url.endsWith('/occasion-obligations'))).toBe(
    false,
  );
});

// --- buildRecordPatch: the field-to-patch switch ----------------------------

test('editing the birthday saves it as a Card anniversary of kind "birth"', async () => {
  const calls = mockFetch();
  renderPage();
  await waitFor(() => expect(screen.getByText('Alice Wonder')).toBeInTheDocument());

  fireEvent.click(within(fieldRow('Birthday')).getByLabelText('Edit'));
  const content = fieldContent('Birthday');
  // The default display format (no stored preference) is EU (DD.MM.YYYY),
  // and onEditValueChange runs every keystroke through autoFormatBirthdayInput
  // -- it extracts digits positionally (DD then MM then YYYY) and re-inserts
  // separators, so the value it lands on here is exactly "30.04.1990".
  fireEvent.change(within(content).getByRole('textbox'), {
    target: { value: '30.04.1990' },
  });
  fireEvent.click(within(content).getByLabelText('Save'));

  await waitFor(() => {
    const putCall = calls.find((c) => c.method === 'PUT');
    expect(putCall).toBeDefined();
  });
  const putCall = calls.find((c) => c.method === 'PUT');
  expect(putCall?.body).toMatchObject({
    card: {
      anniversaries: [{ kind: 'birth', date: { partial: { year: 1990, month: 4, day: 30 } } }],
    },
  });
});

test('an invalid birthday is rejected before any save request', async () => {
  const calls = mockFetch();
  renderPage();
  await waitFor(() => expect(screen.getByText('Alice Wonder')).toBeInTheDocument());

  fireEvent.click(within(fieldRow('Birthday')).getByLabelText('Edit'));
  const content = fieldContent('Birthday');
  // Day 01, month 13 -- autoFormatBirthdayInput reformats the typed digits to
  // "01.13.1990" (still DD.MM.YYYY shaped), but month 13 doesn't exist.
  fireEvent.change(within(content).getByRole('textbox'), {
    target: { value: '01.13.1990' },
  });
  fireEvent.click(within(content).getByLabelText('Save'));

  expect(
    await screen.findByText('Invalid date format. Check your date format setting.'),
  ).toBeInTheDocument();
  expect(calls.some((c) => c.method === 'PUT')).toBe(false);
});

test('editing the organization preserves the existing department (the paired half)', async () => {
  const calls = mockFetch({
    enabledFields: ['organizations'],
    record: {
      ...contactRecord,
      card: {
        ...contactRecord.card,
        organizations: [{ name: 'Acme', units: [{ name: 'Sales' }] }],
      },
    },
  });
  renderPage();
  await waitFor(() => expect(screen.getByText('Alice Wonder')).toBeInTheDocument());
  await waitFor(() => expect(screen.getByText('Organization')).toBeInTheDocument());

  fireEvent.click(within(fieldRow('Organization')).getByLabelText('Edit'));
  const content = fieldContent('Organization');
  fireEvent.change(within(content).getByRole('textbox'), { target: { value: 'NewCo' } });
  fireEvent.click(within(content).getByLabelText('Save'));

  await waitFor(() => {
    expect(calls.some((c) => c.method === 'PUT')).toBe(true);
  });
  const putCall = calls.find((c) => c.method === 'PUT');
  expect(putCall?.body).toMatchObject({
    card: { organizations: [{ name: 'NewCo', units: [{ name: 'Sales' }] }] },
  });
});

test('editing "how we met" saves it as a plain CRM string field', async () => {
  const calls = mockFetch();
  renderPage();
  await waitFor(() => expect(screen.getByText('Alice Wonder')).toBeInTheDocument());

  fireEvent.click(within(fieldRow('How We Met')).getByLabelText('Edit'));
  const content = fieldContent('How We Met');
  fireEvent.change(within(content).getByRole('textbox'), {
    target: { value: 'At a conference' },
  });
  fireEvent.click(within(content).getByLabelText('Save'));

  await waitFor(() => {
    const putCall = calls.find((c) => c.method === 'PUT');
    expect(putCall?.body).toMatchObject({ crm: { how_we_met: 'At a conference' } });
  });
});

// --- handleSaveProfile: name-component assembly + validation ---------------

test('handleSaveProfile alerts and does not save when the first name is blank', async () => {
  const calls = mockFetch();
  const alertSpy = vi.spyOn(window, 'alert').mockImplementation(() => {});
  renderPage();
  await waitFor(() => expect(screen.getByText('Alice Wonder')).toBeInTheDocument());

  // The header's own name-edit pencil is first in the DOM; ContactInformation's
  // per-field pencils (also aria-label "Edit") come after it.
  fireEvent.click(screen.getAllByLabelText('Edit')[0]);
  const firstNameInput = screen.getByLabelText('First Name *');
  fireEvent.change(firstNameInput, { target: { value: '   ' } });
  fireEvent.click(screen.getByLabelText('Save'));

  expect(alertSpy).toHaveBeenCalledWith('First name is required');
  expect(calls.some((c) => c.method === 'PUT')).toBe(false);
  alertSpy.mockRestore();
});

test('handleSaveProfile assembles the edited name components into Card.name', async () => {
  const calls = mockFetch();
  renderPage();
  await waitFor(() => expect(screen.getByText('Alice Wonder')).toBeInTheDocument());

  fireEvent.click(screen.getAllByLabelText('Edit')[0]);
  fireEvent.change(screen.getByLabelText('First Name *'), { target: { value: 'Alicia' } });
  fireEvent.change(screen.getByLabelText('Last Name'), { target: { value: 'Wonderland' } });
  fireEvent.change(screen.getByLabelText('Nickname'), { target: { value: 'Ali' } });
  fireEvent.click(screen.getByLabelText('Save'));

  await waitFor(() => {
    expect(calls.some((c) => c.method === 'PUT')).toBe(true);
  });
  const putCall = calls.find((c) => c.method === 'PUT');
  expect(putCall?.body).toMatchObject({
    card: {
      name: {
        components: [
          { kind: 'given', value: 'Alicia' },
          { kind: 'surname', value: 'Wonderland' },
        ],
      },
      nicknames: [{ name: 'Ali' }],
    },
  });
});

// --- handleDeleteContact / handleArchiveContact -----------------------------

test('deleting a contact asks for confirmation, then navigates away on success', async () => {
  mockFetch();
  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
  renderPage();
  await waitFor(() => expect(screen.getByText('Alice Wonder')).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: 'Delete' }));

  expect(confirmSpy).toHaveBeenCalledWith(
    'Are you sure you want to delete Alice Wonder? This action cannot be undone and will also delete all notes, activities, and reminders associated with this contact.',
  );
  await waitFor(() => expect(screen.getByText('CONTACTS LIST PAGE')).toBeInTheDocument());
  confirmSpy.mockRestore();
});

test('declining the delete confirmation makes no request and stays on the page', async () => {
  const calls = mockFetch();
  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false);
  renderPage();
  await waitFor(() => expect(screen.getByText('Alice Wonder')).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: 'Delete' }));

  expect(calls.some((c) => c.method === 'DELETE')).toBe(false);
  expect(screen.getByText('Alice Wonder')).toBeInTheDocument();
  confirmSpy.mockRestore();
});

test('a failed delete alerts an error and stays on the page', async () => {
  mockFetch({ fail: { delete: true } });
  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
  const alertSpy = vi.spyOn(window, 'alert').mockImplementation(() => {});
  renderPage();
  await waitFor(() => expect(screen.getByText('Alice Wonder')).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: 'Delete' }));

  await waitFor(() =>
    expect(alertSpy).toHaveBeenCalledWith('Failed to delete contact. Please try again.'),
  );
  expect(screen.queryByText('CONTACTS LIST PAGE')).not.toBeInTheDocument();
  confirmSpy.mockRestore();
  alertSpy.mockRestore();
});

test('archiving a contact asks for confirmation, then flips to the archived badge', async () => {
  mockFetch();
  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
  renderPage();
  await waitFor(() => expect(screen.getByText('Alice Wonder')).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: 'Archive' }));

  expect(confirmSpy).toHaveBeenCalledWith(
    "Are you sure you want to archive this contact? All reminders will be deleted. You can unarchive later, but reminders won't be restored.",
  );
  await waitFor(() => expect(screen.getByText('Archived')).toBeInTheDocument());
  confirmSpy.mockRestore();
});

test('declining the archive confirmation leaves the contact unarchived', async () => {
  const calls = mockFetch();
  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false);
  renderPage();
  await waitFor(() => expect(screen.getByText('Alice Wonder')).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: 'Archive' }));

  expect(calls.some((c) => c.url.endsWith('/archive'))).toBe(false);
  expect(screen.queryByText('Archived')).not.toBeInTheDocument();
  confirmSpy.mockRestore();
});

test('a failed archive shows an error and leaves the contact unarchived', async () => {
  mockFetch({ fail: { archive: true } });
  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
  renderPage();
  await waitFor(() => expect(screen.getByText('Alice Wonder')).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: 'Archive' }));

  await waitFor(() => expect(screen.getByText('archive failed')).toBeInTheDocument());
  expect(screen.queryByText('Archived')).not.toBeInTheDocument();
  confirmSpy.mockRestore();
});
