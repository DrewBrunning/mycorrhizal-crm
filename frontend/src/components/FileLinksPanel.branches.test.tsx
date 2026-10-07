// Issue #1482: remaining FileLinksPanel branches (per-system rows, unlink
// paths, callback wiring). Complements FileLinksPanel.test.tsx.
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { ExternalIdentity } from '../api/externalLinks';
import { DateFormatProvider } from '../DateFormatProvider';
import FileLinksPanel from './FileLinksPanel';

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

const seafileFile: ExternalIdentity = {
  ...identity,
  id: 'ei-s',
  system: 'seafile',
  external_id: 'repo:/a.txt',
  url: undefined,
  metadata: { name: 'a.txt', size: 1536, modified_at: '2026-04-01T00:00:00Z', type: 'file' },
};

function renderPanel(overrides: Partial<React.ComponentProps<typeof FileLinksPanel>> = {}) {
  const props: React.ComponentProps<typeof FileLinksPanel> = {
    identities: [],
    configured: { paperless: false, seafile: false, nextcloud: false },
    ...overrides,
  };
  return render(
    <DateFormatProvider>
      <FileLinksPanel {...props} />
    </DateFormatProvider>,
  );
}

test('a seafile file row shows name, formatted size, modified date and system', () => {
  renderPanel({ identities: [seafileFile] });
  expect(screen.getByText('a.txt')).toBeInTheDocument();
  expect(screen.getByText(/1\.5 KB · Modified .* · seafile/)).toBeInTheDocument();
  expect(screen.queryByRole('link')).not.toBeInTheDocument();
});

test('formats byte, MB and GB sizes at the unit boundaries', () => {
  const mk = (id: string, size: number): ExternalIdentity => ({
    ...seafileFile,
    id,
    metadata: { name: id, size },
  });
  renderPanel({
    identities: [mk('b', 1023), mk('mb', 1024 * 1024), mk('gb', 3 * 1024 * 1024 * 1024)],
  });
  expect(screen.getByText('1023 B · seafile')).toBeInTheDocument();
  expect(screen.getByText('1.0 MB · seafile')).toBeInTheDocument();
  expect(screen.getByText('3.0 GB · seafile')).toBeInTheDocument();
});

test('a nextcloud directory row uses mtime (seconds) when no modified_at, and skips a zero size', () => {
  const dir: ExternalIdentity = {
    ...seafileFile,
    id: 'ei-n',
    system: 'nextcloud',
    metadata: { name: 'Docs', type: 'dir', size: 0, mtime: 1_775_000_000 },
  };
  renderPanel({ identities: [dir] });
  expect(screen.getByText('Docs')).toBeInTheDocument();
  expect(screen.getByText(/^Modified .* · nextcloud$/)).toBeInTheDocument();
});

test('a row with no cached title or name falls back to the external id and just the system', () => {
  renderPanel({
    identities: [
      { ...identity, metadata: undefined, url: undefined, id: 'ei-x', external_id: 'ext-99' },
    ],
  });
  expect(screen.getByText('ext-99')).toBeInTheDocument();
  expect(screen.getByText('paperless')).toBeInTheDocument();
});

test('a paperless row without created_at shows only the system', () => {
  renderPanel({ identities: [{ ...identity, metadata: { title: 'T' }, url: undefined }] });
  expect(screen.getByText('paperless')).toBeInTheDocument();
});

test('identities from other systems are not rendered by this panel', () => {
  renderPanel({
    identities: [{ ...identity, system: 'immich', id: 'ei-i', metadata: { title: 'Nope' } }],
  });
  expect(screen.queryByText('Nope')).not.toBeInTheDocument();
});

test('declining the unlink confirmation does not call onUnlink', () => {
  vi.stubGlobal(
    'confirm',
    vi.fn(() => false),
  );
  const onUnlink = vi.fn().mockResolvedValue(undefined);
  renderPanel({ identities: [identity], onUnlink });
  fireEvent.click(screen.getByRole('button', { name: 'Unlink' }));
  expect(onUnlink).not.toHaveBeenCalled();
});

test('unlink confirmation names the system', () => {
  const confirm = vi.fn(() => false);
  vi.stubGlobal('confirm', confirm);
  renderPanel({ identities: [seafileFile], onUnlink: vi.fn() });
  fireEvent.click(screen.getByRole('button', { name: 'Unlink' }));
  expect(confirm).toHaveBeenCalledWith('Remove this Seafile link from the contact?');
});

test('without an onUnlink handler the unlink button does nothing (not even a confirm)', () => {
  const confirm = vi.fn(() => true);
  vi.stubGlobal('confirm', confirm);
  renderPanel({ identities: [identity] });
  fireEvent.click(screen.getByRole('button', { name: 'Unlink' }));
  expect(confirm).not.toHaveBeenCalled();
});

test('an unlink failure shows the error message; a non-Error rejection shows the default text', async () => {
  vi.stubGlobal(
    'confirm',
    vi.fn(() => true),
  );
  const onUnlink = vi
    .fn()
    .mockRejectedValueOnce(new Error('server said no'))
    .mockRejectedValueOnce('boom');
  renderPanel({ identities: [identity], onUnlink });
  fireEvent.click(screen.getByRole('button', { name: 'Unlink' }));
  expect(await screen.findByText('server said no')).toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'Unlink' }));
  expect(await screen.findByText('Could not unlink.')).toBeInTheDocument();
});

test('a later successful unlink clears the previous error', async () => {
  vi.stubGlobal(
    'confirm',
    vi.fn(() => true),
  );
  const onUnlink = vi
    .fn()
    .mockRejectedValueOnce(new Error('first'))
    .mockResolvedValueOnce(undefined);
  renderPanel({ identities: [identity], onUnlink });
  fireEvent.click(screen.getByRole('button', { name: 'Unlink' }));
  expect(await screen.findByText('first')).toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'Unlink' }));
  await waitFor(() => expect(screen.queryByText('first')).not.toBeInTheDocument());
});

test('half-wired systems (configured but missing callbacks) show no add-link button', () => {
  renderPanel({
    configured: { paperless: true, seafile: true, nextcloud: true },
    onFetchSeafileLibraries: vi.fn(),
    onFetchSeafileDir: undefined,
    onLinkSeafile: vi.fn(),
    onFetchNextcloudDir: vi.fn(),
    onLinkNextcloud: undefined,
  });
  expect(screen.queryByRole('button', { name: /^Link / })).not.toBeInTheDocument();
});

test('seafile and nextcloud add-link buttons open their pickers and browse', async () => {
  const onFetchSeafileLibraries = vi.fn().mockResolvedValue([]);
  const onFetchNextcloudDir = vi.fn().mockResolvedValue([]);
  renderPanel({
    configured: { paperless: false, seafile: true, nextcloud: true },
    onFetchSeafileLibraries,
    onFetchSeafileDir: vi.fn().mockResolvedValue([]),
    onLinkSeafile: vi.fn().mockResolvedValue(undefined),
    onFetchNextcloudDir,
    onLinkNextcloud: vi.fn().mockResolvedValue(undefined),
  });
  fireEvent.click(screen.getByRole('button', { name: 'Link Seafile' }));
  await waitFor(() => expect(onFetchSeafileLibraries).toHaveBeenCalled());
  expect(onFetchNextcloudDir).not.toHaveBeenCalled();
  fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' });
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: 'Link Nextcloud / ownCloud' }));
  await waitFor(() => expect(onFetchNextcloudDir).toHaveBeenCalled());
});
