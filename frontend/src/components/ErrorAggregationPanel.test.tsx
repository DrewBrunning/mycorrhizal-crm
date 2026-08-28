import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { type ErrorBucket, getErrorAggregation } from '../api/errorAggregation';
import ErrorAggregationPanel from './ErrorAggregationPanel';

// This codebase's vitest setup has no auto-cleanup and no globals: true.
afterEach(cleanup);

vi.mock('../api/errorAggregation', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/errorAggregation')>();
  return { ...actual, getErrorAggregation: vi.fn() };
});

function bucket(overrides: Partial<ErrorBucket> = {}): ErrorBucket {
  return {
    component: 'contact_sync',
    cause: 'carddav authentication failed (http <n>)',
    sample_error: 'CardDAV authentication failed (HTTP 401)',
    event_types: ['sync_failed'],
    count: 17,
    recurring: true,
    first_seen: '2026-08-27T09:00:00Z',
    last_seen: '2026-08-27T11:30:00Z',
    event_ids: [1, 2, 3],
    event_ids_truncated: false,
    ...overrides,
  };
}

function resp(buckets: ErrorBucket[]) {
  return {
    window_hours: 24,
    since: '2026-08-26T12:00:00Z',
    until: '2026-08-27T12:00:00Z',
    total_events: buckets.reduce((n, b) => n + b.count, 0),
    buckets,
  };
}

beforeEach(() => {
  vi.mocked(getErrorAggregation).mockReset();
});

test('renders one row per cause with its count and sample error', async () => {
  vi.mocked(getErrorAggregation).mockResolvedValue(
    resp([
      bucket(),
      bucket({
        component: 'notification',
        cause: 'smtp timeout',
        sample_error: 'SMTP timeout',
        event_types: ['notification_failed'],
        count: 1,
        recurring: false,
        event_ids: [9],
      }),
    ]),
  );

  render(<ErrorAggregationPanel onViewEvents={vi.fn()} />);

  await waitFor(() => expect(getErrorAggregation).toHaveBeenCalled());
  expect(screen.getByText('CardDAV authentication failed (HTTP 401)')).toBeInTheDocument();
  expect(screen.getByText('SMTP timeout')).toBeInTheDocument();
  expect(screen.getByText('17')).toBeInTheDocument();
  // The recurring one is badged; the single transient one is not.
  expect(screen.getByText('Recurring')).toBeInTheDocument();
});

test('"View N events" hands the bucket\'s exact ids to the parent', async () => {
  const onViewEvents = vi.fn();
  vi.mocked(getErrorAggregation).mockResolvedValue(
    resp([bucket({ event_ids: [11, 12, 13], count: 3 })]),
  );

  render(<ErrorAggregationPanel onViewEvents={onViewEvents} />);

  fireEvent.click(await screen.findByRole('button', { name: 'View 3 events' }));
  expect(onViewEvents).toHaveBeenCalledWith([11, 12, 13]);
});

test('shows the empty state when there are no buckets', async () => {
  vi.mocked(getErrorAggregation).mockResolvedValue(resp([]));

  render(<ErrorAggregationPanel onViewEvents={vi.fn()} />);

  await waitFor(() =>
    expect(screen.getByText(/No repeated errors in the last 24h\./)).toBeInTheDocument(),
  );
});
