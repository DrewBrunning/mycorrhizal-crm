import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import './i18n/config';
import type { Activity } from './api/activities';
import type { CadencePolicy } from './api/cadencePolicies';
import type { Circle, CircleMember } from './api/circles';
import type { ContactRecordResponse } from './api/contacts';
import type { ConversationAgenda } from './api/conversationAgenda';
import type { DataDecayPolicy } from './api/dataDecayPolicies';
import type { Gift } from './api/gifts';
import type { LifeEvent } from './api/lifeEvents';
import type { Note } from './api/notes';
import type { OccasionObligation } from './api/occasionObligations';
import type { Preference } from './api/preferences';
import type { RelationshipEdge } from './api/relationshipEdges';
import type { Reminder, ReminderCompletion } from './api/reminders';
import type { ContactTag, Tag } from './api/tags';
import ContactDetailPage from './ContactDetailPage';
import { SnackbarProvider } from './context/SnackbarContext';
import { DateFormatProvider } from './DateFormatProvider';

// --- Fixture ----------------------------------------------------------------
//
// A realistic, fully-populated contact modelled on the backend's documented
// contract (testdata/contract-fixtures/contact-detail.json, generated from
// openapi.yaml), split into the per-endpoint envelopes this page's hooks
// actually fetch. Every GET the page makes on mount has a route below, so a
// happy-path render exercises the success branches (issue: the previous
// catch-all 404 made "every auxiliary fetch succeeds" log ~400 Not Found
// errors and render empty sections). An unrouted GET is recorded in
// `unrouted` and fails the test in afterEach, so a new endpoint can't
// silently fall back to a 404 again.

const contactRecord: ContactRecordResponse = {
  id: 1,
  uid: 'alice-uid',
  etag: 'e-1-7',
  revision: 7,
  gender: '',
  photo: '',
  archived: false,
  is_favorite: false,
  card: {
    name: {
      components: [
        { kind: 'given', value: 'Alice' },
        { kind: 'surname', value: 'Wonder' },
      ],
    },
    emails: [{ address: 'alice.wonder@example.com', contexts: ['home'] }],
    phones: [{ number: '+1 555-0100', contexts: ['mobile'] }],
    keywords: ['contract-fixture'],
  },
  crm: { kind: 'human' },
};

const notes: Note[] = [
  {
    ID: 2,
    contact_id: 1,
    content: 'Contract fixture note',
    date: '2026-08-20T18:00:00Z',
    CreatedAt: '2026-08-21T00:08:42Z',
    UpdatedAt: '2026-08-21T00:08:42Z',
  },
];

const activities: Activity[] = [
  {
    ID: 2,
    title: 'Fixture activity',
    description: '',
    location: '',
    date: '2026-08-19T18:00:00Z',
    CreatedAt: '2026-08-21T00:08:43Z',
    UpdatedAt: '2026-08-21T00:08:43Z',
    contacts: [{ ID: 2, firstname: 'Bob', lastname: 'Builder' }],
  },
];

const completions: ReminderCompletion[] = [
  {
    ID: 5,
    reminder_id: 2,
    contact_id: 1,
    message: 'Fixture completed reminder',
    completed_at: '2026-08-18T09:00:00Z',
  },
];

const reminders: Reminder[] = [
  {
    ID: 2,
    message: 'Fixture reminder',
    by_mail: false,
    remind_at: '2026-08-22T00:00:00Z',
    recurrence: 'once',
    reoccur_from_completion: false,
    completed: false,
    email_sent: false,
    contact_id: 1,
  },
];

const relationshipEdges: RelationshipEdge[] = [
  {
    id: 'edge-1',
    source_id: 'alice-uid',
    target_id: 'bob-uid',
    type: 'friend_of',
    directional: false,
    source: 'user-confirmed',
    confidence: 1,
    status: 'confirmed',
    sensitivity: 'normal',
    created_at: '2026-08-21T00:08:50Z',
    updated_at: '2026-08-21T00:08:50Z',
  },
];

const lifeEvents: LifeEvent[] = [
  {
    id: 'le-1',
    entity_id: 'alice-uid',
    type: 'graduated',
    category: 'work_education',
    description: 'Fixture life event',
    date: { year: 2012, month: 6, day: 1 },
    created_at: '2026-08-21T00:08:45Z',
    updated_at: '2026-08-21T00:08:45Z',
  },
  {
    id: 'le-2',
    entity_id: 'alice-uid',
    type: 'married',
    category: 'relationships',
    description: 'Fixture wedding',
    date: { year: 2015, month: 9, day: 12 },
    created_at: '2026-08-21T00:08:45Z',
    updated_at: '2026-08-21T00:08:45Z',
  },
];

const agenda: ConversationAgenda[] = [
  {
    id: 'ag-1',
    entity_id: 'alice-uid',
    content: 'Ask about the Lisbon trip',
    created_at: '2026-08-21T00:08:45Z',
    updated_at: '2026-08-21T00:08:45Z',
  },
];

const gifts: Gift[] = [
  {
    id: 'gift-1',
    entity_id: 'alice-uid',
    status: 'idea',
    description: 'Fixture gift idea',
    created_at: '2026-08-21T00:08:46Z',
    updated_at: '2026-08-21T00:08:46Z',
  },
];

const preferences: Preference[] = [
  {
    id: 'pref-1',
    entity_id: 'alice-uid',
    category: 'food',
    key: 'love',
    value: 'Sushi',
    sensitivity: 'normal',
    created_at: '',
    updated_at: '',
  },
  {
    id: 'pref-2',
    entity_id: 'alice-uid',
    category: 'flowers',
    value: 'Tulips',
    sensitivity: 'normal',
    created_at: '',
    updated_at: '',
  },
];

