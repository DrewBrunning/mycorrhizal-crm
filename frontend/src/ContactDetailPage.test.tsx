import { cleanup, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { afterEach, expect, test, vi } from 'vitest';
import './i18n/config';
import type { ContactRecordResponse } from './api/contacts';
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

// Issue #958: one failed auxiliary fetch (notes, activities, or reminder
// completions) must not make an existing contact look deleted. Only the
// core `getContactRecord` call is allowed to gate the not-found branch.
// `notesOk: false` reproduces the reported scenario -- everything else on
// the page still succeeds (or 404s and is caught internally by its own
// hook, same as the rest of this page's many auxiliary calls).
function mockFetch({ notesOk = true }: { notesOk?: boolean } = {}) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      if (url.endsWith('/contacts/1')) {
        return { ok: true, json: async () => contactRecord };
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
}

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/contacts/1']}>
      <SnackbarProvider>
        <DateFormatProvider>
          <Routes>
            <Route path="/contacts/:id" element={<ContactDetailPage />} />
          </Routes>
        </DateFormatProvider>
      </SnackbarProvider>
    </MemoryRouter>,
  );
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
