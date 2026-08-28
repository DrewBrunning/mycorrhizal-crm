import { cleanup, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { getSystemStatus, type SystemStatusResponse } from './api/systemStatus';
import { isAdmin } from './auth';
import './i18n/config';
import SystemStatusPage from './SystemStatusPage';

// This codebase's vitest setup has no auto-cleanup and no globals: true
// (CLAUDE.md frontend trap #1).
afterEach(cleanup);

vi.mock('./api/systemStatus', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/systemStatus')>();
  return { ...actual, getSystemStatus: vi.fn() };
});
vi.mock('./auth', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./auth')>();
  return { ...actual, isAdmin: vi.fn() };
});

const getMock = vi.mocked(getSystemStatus);

function fullStatus(overrides: Partial<SystemStatusResponse> = {}): SystemStatusResponse {
  return {
    overall: 'degraded',
    health: {
      status: 'degraded',
      database: { status: 'ok' },
      migrations: { status: 'ok' },
      integrity_check: { status: 'ok' },
      restore_drill: { status: 'not_configured' },
      background_jobs: { status: 'degraded', reason: 'stuck job lock(s): reminders' },
      integrations: {
        email: { status: 'ok' },
        oidc: { status: 'degraded', reason: 'unreachable' },
      },
    },
    version: { version: 'v0.6.2', commit: 'abc123', build_date: '2026-08-27T00:00:00Z' },
    uptime: { started_at: '2026-08-27T10:00:00Z', uptime_seconds: 90061 },
    migration: { applied: 42, latest: 43, pending: 1, dirty: false },
    config: {
      validation: [{ field: 'SMTP_HOST', message: 'SMTP is enabled but SMTP_HOST is empty' }],
      features: {
        carddav: true,
        caldav: true,
        oidc: false,
        email: true,
        metrics: false,
        db_integrity_check: true,
        db_restore_drill: false,
      },
    },
    database: { sqlite_version: '3.45.1', journal_mode: 'wal', wal_bytes: 4096 },
    storage: {
      database_bytes: 1048576,
      filesystem: { free_bytes: 68719476736, total_bytes: 107374182400 },
      directories: [
        { path: '/app/static/photos', bytes: 2048, file_count: 12, truncated: false },
        { path: '/app/data/attachments', bytes: 2097152, file_count: 300, truncated: true },
      ],
    },
    // Default: the update-availability flag is off (UPDATE_CHECK_ENABLED unset),
    // so the block is exactly {enabled: false} and nothing is rendered.
    update: { enabled: false },
    ...overrides,
  };
}

beforeEach(() => {
  vi.mocked(isAdmin).mockReset();
  vi.mocked(isAdmin).mockReturnValue(true);
  getMock.mockReset();
  getMock.mockResolvedValue(fullStatus());
});

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/system-status']}>
      <Routes>
        <Route path="/system-status" element={<SystemStatusPage />} />
        <Route path="/" element={<div>DASHBOARD PAGE</div>} />
      </Routes>
    </MemoryRouter>,
  );
}

test('renders every section from a fully-populated payload', async () => {
  renderPage();

  // Overall status chip (degraded -> warning) + title. The same "Degraded"
  // label also appears in the health-facet table, so match any instance.
  expect(await screen.findByText('System status')).toBeInTheDocument();
  expect(screen.getByText('Overall status')).toBeInTheDocument();
  expect(screen.getAllByText('Degraded').length).toBeGreaterThan(0);

  // Version & uptime.
  expect(screen.getByText('v0.6.2')).toBeInTheDocument();
  expect(screen.getByText('abc123')).toBeInTheDocument();
  expect(screen.getByText('2026-08-27T00:00:00Z')).toBeInTheDocument();
  expect(screen.getByText('1d 1h 1m 1s')).toBeInTheDocument();

  // Health facets: statuses + reasons.
  expect(screen.getByText('Health checks')).toBeInTheDocument();
  expect(screen.getAllByText('OK').length).toBeGreaterThan(0);
  expect(screen.getByText('Not configured')).toBeInTheDocument();
  expect(screen.getByText('stuck job lock(s): reminders')).toBeInTheDocument();
  expect(screen.getByText('email')).toBeInTheDocument();
  expect(screen.getByText('oidc')).toBeInTheDocument();
  expect(screen.getByText('unreachable')).toBeInTheDocument();

  // Migration ("Migrations" is also a health-facet row label, so match any).
  expect(screen.getAllByText('Migrations').length).toBeGreaterThan(0);
  expect(screen.getByText('42')).toBeInTheDocument();
  expect(screen.getByText('43')).toBeInTheDocument();
  expect(screen.getByText('1')).toBeInTheDocument();
  expect(screen.getByText('No')).toBeInTheDocument();

  // Configuration: failing validation row in error colour + feature chips.
  expect(screen.getByText('SMTP_HOST')).toBeInTheDocument();
  expect(screen.getByText('SMTP is enabled but SMTP_HOST is empty')).toBeInTheDocument();
  expect(screen.getByText('CardDAV · On')).toBeInTheDocument();
  expect(screen.getByText('OIDC · Off')).toBeInTheDocument();

  // Database.
  expect(screen.getByText('3.45.1')).toBeInTheDocument();
  expect(screen.getByText('wal')).toBeInTheDocument();
  expect(screen.getByText('4.0 kB')).toBeInTheDocument();

  // Storage: sizes, filesystem bar, and the directories list with the
  // explicit "approx" marker on the truncated entry.
  expect(screen.getByText('1.0 MB')).toBeInTheDocument();
  expect(screen.getByText('36.0 GB of 100.0 GB used')).toBeInTheDocument();
  expect(screen.getByText('Free: 64.0 GB')).toBeInTheDocument();
  expect(screen.getByText('/app/static/photos')).toBeInTheDocument();
  expect(screen.getByText('/app/data/attachments')).toBeInTheDocument();
  expect(screen.getByText('2.0 MB')).toBeInTheDocument();
  expect(screen.getByText('approx.')).toBeInTheDocument();
});

