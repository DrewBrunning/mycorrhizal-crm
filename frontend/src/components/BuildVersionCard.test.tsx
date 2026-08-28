import { cleanup, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';
import '../i18n/config';
import { formatBuildVersion } from '../api/health';
import { isAdmin } from '../auth';
import { SnackbarProvider } from '../context/SnackbarContext';
import BuildVersionCard from './BuildVersionCard';

// This codebase's vitest setup has no auto-cleanup and no globals: true
// (CLAUDE.md frontend trap #1).
vi.mock('../auth', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../auth')>();
  return { ...actual, isAdmin: vi.fn() };
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.mocked(isAdmin).mockReset();
});

function renderCard() {
  return render(
    <SnackbarProvider>
      <BuildVersionCard />
    </SnackbarProvider>,
  );
}

const healthPayload = {
  status: 'healthy',
  timestamp: '2026-08-04T18:00:00Z',
  database: { status: 'healthy', response_time_ms: 1 },
  version: 'v0.2.0-alpha2',
  commit: 'abc1234',
  build_date: '2026-08-04T17:00:00Z',
};

// Stub global fetch with URL-aware responses: /health returns healthData, the
// admin system-status endpoint returns systemStatusData (issue #650's chip).
function mockFetch(healthData: unknown, systemStatusData?: unknown) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('/admin/system-status')) {
        return { ok: true, json: async () => systemStatusData, status: 200 };
      }
      return { ok: true, json: async () => healthData, status: 200 };
    }),
  );
}

test('shows the running build version and commit', async () => {
  vi.mocked(isAdmin).mockReturnValue(false);
  mockFetch(healthPayload);
  renderCard();

  await waitFor(() => expect(screen.getByText('v0.2.0-alpha2 (abc1234)')).toBeInTheDocument());
  expect(screen.getByText('Include this when reporting a problem.')).toBeInTheDocument();
});

test('falls back to the bare version when no commit is stamped', async () => {
  vi.mocked(isAdmin).mockReturnValue(false);
  mockFetch({ ...healthPayload, commit: undefined, version: 'dev' });
  renderCard();

  await waitFor(() => expect(screen.getByText('dev')).toBeInTheDocument());
});

// The version display is informational — a failed lookup must not take over
// the settings page it renders on.
test('renders a dash rather than throwing when /health fails', async () => {
  vi.mocked(isAdmin).mockReturnValue(false);
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => {
      throw new Error('network down');
    }),
  );
  renderCard();

  await waitFor(() => expect(screen.getByText('—')).toBeInTheDocument());
  expect(screen.getByText('About')).toBeInTheDocument();
});

// The update-availability chip (issue #650) is admin-only and informational.

test('an admin sees the update-available chip when the check reports one', async () => {
  vi.mocked(isAdmin).mockReturnValue(true);
  mockFetch(healthPayload, {
    update: { enabled: true, current: 'v0.2.0-alpha2', latest: 'v9.9.9', update_available: true },
  });
  renderCard();

  expect(await screen.findByText('Update available: v9.9.9')).toBeInTheDocument();
});

test('an admin on the latest release sees no chip', async () => {
  vi.mocked(isAdmin).mockReturnValue(true);
  mockFetch(healthPayload, {
    update: { enabled: true, current: 'v0.2.0-alpha2', latest: 'v0.2.0-alpha2' },
  });
  renderCard();

  await waitFor(() => expect(screen.getByText('v0.2.0-alpha2 (abc1234)')).toBeInTheDocument());
  expect(screen.queryByText(/Update available:/)).not.toBeInTheDocument();
});

test('a disabled or failed update check shows no chip', async () => {
  vi.mocked(isAdmin).mockReturnValue(true);
  mockFetch(healthPayload, { update: { enabled: false } });
  renderCard();

  await waitFor(() => expect(screen.getByText('v0.2.0-alpha2 (abc1234)')).toBeInTheDocument());
  expect(screen.queryByText(/Update available:/)).not.toBeInTheDocument();
});

test('a non-admin never requests the admin system-status endpoint', async () => {
  vi.mocked(isAdmin).mockReturnValue(false);
  const fetchMock = vi.fn(async (_input: RequestInfo | URL) => ({
    ok: true,
    json: async () => healthPayload,
    status: 200,
  }));
  vi.stubGlobal('fetch', fetchMock);
  renderCard();

  await waitFor(() => expect(screen.getByText('v0.2.0-alpha2 (abc1234)')).toBeInTheDocument());
  expect(screen.queryByText(/Update available:/)).not.toBeInTheDocument();
  const urls = fetchMock.mock.calls.map((args) => String(args[0]));
  expect(urls.some((u) => u.includes('/admin/system-status'))).toBe(false);
});

describe('formatBuildVersion', () => {
  test('combines version and commit', () => {
    expect(formatBuildVersion({ version: 'v1.2.3', commit: 'deadbee' })).toBe('v1.2.3 (deadbee)');
  });

  test('returns the version alone without a commit', () => {
    expect(formatBuildVersion({ version: 'v1.2.3' })).toBe('v1.2.3');
  });

  // A dirty tree means the commit alone does not identify the source; the
  // suffix must survive to the display so a report says so.
  test('preserves the dirty marker', () => {
    expect(formatBuildVersion({ version: 'dev', commit: 'abc123-dirty' })).toBe(
      'dev (abc123-dirty)',
    );
  });
});
