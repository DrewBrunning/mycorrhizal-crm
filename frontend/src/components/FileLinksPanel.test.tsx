import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent, waitFor } from '@testing-library/react';
import '../i18n/config';
import FileLinksPanel from './FileLinksPanel';
import { DateFormatProvider } from '../DateFormatProvider';
import { ExternalIdentity } from '../api/externalLinks';
import { PaperlessDocument } from '../api/paperless';

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const identity: ExternalIdentity = {
  id: 'ei-1',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  entity_id: 'alice-uid',
  system: 'paperless',
  external_id: '42',
  url: 'https://paperless.example/documents/42/details',
  metadata: { title: 'Signed Contract', created_at: '2026-03-01' },
  sync_status: 'idle',
};

const paperlessDoc: PaperlessDocument = { id: 7, title: 'Passport', file_name: 'passport.pdf' };

function renderPanel(overrides: Partial<React.ComponentProps<typeof FileLinksPanel>> = {}) {
  const defaults: React.ComponentProps<typeof FileLinksPanel> = {
    identities: [],
    configured: { paperless: false, seafile: false, nextcloud: false },
    onFetchPaperlessDocuments: vi.fn().mockResolvedValue([paperlessDoc]),
    onLinkPaperless: vi.fn().mockResolvedValue(undefined),
    onUnlink: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };
  return render(
    <DateFormatProvider>
      <FileLinksPanel {...defaults} />
    </DateFormatProvider>
  );
}

test('unconfigured systems show no add-link affordance', () => {
  renderPanel();
  expect(screen.queryByText(/Link Paperless-ngx/)).not.toBeInTheDocument();
  expect(screen.queryByText(/Link Seafile/)).not.toBeInTheDocument();
  expect(screen.queryByText(/Link Nextcloud/)).not.toBeInTheDocument();
});

test('a configured system with browse callbacks shows its add-link button', () => {
  renderPanel({
    configured: { paperless: true, seafile: false, nextcloud: false },
    onFetchPaperlessDocuments: vi.fn().mockResolvedValue([paperlessDoc]),
    onLinkPaperless: vi.fn().mockResolvedValue(undefined),
  });
  expect(screen.getByRole('button', { name: 'Link Paperless-ngx' })).toBeInTheDocument();
});

test('picking a document in the picker calls onLinkPaperless', async () => {
  const onLinkPaperless = vi.fn().mockResolvedValue(undefined);
  renderPanel({
    configured: { paperless: true, seafile: false, nextcloud: false },
    onFetchPaperlessDocuments: vi.fn().mockResolvedValue([paperlessDoc]),
    onLinkPaperless,
  });

  fireEvent.click(screen.getByRole('button', { name: 'Link Paperless-ngx' }));
  await waitFor(() => expect(screen.getByText('Passport')).toBeInTheDocument());

  fireEvent.click(screen.getByText('Passport'));
  await waitFor(() => expect(onLinkPaperless).toHaveBeenCalledWith(paperlessDoc));
});

test('a paperless identity renders with its cached title and a safe deep link', () => {
  renderPanel({
    identities: [identity],
    configured: { paperless: true, seafile: false, nextcloud: false },
  });
  expect(screen.getByText('Signed Contract')).toBeInTheDocument();
  expect(screen.getByRole('link', { name: /Open in Paperless-ngx/ })).toHaveAttribute(
    'href',
    'https://paperless.example/documents/42/details'
  );
});

test('an unsafe file-link URL is shown as text, never as an href', () => {
  renderPanel({
    identities: [{ ...identity, url: 'javascript:alert(1)' }],
  });
  expect(screen.queryByRole('link')).not.toBeInTheDocument();
  expect(screen.getByText('javascript:alert(1)')).toBeInTheDocument();
});

test('unlink asks for confirmation then fires onUnlink', async () => {
  vi.stubGlobal('confirm', vi.fn(() => true));
  const onUnlink = vi.fn().mockResolvedValue(undefined);
  renderPanel({ identities: [identity], onUnlink });

  fireEvent.click(screen.getByRole('button', { name: 'Unlink' }));
  await waitFor(() => expect(onUnlink).toHaveBeenCalledWith('paperless', identity.id));
});