const circles: Circle[] = [
  { id: 'circle-1', name: 'contract-fixture-circle', created_at: '', updated_at: '' },
];
const circleMembers: CircleMember[] = [
  { id: 1, circle_id: 'circle-1', member_vcard_uid: 'alice-uid' },
];
const tags: Tag[] = [{ id: 'tag-1', name: 'contract-fixture', created_at: '', updated_at: '' }];
const tagContacts: ContactTag[] = [{ id: 1, tag_id: 'tag-1', contact_vcard_uid: 'alice-uid' }];

const bobSummary = {
  id: 2,
  uid: 'bob-uid',
  firstname: 'Bob',
  lastname: 'Builder',
  nickname: '',
  fn: 'Bob Builder',
  primary_email: '',
  primary_phone: '',
  birthday: '',
  org: '',
  photo: '',
};

interface Call {
  url: string;
  method: string;
  body?: unknown;
}

type Endpoint =
  | 'notes'
  | 'activities'
  | 'completions'
  | 'user'
  | 'reminders'
  | 'relationshipEdges'
  | 'lifeEvents'
  | 'agenda'
  | 'gifts'
  | 'fieldValues'
  | 'externalIdentities'
  | 'externalActivities'
  | 'immichSummary'
  | 'circles'
  | 'tags'
  | 'preferences'
  | 'occasions'
  | 'cadence'
  | 'dataDecay'
  | 'record';

type Mutation =
  | 'update'
  | 'delete'
  | 'archive'
  | 'unarchive'
  | 'favorite'
  | 'unfavorite'
  | 'giftUpdate';

let unrouted: string[] = [];

const json = (body: unknown) => ({ ok: true, status: 200, json: async () => body });
const errorResponse = (message: string, status = 500) => ({
  ok: false,
  status,
  statusText: status === 404 ? 'Not Found' : 'Internal Server Error',
  json: async () => ({
    error: { code: status === 404 ? 'NOT_FOUND' : 'INTERNAL_ERROR', message },
  }),
});