test('omitted directories and an empty validation list render the pass state, not a crash', async () => {
  getMock.mockResolvedValue(
    fullStatus({
      config: { ...fullStatus().config, validation: [] },
      storage: { ...fullStatus().storage, directories: [] },
    }),
  );

  renderPage();

  expect(await screen.findByText('System status')).toBeInTheDocument();
  // The "all checks pass" state, not a validation table.
  expect(screen.getByText('All configuration checks pass.')).toBeInTheDocument();
  expect(screen.queryByText('SMTP_HOST')).not.toBeInTheDocument();
  // Directories are only rendered when present.
  expect(screen.queryByText('/app/static/photos')).not.toBeInTheDocument();
  // The page survived: sections that were populated still render.
  expect(screen.getByText('v0.6.2')).toBeInTheDocument();
});

test('a missing section degrades to em dashes instead of throwing', async () => {
  getMock.mockResolvedValue(
    fullStatus({
      version: { version: 'v0.6.2' },
      uptime: { started_at: '2026-08-27T10:00:00Z', uptime_seconds: 12 },
      database: { sqlite_version: '', journal_mode: '', wal_bytes: 0 },
      storage: {
        database_bytes: 0,
        filesystem: { free_bytes: 0, total_bytes: 0 },
        directories: [],
      },
    }),
  );

  renderPage();

  expect(await screen.findByText('System status')).toBeInTheDocument();
  // Empty strings and zero-byte facts render dashes, never raw blanks.
  expect(screen.getAllByText('—').length).toBeGreaterThan(0);
});

test('a non-admin visiting the page is redirected away', async () => {
  vi.mocked(isAdmin).mockReturnValue(false);

  renderPage();

  await waitFor(() => expect(screen.getByText('DASHBOARD PAGE')).toBeInTheDocument());
  expect(screen.queryByText('Overall status')).not.toBeInTheDocument();
});

test('an update-available block renders the update card and chip', async () => {
  getMock.mockResolvedValue(
    fullStatus({
      update: {
        enabled: true,
        current: 'v0.6.2',
        latest: 'v9.9.9',
        update_available: true,
        checked_at: '2026-08-28T10:00:00Z',
      },
    }),
  );

  renderPage();

  expect(await screen.findByText('Update check')).toBeInTheDocument();
  expect(screen.getByText('Update available: v9.9.9')).toBeInTheDocument();
  // "v0.6.2" also appears in the Version & uptime section, so match any.
  expect(screen.getAllByText('v0.6.2').length).toBeGreaterThan(0);
  expect(screen.getAllByText('v9.9.9').length).toBeGreaterThan(0);
  // checked_at renders through toLocaleString, so assert the card is present
  // rather than a specific locale-dependent rendering.
  expect(screen.getByText('Checked at')).toBeInTheDocument();
});

test('an enabled but up-to-date update block renders the release line, not a chip', async () => {
  getMock.mockResolvedValue(
    fullStatus({
      update: { enabled: true, current: 'v0.6.2', latest: 'v0.6.2', update_available: false },
    }),
  );

  renderPage();

  expect(await screen.findByText('Update check')).toBeInTheDocument();
  expect(screen.getByText('You are running the latest release.')).toBeInTheDocument();
  expect(screen.queryByText(/Update available:/)).not.toBeInTheDocument();
});

test('a disabled update check renders no update section', async () => {
  getMock.mockResolvedValue(fullStatus({ update: { enabled: false } }));

  renderPage();

  expect(await screen.findByText('System status')).toBeInTheDocument();
  expect(screen.queryByText('Update check')).not.toBeInTheDocument();
  expect(screen.queryByText(/Update available:/)).not.toBeInTheDocument();
});

test('an enabled update check with unknown latest renders no update section', async () => {
  getMock.mockResolvedValue(fullStatus({ update: { enabled: true, current: 'v0.6.2' } }));

  renderPage();

  expect(await screen.findByText('System status')).toBeInTheDocument();
  expect(screen.queryByText('Update check')).not.toBeInTheDocument();
  expect(screen.queryByText(/Update available:/)).not.toBeInTheDocument();
});
