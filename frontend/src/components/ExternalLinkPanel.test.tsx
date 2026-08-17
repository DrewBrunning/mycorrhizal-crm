import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent, waitFor } from '@testing-library/react';
import '../i18n/config';
import ExternalLinkPanel from './ExternalLinkPanel';
import { DateFormatProvider } from '../DateFormatProvider';
import { ExternalIdentity } from '../api/externalLinks';
import { ImmichPerson, ImmichPersonSummary } from '../api/immich';

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const immichPerson: ImmichPerson = { id: 'p-alice', name: 'Alice Example' };

const identity: ExternalIdentity = {
  id: 'ei-1',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  entity_id: 'alice-uid',
  system: 'immich',
  external_id: 'p-alice',
  url: 'https://immich.example/people/p-alice',
  metadata: { person_name: 'Alice Example' },
  sync_status: 'idle',
};

const summary: ImmichPersonSummary = {
  identity,
  person_name: 'Alice Example',
  photo_count: 42,
  latest_asset_id: 'asset-1',
  latest_at: '2026-08-03T10:00:00Z',
};

function renderPanel(overrides: Partial<React.ComponentProps<typeof ExternalLinkPanel>> = {}) {
  const defaults: React.ComponentProps<typeof ExternalLinkPanel> = {
    contactUid: 'alice-uid',
    identities: [],
    onFetchImmichPeople: vi.fn().mockResolvedValue([immichPerson]),
    onLinkImmich: vi.fn().mockResolvedValue(undefined),
    onUnlinkImmich: vi.fn().mockResolvedValue(undefined),
    onSyncImmich: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };
  return render(
    <DateFormatProvider>
      <ExternalLinkPanel {...defaults} />
    </DateFormatProvider>
  );
}

test('unlinked contact shows the link affordance', () => {
  renderPanel();
  expect(screen.getByRole('button', { name: 'Link a person' })).toBeInTheDocument();
});

test('linking opens the person picker and calls onLinkImmich on pick', async () => {
  const onLinkImmich = vi.fn().mockResolvedValue(undefined);
  renderPanel({ onLinkImmich });

  fireEvent.click(screen.getByRole('button', { name: 'Link a person' }));
  await waitFor(() => expect(screen.getByText('Alice Example')).toBeInTheDocument());

  fireEvent.click(screen.getByText('Alice Example'));
  await waitFor(() => expect(onLinkImmich).toHaveBeenCalledWith(immichPerson));
});

test('linked contact shows the live summary and the deep link', () => {
  renderPanel({ identities: [identity], immichSummary: summary });
  expect(screen.getByText('Alice Example')).toBeInTheDocument();
  expect(screen.getByText('42 photos')).toBeInTheDocument();
  expect(screen.getByText(/Last appearance/)).toBeInTheDocument();
  expect(screen.getByRole('link', { name: /Open in Immich/ })).toHaveAttribute(
    'href',
    'https://immich.example/people/p-alice'
  );
});

test('unlink asks for confirmation then fires the handler', async () => {
  vi.stubGlobal('confirm', vi.fn(() => true));
  const onUnlinkImmich = vi.fn().mockResolvedValue(undefined);
  renderPanel({ identities: [identity], immichSummary: summary, onUnlinkImmich });

  fireEvent.click(screen.getByRole('button', { name: 'Unlink' }));
  await waitFor(() => expect(onUnlinkImmich).toHaveBeenCalled());
});

test('a cancelled unlink does not fire the handler', async () => {
  vi.stubGlobal('confirm', vi.fn(() => false));
  const onUnlinkImmich = vi.fn().mockResolvedValue(undefined);
  renderPanel({ identities: [identity], immichSummary: summary, onUnlinkImmich });

  fireEvent.click(screen.getByRole('button', { name: 'Unlink' }));
  expect(onUnlinkImmich).not.toHaveBeenCalled();
});

test('an unsafe Immich deep-link URL is shown as text, never as an href', () => {
  // A URL that predates the httpurl validator (or arrives via a non-API
  // path) must not become a clickable javascript: link (T41).
  renderPanel({
    identities: [{ ...identity, url: 'javascript:alert(1)' }],
    immichSummary: summary,
  });

  expect(screen.queryByRole('link')).not.toBeInTheDocument();
  expect(screen.getByText('javascript:alert(1)')).toBeInTheDocument();
});

test('a non-http scheme Immich deep-link URL is shown as text, never as an href', () => {
  renderPanel({
    identities: [{ ...identity, url: 'mailto:alice@example.com' }],
    immichSummary: summary,
  });

  expect(screen.queryByRole('link')).not.toBeInTheDocument();
  expect(screen.getByText('mailto:alice@example.com')).toBeInTheDocument();
});

test('a generic identity with an unsafe URL is shown as text, never as an href', () => {
  const matrix: ExternalIdentity = {
    id: 'ei-2',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    entity_id: 'alice-uid',
    system: 'matrix',
    external_id: 'room-42',
    url: 'ftp://matrix.example/rooms/42',
    metadata: {},
    sync_status: 'idle',
  };
  renderPanel({ identities: [matrix] });

  expect(screen.queryByRole('link')).not.toBeInTheDocument();
  expect(screen.getByText('ftp://matrix.example/rooms/42')).toBeInTheDocument();
});

test('a generic identity with a non-http scheme URL is shown as text, never as an href', () => {
  const matrix: ExternalIdentity = {
    id: 'ei-2',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    entity_id: 'alice-uid',
    system: 'matrix',
    external_id: 'room-42',
    url: 'mailto:room@example.com',
    metadata: {},
    sync_status: 'idle',
  };
  renderPanel({ identities: [matrix] });

  expect(screen.queryByRole('link')).not.toBeInTheDocument();
  expect(screen.getByText('mailto:room@example.com')).toBeInTheDocument();
});

test('generic non-Immich identities render under other integrations', () => {
  const matrix: ExternalIdentity = {
    id: 'ei-2',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    entity_id: 'alice-uid',
    system: 'matrix',
    external_id: 'room-42',
    url: 'https://matrix.example/rooms/42',
    metadata: {},
    sync_status: 'idle',
  };
  renderPanel({ identities: [identity, matrix] });

  // The Immich row is rich; the matrix identity shows in the generic list.
  expect(screen.getByText('Other integrations')).toBeInTheDocument();
  expect(screen.getByText('matrix')).toBeInTheDocument();
  expect(screen.getByText('room-42')).toBeInTheDocument();
});

test('a file-system identity renders in the file links surface with its metadata', () => {
  const paperless: ExternalIdentity = {
    id: 'ei-2',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    entity_id: 'alice-uid',
    system: 'paperless',
    external_id: '42',
    url: 'https://paperless.example/documents/42/details',
    metadata: { title: 'Signed Contract' },
    sync_status: 'idle',
  };
  renderPanel({ identities: [paperless] });

  // The paperless identity renders as a file-link row (title from metadata),
  // NOT in the generic "Other integrations" list.
  expect(screen.getByText('Signed Contract')).toBeInTheDocument();
  expect(screen.queryByText('Other integrations')).not.toBeInTheDocument();
  expect(screen.getByRole('link', { name: /Open in Paperless-ngx/ })).toHaveAttribute(
    'href',
    'https://paperless.example/documents/42/details'
  );
});
