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

test('generic non-Immich identities render under other integrations', () => {
  const paperless: ExternalIdentity = {
    id: 'ei-2',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    entity_id: 'alice-uid',
    system: 'paperless',
    external_id: 'doc-42',
    url: 'https://paperless.example/documents/42',
    metadata: {},
    sync_status: 'idle',
  };
  renderPanel({ identities: [identity, paperless] });

  // The Immich row is rich; the paperless identity shows in the generic list.
  expect(screen.getByText('Other integrations')).toBeInTheDocument();
  expect(screen.getByText('paperless')).toBeInTheDocument();
  expect(screen.getByText('doc-42')).toBeInTheDocument();
});