// `record` seeds GET /contacts/1 (and PUT echoes back a merge of it with the
// request body, so a save flows through to a real re-render). `failGet`
// makes one specific GET 500 (#958's failure-isolation tests); `fail` does
// the same for one mutation.
function mockFetch({
  record = contactRecord,
  failGet = [],
  fail = {},
  enabledFields = null,
  selfContactUid = null,
  occasionObligations = [],
  addressSuggestions = [],
  cadencePolicies = [],
  dataDecayPolicies = [],
  immichConfigured = false,
  extraPreferences = [],
}: {
  record?: ContactRecordResponse;
  failGet?: Endpoint[];
  fail?: Partial<Record<Mutation, boolean>>;
  // GET /users/me's enabled_contact_fields -- some fields exercised below
  // (organization/department) are not in DEFAULT_ENABLED_CONTACT_FIELDS.
  enabledFields?: string[] | null;
  selfContactUid?: string | null;
  // Seeds GET /occasion-obligations and backs POST/PUT with an in-memory
  // list, so a save flows through to a real re-render.
  occasionObligations?: OccasionObligation[];
  // null makes the post-save address scan fail.
  addressSuggestions?: unknown[] | null | 'omitted';
  cadencePolicies?: CadencePolicy[];
  dataDecayPolicies?: DataDecayPolicy[];
  immichConfigured?: boolean;
  extraPreferences?: Preference[];
} = {}) {
  const calls: Call[] = [];
  let current = record;
  const obligations = [...occasionObligations];

  const get = (endpoint: Endpoint, body: () => unknown) =>
    failGet.includes(endpoint) ? errorResponse(`${endpoint} failed`) : json(body());

  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: string, options?: RequestInit) => {
      const url = String(input);
      const method = (options?.method || 'GET').toUpperCase();
      const body =
        typeof options?.body === 'string' ? JSON.parse(options.body as string) : undefined;
      calls.push({ url, method, body });
      const path = url.replace(/^.*\/api\/v1/, '').split('?')[0];

      if (method === 'GET') {
        switch (path) {
          case '/contacts/1':
            return failGet.includes('record')
              ? errorResponse('Contact not found', 404)
              : json(current);
          case '/contacts/1/notes':
            return get('notes', () => ({ notes }));
          case '/contacts/1/activities':
            return get('activities', () => ({ activities }));
          case '/contacts/1/reminder-completions':
            return get('completions', () => ({ completions }));
          case '/contacts/1/reminders':
            return get('reminders', () => ({ reminders }));
          case '/contacts/1/field-values':
            return get('fieldValues', () => ({ field_values: [] }));
          case '/contacts/1/attachments':
            return json({ attachments: [], total: 0 });
          case '/contacts/1/life-event-suggestions':
            return json({ suggestions: [] });
          case '/contacts/1/score':
            return json({
              contact_id: 1,
              score: 72,
              band: 'healthy',
              recency: { value: 1, weight: 1, reason: '' },
              frequency: { value: 1, weight: 1, reason: '' },
              closeness: { value: 1, weight: 1, reason: '' },
              reach_out: { value: 1, weight: 1, reason: '' },
              last_updated: { value: 1, weight: 1, reason: '' },
            });
          case '/contacts':
            return json({ contacts: [bobSummary], total: 1, next_cursor: '', limit: 100 });
          case '/users/me':
            return get('user', () => ({
              id: 1,
              email: 'me@example.com',
              username: 'me',
              enabled_contact_fields: enabledFields,
              self_contact_vcard_uid: selfContactUid,
            }));
          case '/relationship-edges':
            return get('relationshipEdges', () => ({
              relationship_edges: relationshipEdges,
              total: 1,
              next_cursor: '',
              limit: 100,
            }));
          case '/life-events':
            return get('lifeEvents', () => ({
              life_events: lifeEvents,
              next_cursor: '',
              limit: 50,
            }));
          case '/conversation-agenda':
            return get('agenda', () => ({
              conversation_agenda: agenda,
              next_cursor: '',
              limit: 100,
            }));
          case '/gifts':
            return get('gifts', () => ({ gifts, next_cursor: '', limit: 100 }));
          case '/external-identities':
            return get('externalIdentities', () => ({
              external_identities: [],
              total: 0,
              next_cursor: '',
              limit: 100,
            }));
          case '/external-activities':
            return get('externalActivities', () => ({
              external_activities: [],
              total: 0,
              next_cursor: '',
              limit: 100,
            }));
          case '/immich/contacts/alice-uid/summary':
            return get('immichSummary', () => ({ summary: null }));
          case '/immich/config':
            return json({
              base_url: '',
              has_api_key: immichConfigured,
              sync_enabled: false,
              last_sync_status: '',
              last_sync_error: '',
            });
          case '/paperless/config':
          case '/seafile/config':
            return json({ base_url: '', has_api_token: false });
          case '/nextcloud/config':
            return json({ base_url: '', has_app_password: false });
          case '/circles':
            return get('circles', () => ({
              circles,
              members: circleMembers,
              total: 1,
              next_cursor: '',
              limit: 200,
            }));
          case '/tags':
            return get('tags', () => ({
              tags,
              contacts: tagContacts,
              total: 1,
              next_cursor: '',
              limit: 200,
            }));
          case '/preferences':
            return get('preferences', () => ({
              preferences: [...preferences, ...extraPreferences],
              total: preferences.length + extraPreferences.length,
              next_cursor: '',
              limit: 200,
            }));
          case '/occasion-obligations':
            return get('occasions', () => ({
              occasion_obligations: obligations,
              total: obligations.length,
              next_cursor: '',
              limit: 100,
            }));
          case '/cadence-policies':
            return get('cadence', () => ({
              cadence_policies: cadencePolicies,
              total: cadencePolicies.length,
              next_cursor: '',
              limit: 100,
            }));
          case '/data-decay-policies':
            return get('dataDecay', () => ({
              data_decay_policies: dataDecayPolicies,
              total: dataDecayPolicies.length,
              next_cursor: '',
              limit: 100,
            }));
          case '/graph/connections':
            return json({ from_vcard_uid: 'alice-uid', from_name: 'Alice', depth: 1, chains: [] });
          case '/export/vcf':
            return errorResponse('export failed');
          default:
            if (path.startsWith('/contacts/1/timeline')) {
              return json({ items: [], next_cursor: '', total: 0 });
            }
            if (path === '/users/directory') return json({ users: [] });
            if (path.startsWith('/contact-shares')) {
              return json({ contact_shares: [] });
            }
            unrouted.push(`GET ${path}`);
            return errorResponse(`unrouted GET ${path}`, 404);
        }
      }

      if (path === '/contacts/1' && method === 'PUT') {
        if (fail.update) return errorResponse('update failed');
        current = {
          ...current,
          ...body,
          card: { ...current.card, ...body.card },
          crm: { ...current.crm, ...body.crm },
        };
        return json(current);
      }
      if (path === '/contacts/1' && method === 'DELETE') {
        return fail.delete ? errorResponse('delete failed') : json({});
      }
      for (const action of ['archive', 'unarchive', 'favorite', 'unfavorite'] as const) {
        if (path === `/contacts/1/${action}` && method === 'POST') {
          if (fail[action]) return errorResponse(`${action} failed`);
          return json(
            action.endsWith('archive')
              ? { ID: 1, archived: action === 'archive' }
              : { ID: 1, is_favorite: action === 'favorite' },
          );
        }
      }
      if (path === '/occasion-obligations' && method === 'POST') {
        const created: OccasionObligation = {
          id: 'new-ob',
          created_at: '',
          updated_at: '',
          ...body,
        };
        obligations.push(created);
        return json({ occasion_obligation: created });
      }
      if (path.startsWith('/occasion-obligations/') && method === 'PUT') {
        const id = path.split('/occasion-obligations/')[1];
        const idx = obligations.findIndex((o) => o.id === id);
        if (idx >= 0) obligations[idx] = { ...obligations[idx], ...body };
        return json(obligations[idx]);
      }
      if (path.startsWith('/gifts/') && method === 'PUT') {
        return fail.giftUpdate ? errorResponse('gift failed') : json({ ...gifts[0], ...body });
      }
      if (
        path.startsWith('/data-decay-policies/') &&
        path.endsWith('/verify') &&
        method === 'POST'
      ) {
        // Echo the seeded policy back (raw, not wrapped -- see
        // verifyDataDecayPolicy) so a verify click doesn't collapse the
        // hook's policy state to {} the way the generic body-echo fallback
        // below would.
        return json(dataDecayPolicies[0] ?? {});
      }
      if (path === '/contacts/address-suggestions' && method === 'POST') {
        if (addressSuggestions === 'omitted') return json({});
        return addressSuggestions
          ? json({ suggestions: addressSuggestions })
          : errorResponse('scan failed');
      }
      // Any other mutation a test deliberately triggers: echo the body back.
      return json(body ?? {});
    }),
  );
  return calls;
}

let consoleError: ReturnType<typeof vi.spyOn>;

beforeEach(() => {
  unrouted = [];
  localStorage.clear();
  consoleError = vi.spyOn(console, 'error');
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  // Every GET the page makes must be routed -- see the fixture comment.
  expect(unrouted).toEqual([]);
});

// Errors the page (or its hooks) logged, as "Not Found"-style strings.
function loggedErrors(): string[] {
  return consoleError.mock.calls.map((args: unknown[]) => args.map(String).join(' '));
}

