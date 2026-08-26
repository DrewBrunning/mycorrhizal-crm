import { cleanup, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';
import '../i18n/config';
import { formatBuildVersion } from '../api/health';
import { SnackbarProvider } from '../context/SnackbarContext';
import BuildVersionCard from './BuildVersionCard';

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function renderCard() {
  return render(
    <SnackbarProvider>
      <BuildVersionCard />
    </SnackbarProvider>,
  );
}

function mockHealth(data: unknown, ok = true) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => ({ ok, json: async () => data, status: ok ? 200 : 500 })),
  );
}

test('shows the running build version and commit', async () => {
  mockHealth({
    status: 'healthy',
    timestamp: '2026-08-04T18:00:00Z',
    database: { status: 'healthy', response_time_ms: 1 },
    version: 'v0.2.0-alpha2',
    commit: 'abc1234',
    build_date: '2026-08-04T17:00:00Z',
  });
  renderCard();

  await waitFor(() => expect(screen.getByText('v0.2.0-alpha2 (abc1234)')).toBeInTheDocument());
  expect(screen.getByText('Include this when reporting a problem.')).toBeInTheDocument();
});

test('falls back to the bare version when no commit is stamped', async () => {
  mockHealth({
    status: 'healthy',
    timestamp: '2026-08-04T18:00:00Z',
    database: { status: 'healthy', response_time_ms: 1 },
    version: 'dev',
  });
  renderCard();

  await waitFor(() => expect(screen.getByText('dev')).toBeInTheDocument());
});

// The version display is informational — a failed lookup must not take over
// the settings page it renders on.
test('renders a dash rather than throwing when /health fails', async () => {
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