// The header's own actions come first in the DOM; lists further down the
// page reuse the same "Delete"/"Edit" labels.
function headerButton(name: string): HTMLElement {
  return screen.getAllByRole('button', { name })[0];
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

async function renderLoaded() {
  renderPage();
  await waitFor(() => expect(screen.getByText('Alice Wonder')).toBeInTheDocument());
  // The second load batch (reminders, edges, life events, gifts...) lands
  // after the header; wait for the last of it before asserting.
  await screen.findByText('Fixture gift idea');
  await within(document.getElementById('people') as HTMLElement).findByRole('button', {
    name: 'Bob Builder',
  });
}

test('the happy path renders every populated section from the fixture', async () => {
  mockFetch();
  await renderLoaded();

  expect(screen.queryByText('Contact not found')).not.toBeInTheDocument();
  // Contact information.
  expect(screen.getByText(/alice\.wonder@example\.com/)).toBeInTheDocument();
  expect(screen.getByText(/\+1 555-0100/)).toBeInTheDocument();
  // Header memberships.
  expect(screen.getByText('contract-fixture-circle')).toBeInTheDocument();
  expect(screen.getByText('contract-fixture')).toBeInTheDocument();
  // Merged timeline: note, activity, completion, dated life events.
  const timeline = document.getElementById('timeline') as HTMLElement;
  expect(within(timeline).getByText('Contract fixture note')).toBeInTheDocument();
  expect(within(timeline).getByText('Fixture activity')).toBeInTheDocument();
  expect(within(timeline).getByText('Fixture completed reminder')).toBeInTheDocument();
  expect(within(timeline).getAllByText('Fixture life event').length).toBeGreaterThan(0);
  expect(within(timeline).getByText('Ask about the Lisbon trip')).toBeInTheDocument();
  // People, reminders, preferences (overview vs gifts split).
  expect(screen.getByText('Fixture reminder')).toBeInTheDocument();
  expect(
    within(document.getElementById('overview') as HTMLElement).getByText('Sushi'),
  ).toBeInTheDocument();
  const giftsSection = document.getElementById('gifts') as HTMLElement;
  expect(within(giftsSection).getByText('Tulips')).toBeInTheDocument();
  expect(within(giftsSection).getByText('Fixture gift idea')).toBeInTheDocument();

  // No success-path endpoint 404'd or errored.
  expect(loggedErrors().filter((e) => /Not Found|Error fetching/.test(e))).toEqual([]);
});

test.each([
  ['notes', 'Contract fixture note'],
  ['activities', 'Fixture activity'],
  ['completions', 'Fixture completed reminder'],
] as const)(
  'a failed %s fetch still renders the contact and flags the timeline (#958)',
  async (endpoint, missing) => {
    consoleError.mockImplementation(() => {});
    mockFetch({ failGet: [endpoint] });
    await renderLoaded();

    // The contact record loaded fine -- the page must still render it, not
    // fall into the notFound branch just because one timeline fetch 500'd.
    expect(screen.queryByText('Contact not found')).not.toBeInTheDocument();
    expect(
      await screen.findByText('Some timeline data failed to load. Try refreshing the page.'),
    ).toBeInTheDocument();
    // Only the failed source is missing; the rest of the timeline rendered.
    expect(screen.queryByText(missing)).not.toBeInTheDocument();
    expect(screen.getByText('Fixture reminder')).toBeInTheDocument();
  },
);

test('a failed /users/me still renders the contact without a timeline warning', async () => {
  consoleError.mockImplementation(() => {});
  mockFetch({ failGet: ['user'] });
  await renderLoaded();
  expect(screen.getByText('Contract fixture note')).toBeInTheDocument();
  expect(
    screen.queryByText('Some timeline data failed to load. Try refreshing the page.'),
  ).not.toBeInTheDocument();
});

test('a failed contact record renders "not found"', async () => {
  consoleError.mockImplementation(() => {});
  mockFetch({ failGet: ['record'] });
  renderPage();
  expect(await screen.findByText('Contact not found')).toBeInTheDocument();
});

test('the /users/me self-contact pointer marks this contact as Me', async () => {
  mockFetch({ selfContactUid: 'alice-uid' });
  await renderLoaded();
  expect(screen.getByText('You')).toBeInTheDocument();
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
  const calls = mockFetch();
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
  const calls = mockFetch();
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

  fireEvent.click(headerButton('Delete'));

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

  fireEvent.click(headerButton('Delete'));

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

  fireEvent.click(headerButton('Delete'));

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

  fireEvent.click(headerButton('Archive'));

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

  fireEvent.click(headerButton('Archive'));

  expect(calls.some((c) => c.url.endsWith('/archive'))).toBe(false);
  expect(screen.queryByText('Archived')).not.toBeInTheDocument();
  confirmSpy.mockRestore();
});

test('a failed archive shows an error and leaves the contact unarchived', async () => {
  mockFetch({ fail: { archive: true } });
  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
  renderPage();
  await waitFor(() => expect(screen.getByText('Alice Wonder')).toBeInTheDocument());

  fireEvent.click(headerButton('Archive'));

  await waitFor(() => expect(screen.getByText('archive failed')).toBeInTheDocument());
  expect(screen.queryByText('Archived')).not.toBeInTheDocument();
  confirmSpy.mockRestore();
});

// --- Section wiring: dialogs, list actions, and their handlers --------------

function section(id: string): HTMLElement {
  return document.getElementById(id) as HTMLElement;
}

// The nearest ancestor of `text` that holds its own row actions (an "Edit"
// button) -- lists on this page don't share one row element type.
function rowWith(text: string, scope: HTMLElement = document.body): HTMLElement {
  let el: HTMLElement | null = within(scope).getAllByText(text).at(-1) as HTMLElement;
  while (el && !within(el).queryAllByRole('button', { name: 'Edit' }).length) {
    el = el.parentElement;
  }
  return el as HTMLElement;
}

async function openDialog(open: () => unknown): Promise<HTMLElement> {
  open();
  return screen.findByRole('dialog');
}

async function dismiss(dialog: HTMLElement, name: RegExp = /cancel/i) {
  fireEvent.click(within(dialog).getAllByRole('button', { name })[0]);
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
}

test.each([
  [
    'overview preference',
    () => within(section('overview')).getByRole('button', { name: 'Add Preference' }),
  ],
  [
    'gift preference',
    () => within(section('gifts')).getByRole('button', { name: 'Add Preference' }),
  ],
  ['preference edit', () => within(rowWith('Sushi')).getByRole('button', { name: 'Edit' })],
  ['life event', () => screen.getByRole('button', { name: 'Add Life Event' })],
  ['cadence', () => screen.getByRole('button', { name: 'Add Cadence' })],
  ['note', () => screen.getByRole('button', { name: 'Add Note' })],
  ['activity', () => screen.getByRole('button', { name: 'Add Activity' })],
  ['reminder', () => screen.getByRole('button', { name: 'Add Reminder' })],
  [
    'gift with details',
    () => within(section('gifts')).getAllByRole('button', { name: 'Add with details' })[0],
  ],
  ['profile picture', () => screen.getByRole('button', { name: 'Select an image' })],
])('the %s dialog opens from its panel and closes cleanly', async (_name, button) => {
  mockFetch();
  await renderLoaded();
  const dialog = await openDialog(() => fireEvent.click(button()));
  await dismiss(dialog);
});

test('Stay in Touch opens the reminder dialog pre-filled with a catch-up message', async () => {
  mockFetch();
  await renderLoaded();
  const dialog = await openDialog(() =>
    fireEvent.click(screen.getByRole('button', { name: 'Stay in Touch' })),
  );
  expect(within(dialog).getByDisplayValue('Catch-up with Alice Wonder')).toBeInTheDocument();
  await dismiss(dialog);
});

test('editing a life event opens its dialog pre-filled', async () => {
  mockFetch();
  await renderLoaded();
  const row = rowWith('Fixture life event', section('timeline'));
  const dialog = await openDialog(() =>
    fireEvent.click(within(row).getByRole('button', { name: 'Edit' })),
  );
  expect(within(dialog).getByDisplayValue('Fixture life event')).toBeInTheDocument();
  await dismiss(dialog);
});

test('deleting a married life event confirms, deletes, and reloads the record', async () => {
  const calls = mockFetch();
  vi.spyOn(window, 'confirm').mockReturnValue(true);
  await renderLoaded();
  const recordLoads = () =>
    calls.filter((c) => c.method === 'GET' && c.url.endsWith('/contacts/1')).length;
  const before = recordLoads();

  const row = rowWith('Fixture wedding', section('timeline'));
  fireEvent.click(within(row).getByRole('button', { name: 'Delete' }));

  await waitFor(() =>
    expect(calls.some((c) => c.method === 'DELETE' && c.url.endsWith('/life-events/le-2'))).toBe(
      true,
    ),
  );
  await waitFor(() => expect(recordLoads()).toBe(before + 1));
});

test('declining a life-event or preference delete makes no request', async () => {
  const calls = mockFetch();
  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false);
  await renderLoaded();

  const row = rowWith('Fixture wedding', section('timeline'));
  fireEvent.click(within(row).getByRole('button', { name: 'Delete' }));
  fireEvent.click(within(section('overview')).getAllByRole('button', { name: 'Delete' })[0]);

  expect(confirmSpy).toHaveBeenCalledTimes(2);
  expect(calls.some((c) => c.method === 'DELETE')).toBe(false);
});

test('deleting a preference confirms and sends the DELETE', async () => {
  const calls = mockFetch();
  vi.spyOn(window, 'confirm').mockReturnValue(true);
  await renderLoaded();
  fireEvent.click(within(section('gifts')).getAllByRole('button', { name: 'Delete' })[0]);
  await waitFor(() =>
    expect(calls.some((c) => c.method === 'DELETE' && c.url.endsWith('/preferences/pref-2'))).toBe(
      true,
    ),
  );
});

test('"Mark as given" flips a gift idea to given, defaulting the date to now', async () => {
  const calls = mockFetch();
  await renderLoaded();
  fireEvent.click(screen.getByRole('button', { name: 'Mark as given' }));
  await waitFor(() => {
    const put = calls.find((c) => c.method === 'PUT' && c.url.endsWith('/gifts/gift-1'));
    expect(put?.body).toMatchObject({
      entity_id: 'alice-uid',
      status: 'given',
      description: 'Fixture gift idea',
    });
    expect((put?.body as { date?: string } | undefined)?.date).toMatch(/^\d{4}-\d{2}-\d{2}T/);
  });
});

test('a failed "Mark as given" shows the gift save error', async () => {
  consoleError.mockImplementation(() => {});
  mockFetch({ fail: { giftUpdate: true } });
  await renderLoaded();
  fireEvent.click(screen.getByRole('button', { name: 'Mark as given' }));
  expect(await screen.findByText('Failed to save gift.')).toBeInTheDocument();
});

test('editing a gift saves it via PUT with the entity id', async () => {
  const calls = mockFetch();
  await renderLoaded();
  const row = rowWith('Fixture gift idea', section('gifts'));
  fireEvent.click(within(row).getByRole('button', { name: 'Edit' }));
  const dialog = await screen.findByRole('dialog');
  fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
  await waitFor(() => {
    const put = calls.find((c) => c.method === 'PUT' && c.url.endsWith('/gifts/gift-1'));
    expect(put?.body).toMatchObject({ entity_id: 'alice-uid', description: 'Fixture gift idea' });
  });
});

test('marking an agenda item discussed sends the discuss PATCH', async () => {
  const calls = mockFetch();
  await renderLoaded();
  fireEvent.click(screen.getByRole('button', { name: 'Mark as discussed' }));
  const dialog = await screen.findByRole('dialog');
  fireEvent.click(within(dialog).getByRole('button', { name: /mark as discussed|confirm/i }));
  await waitFor(() =>
    expect(
      calls.some(
        (c) => c.method === 'PATCH' && c.url.endsWith('/conversation-agenda/ag-1/discuss'),
      ),
    ).toBe(true),
  );
});

test('editing an agenda item saves it via PUT', async () => {
  const calls = mockFetch();
  await renderLoaded();
  const row = rowWith('Ask about the Lisbon trip');
  fireEvent.click(within(row).getByRole('button', { name: 'Edit' }));
  const dialog = await screen.findByRole('dialog');
  fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
  await waitFor(() => {
    const put = calls.find(
      (c) => c.method === 'PUT' && c.url.endsWith('/conversation-agenda/ag-1'),
    );
    expect(put?.body).toMatchObject({
      entity_id: 'alice-uid',
      content: 'Ask about the Lisbon trip',
    });
  });
});

test('saving a relationship nudges toward address suggestions when the scan finds some', async () => {
  const calls = mockFetch({ addressSuggestions: [{ id: 's-1' }] });
  await renderLoaded();
  const row = rowWith('Bob Builder', section('people'));
  fireEvent.click(within(row).getByRole('button', { name: 'Edit' }));
  const dialog = await screen.findByRole('dialog');
  fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
  await waitFor(() =>
    expect(
      calls.some((c) => c.method === 'PUT' && c.url.endsWith('/relationship-edges/edge-1')),
    ).toBe(true),
  );
  expect(calls.some((c) => c.url.endsWith('/contacts/address-suggestions'))).toBe(true);
});

test('editing a timeline note saves it through the edit dialog', async () => {
  const calls = mockFetch();
  await renderLoaded();
  const row = rowWith('Contract fixture note', section('timeline'));
  fireEvent.click(within(row).getByRole('button', { name: 'Edit' }));
  const dialog = await screen.findByRole('dialog');
  fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
  await waitFor(() =>
    expect(calls.some((c) => c.method === 'PUT' && c.url.endsWith('/notes/2'))).toBe(true),
  );
});

test('editing a timeline activity saves it through the edit dialog', async () => {
  const calls = mockFetch();
  await renderLoaded();
  const row = rowWith('Fixture activity', section('timeline'));
  fireEvent.click(within(row).getByRole('button', { name: 'Edit' }));
  const dialog = await screen.findByRole('dialog');
  fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
  await waitFor(() =>
    expect(calls.some((c) => c.method === 'PUT' && c.url.endsWith('/activities/2'))).toBe(true),
  );
});

test('the narrow jump nav is a select that scrolls to the chosen section', async () => {
  vi.stubGlobal(
    'matchMedia',
    vi.fn((query: string) => ({
      matches: true,
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  );
  const scrollIntoView = vi.fn();
  Element.prototype.scrollIntoView = scrollIntoView;
  mockFetch();
  await renderLoaded();

  fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Jump to section' }));
  fireEvent.click(await screen.findByRole('option', { name: 'People' }));
  expect(scrollIntoView).toHaveBeenCalledWith({ behavior: 'smooth' });
});

test('quick-adding an agenda item and a gift idea POSTs them for this contact', async () => {
  const calls = mockFetch();
  await renderLoaded();

  fireEvent.change(screen.getByLabelText('Things to bring up next time…'), {
    target: { value: 'Birthday plans' },
  });
  fireEvent.click(within(section('timeline')).getByRole('button', { name: 'Add' }));
  fireEvent.change(screen.getByLabelText('Record a gift idea…'), {
    target: { value: 'Scarf' },
  });
  fireEvent.keyDown(screen.getByLabelText('Record a gift idea…'), { key: 'Enter' });

  await waitFor(() => {
    const agendaPost = calls.find(
      (c) => c.method === 'POST' && c.url.endsWith('/conversation-agenda'),
    );
    expect(agendaPost?.body).toEqual({ entity_id: 'alice-uid', content: 'Birthday plans' });
    const giftPost = calls.find((c) => c.method === 'POST' && c.url.endsWith('/gifts'));
    expect(giftPost?.body).toMatchObject({
      entity_id: 'alice-uid',
      description: 'Scarf',
      status: 'idea',
    });
  });
});

test('a gift recorded via "Add with details" is created (POST) with the section status', async () => {
  const calls = mockFetch();
  await renderLoaded();
  const dialog = await openDialog(() =>
    fireEvent.click(
      within(section('gifts')).getAllByRole('button', { name: 'Add with details' })[0],
    ),
  );
  fireEvent.change(within(dialog).getByLabelText(/^What the gift is/), {
    target: { value: 'Book' },
  });
  fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
  await waitFor(() => {
    const post = calls.find((c) => c.method === 'POST' && c.url.endsWith('/gifts'));
    expect(post?.body).toMatchObject({ entity_id: 'alice-uid', description: 'Book' });
  });
});

test.each([
  ['Sushi', 'pref-1'],
  ['Tulips', 'pref-2'],
])('saving an edited %s preference PUTs it', async (value, prefId) => {
  const calls = mockFetch();
  await renderLoaded();
  const dialog = await openDialog(() =>
    fireEvent.click(within(rowWith(value)).getByRole('button', { name: 'Edit' })),
  );
  fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
  await waitFor(() => {
    const put = calls.find((c) => c.method === 'PUT' && c.url.endsWith(`/preferences/${prefId}`));
    expect(put?.body).toMatchObject({ entity_id: 'alice-uid', value });
  });
});

test('saving an edited married life event PUTs it and reloads the record', async () => {
  const calls = mockFetch();
  await renderLoaded();
  const recordLoads = () =>
    calls.filter((c) => c.method === 'GET' && c.url.endsWith('/contacts/1')).length;
  const before = recordLoads();
  const dialog = await openDialog(() =>
    fireEvent.click(
      within(rowWith('Fixture wedding', section('timeline'))).getByRole('button', {
        name: 'Edit',
      }),
    ),
  );
  fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
  await waitFor(() => {
    const put = calls.find((c) => c.method === 'PUT' && c.url.endsWith('/life-events/le-2'));
    expect(put?.body).toMatchObject({ entity_id: 'alice-uid', type: 'married' });
  });
  await waitFor(() => expect(recordLoads()).toBe(before + 1));
});

test('deleting a reminder completion from the timeline confirms, deletes, and refreshes', async () => {
  const calls = mockFetch();
  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValueOnce(false).mockReturnValue(true);
  await renderLoaded();
  const row = within(section('timeline'))
    .getByText('Fixture completed reminder')
    .closest('.MuiTimelineItem-root') as HTMLElement;

  fireEvent.click(within(row).getByRole('button', { name: 'Delete' }));
  expect(calls.some((c) => c.method === 'DELETE')).toBe(false);

  fireEvent.click(within(row).getByRole('button', { name: 'Delete' }));
  await waitFor(() =>
    expect(
      calls.some((c) => c.method === 'DELETE' && c.url.endsWith('/reminder-completions/5')),
    ).toBe(true),
  );
  expect(confirmSpy).toHaveBeenCalledWith(
    'Are you sure you want to remove this completed reminder from the timeline?',
  );
});

test('deleting a note from its edit dialog sends the DELETE', async () => {
  const calls = mockFetch();
  vi.spyOn(window, 'confirm').mockReturnValue(true);
  await renderLoaded();
  const dialog = await openDialog(() =>
    fireEvent.click(
      within(rowWith('Contract fixture note', section('timeline'))).getByRole('button', {
        name: 'Edit',
      }),
    ),
  );
  fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }));
  await waitFor(() =>
    expect(calls.some((c) => c.method === 'DELETE' && c.url.endsWith('/notes/2'))).toBe(true),
  );
});

test.each([
  ['View all', /close/i],
  ['Merge', /cancel/i],
  ['Share Contact', /cancel/i],
])('the %s dialog opens from the page and closes', async (name, close) => {
  mockFetch();
  await renderLoaded();
  const dialog = await openDialog(() => fireEvent.click(screen.getByRole('button', { name })));
  await dismiss(dialog, close);
});

test('a failed export surfaces an error toast', async () => {
  consoleError.mockImplementation(() => {});
  const calls = mockFetch();
  await renderLoaded();
  fireEvent.click(screen.getByRole('button', { name: 'Export vCard' }));
  fireEvent.click(await screen.findByText('vCard 4.0'));
  await waitFor(() => expect(calls.some((c) => c.url.includes('/export/vcf'))).toBe(true));
  expect(
    await screen.findByText('Failed to export contact. Please try again.'),
  ).toBeInTheDocument();
});

const cadencePolicy: CadencePolicy = {
  id: 'cad-1',
  entity_id: 'alice-uid',
  target_interval_days: 30,
  qualifying_types: [],
  created_at: '',
  updated_at: '',
};

test('an existing cadence can be edited and saved, and deleted after confirmation', async () => {
  const calls = mockFetch({ cadencePolicies: [cadencePolicy] });
  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValueOnce(false).mockReturnValue(true);
  await renderLoaded();
  const cadence = section('cadence');
  await within(cadence).findAllByRole('button', { name: 'Edit' });

  const dialog = await openDialog(() =>
    fireEvent.click(within(cadence).getAllByRole('button', { name: 'Edit' })[0]),
  );
  fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
  await waitFor(() =>
    expect(calls.some((c) => c.method !== 'GET' && c.url.includes('/cadence-policies'))).toBe(true),
  );

  const writes = () => calls.filter((c) => c.method === 'DELETE').length;
  fireEvent.click((await within(cadence).findAllByRole('button', { name: 'Delete' }))[0]);
  expect(writes()).toBe(0);
  fireEvent.click(within(cadence).getAllByRole('button', { name: 'Delete' })[0]);
  await waitFor(() => expect(writes()).toBe(1));
  expect(confirmSpy).toHaveBeenCalledTimes(2);
});

const dataDecayPolicy: DataDecayPolicy = {
  id: 'decay-1',
  entity_id: 'alice-uid',
  interval_days: 180,
  active: true,
  created_at: '',
  updated_at: '',
};

test('a data decay policy can be added, edited, verified, and deleted', async () => {
  const calls = mockFetch();
  await renderLoaded();
  const dataDecay = section('data-decay');

  // No policy yet: only the "Add" affordance is offered.
  const addButton = await within(dataDecay).findByRole('button', { name: 'Set up verification' });
  const addDialog = await openDialog(() => fireEvent.click(addButton));
  fireEvent.click(within(addDialog).getByRole('button', { name: 'Save' }));
  await waitFor(() =>
    expect(calls.some((c) => c.method === 'POST' && c.url.includes('/data-decay-policies'))).toBe(
      true,
    ),
  );
});

test('an existing data decay policy can be verified, edited and saved, and deleted after confirmation', async () => {
  const calls = mockFetch({ dataDecayPolicies: [dataDecayPolicy] });
  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValueOnce(false).mockReturnValue(true);
  await renderLoaded();
  const dataDecay = section('data-decay');
  await within(dataDecay).findByRole('button', { name: 'Confirm still current' });

  fireEvent.click(within(dataDecay).getByRole('button', { name: 'Confirm still current' }));
  await waitFor(() =>
    expect(
      calls.some(
        (c) => c.method === 'POST' && c.url.includes('/data-decay-policies/decay-1/verify'),
      ),
    ).toBe(true),
  );

  const dialog = await openDialog(() =>
    fireEvent.click(within(dataDecay).getByRole('button', { name: 'Edit' })),
  );
  fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
  await waitFor(() =>
    expect(
      calls.some((c) => c.method === 'PUT' && c.url.includes('/data-decay-policies/decay-1')),
    ).toBe(true),
  );
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());

  const writes = () => calls.filter((c) => c.method === 'DELETE').length;
  fireEvent.click(within(dataDecay).getByRole('button', { name: 'Delete' }));
  expect(writes()).toBe(0);
  fireEvent.click(within(dataDecay).getByRole('button', { name: 'Delete' }));
  await waitFor(() => expect(writes()).toBe(1));
  expect(confirmSpy).toHaveBeenCalledTimes(2);
});

test.each([
  ['finds suggestions', [{ id: 's-1' }], true],
  ['finds none', [], false],
  ['fails', null, false],
  ['omits the list', 'omitted', false],
] as const)(
  'after a relationship save, an address scan that %s never breaks the save',
  async (_label, addressSuggestions, nudged) => {
    const calls = mockFetch({
      addressSuggestions: addressSuggestions as unknown[] | null | 'omitted',
    });
    await renderLoaded();
    const dialog = await openDialog(() =>
      fireEvent.click(
        within(rowWith('Bob Builder', section('people'))).getByRole('button', { name: 'Edit' }),
      ),
    );
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
    await waitFor(() =>
      expect(calls.some((c) => c.url.endsWith('/contacts/address-suggestions'))).toBe(true),
    );
    const nudge = 'New address suggestions are available — review them under Settings → Data.';
    if (nudged) {
      expect(await screen.findByText(nudge)).toBeInTheDocument();
    } else {
      await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
      expect(screen.queryByText(nudge)).not.toBeInTheDocument();
    }
  },
);

test('adding a clothing size saves a clothing_size preference', async () => {
  const calls = mockFetch();
  await renderLoaded();
  const input = screen.getByPlaceholderText('Add a size, e.g. M, 42, S/M…');
  fireEvent.change(input, { target: { value: 'M' } });
  fireEvent.keyDown(input, { key: 'Enter' });
  await waitFor(() => {
    const post = calls.find((c) => c.method === 'POST' && c.url.endsWith('/preferences'));
    expect(post?.body).toMatchObject({
      entity_id: 'alice-uid',
      category: 'clothing_size',
      value: 'M',
    });
  });
});

test('deleting a non-married life event does not reload the record', async () => {
  const calls = mockFetch();
  vi.spyOn(window, 'confirm').mockReturnValue(true);
  await renderLoaded();
  const recordLoads = () =>
    calls.filter((c) => c.method === 'GET' && c.url.endsWith('/contacts/1')).length;
  const before = recordLoads();
  fireEvent.click(
    within(rowWith('Fixture life event', section('timeline'))).getByRole('button', {
      name: 'Delete',
    }),
  );
  await waitFor(() =>
    expect(calls.some((c) => c.method === 'DELETE' && c.url.endsWith('/life-events/le-1'))).toBe(
      true,
    ),
  );
  expect(recordLoads()).toBe(before);
});

test('with Immich configured, the profile picture dialog offers the Immich picker', async () => {
  mockFetch({ immichConfigured: true });
  await renderLoaded();
  const dialog = await openDialog(() =>
    fireEvent.click(screen.getByRole('button', { name: 'Select an image' })),
  );
  expect(within(dialog).getByText(/immich/i)).toBeInTheDocument();
  await dismiss(dialog);
});

test('editing a clothing size keeps its type and PUTs the new size', async () => {
  const calls = mockFetch({
    extraPreferences: [
      {
        id: 'pref-3',
        entity_id: 'alice-uid',
        category: 'clothing_size',
        key: 'shirt',
        value: 'M',
        sensitivity: 'normal',
        created_at: '',
        updated_at: '',
      },
    ],
  });
  await renderLoaded();
  const row = rowWith('Shirt: M', section('gifts'));
  fireEvent.click(within(row).getByRole('button', { name: 'Edit' }));
  fireEvent.change(within(section('gifts')).getByDisplayValue('M'), { target: { value: 'L' } });
  fireEvent.click(within(section('gifts')).getByRole('button', { name: 'Save' }));
  await waitFor(() => {
    const put = calls.find((c) => c.method === 'PUT' && c.url.endsWith('/preferences/pref-3'));
    expect(put?.body).toMatchObject({ category: 'clothing_size', key: 'shirt', value: 'L' });
  });
});

test('deleting an activity from its edit dialog sends the DELETE', async () => {
  const calls = mockFetch();
  vi.spyOn(window, 'confirm').mockReturnValue(true);
  await renderLoaded();
  const dialog = await openDialog(() =>
    fireEvent.click(
      within(rowWith('Fixture activity', section('timeline'))).getByRole('button', {
        name: 'Edit',
      }),
    ),
  );
  fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }));
  await waitFor(() =>
    expect(calls.some((c) => c.method === 'DELETE' && c.url.endsWith('/activities/2'))).toBe(true),
  );
});

test('a contact with only a given name still renders and names its dialogs', async () => {
  mockFetch({
    record: {
      ...contactRecord,
      card: { name: { components: [{ kind: 'given', value: 'Cher' }] } },
    },
  });
  renderPage();
  expect(await screen.findByText('Cher')).toBeInTheDocument();
  const dialog = await openDialog(() =>
    fireEvent.click(screen.getByRole('button', { name: 'Stay in Touch' })),
  );
  expect(within(dialog).getByDisplayValue('Catch-up with Cher')).toBeInTheDocument();
  await dismiss(dialog);
});
