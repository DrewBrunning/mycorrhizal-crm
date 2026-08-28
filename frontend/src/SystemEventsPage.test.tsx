import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import './i18n/config';
import { getErrorAggregation } from './api/errorAggregation';
import { getNotificationChannelHealth } from './api/notificationHealth';
import { getSubsystemHealth } from './api/subsystemHealth';
import { getSystemEvents, type SystemEvent } from './api/systemEvents';
import SystemEventsPage from './SystemEventsPage';

// This codebase's vitest has no auto-cleanup (CLAUDE.md frontend trap #1).
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

vi.mock('./api/systemEvents', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/systemEvents')>();
  return { ...actual, getSystemEvents: vi.fn() };
});

vi.mock('./api/subsystemHealth', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/subsystemHealth')>();
  return { ...actual, getSubsystemHealth: vi.fn() };
});

vi.mock('./api/errorAggregation', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/errorAggregation')>();
  return { ...actual, getErrorAggregation: vi.fn() };
});

vi.mock('./api/notificationHealth', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/notificationHealth')>();
  return { ...actual, getNotificationChannelHealth: vi.fn() };
});

const getMock = vi.mocked(getSystemEvents);

function ev(overrides: Partial<SystemEvent>): SystemEvent {
  return {
    id: 1,
    created_at: '2026-08-27T12:00:00Z',
    occurred_at: '2026-08-27T12:00:00Z',
    event_type: 'sync_completed',
    severity: 'info',
    component: 'contact_sync',
    correlation_id: 'chain-A',
    ...overrides,
  };
}

beforeEach(() => {
  getMock.mockReset();
  getMock.mockResolvedValue({ system_events: [], total: 0 });
  vi.mocked(getSubsystemHealth).mockReset();
  vi.mocked(getSubsystemHealth).mockResolvedValue({ subsystems: [] });
  vi.mocked(getErrorAggregation).mockReset();
  vi.mocked(getErrorAggregation).mockResolvedValue({
    window_hours: 24,
    since: '2026-08-26T12:00:00Z',
    until: '2026-08-27T12:00:00Z',
    total_events: 0,
    buckets: [],
  });
  vi.mocked(getNotificationChannelHealth).mockReset();
  vi.mocked(getNotificationChannelHealth).mockResolvedValue({ channels: [] });
});

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/system-events']}>
      <SystemEventsPage />
    </MemoryRouter>,
  );
}

test('renders rows from the API newest-first as returned', async () => {
  getMock.mockResolvedValue({
    system_events: [
      ev({
        id: 2,
        event_type: 'sync_failed',
        severity: 'error',
        result: 'failure',
        correlation_id: 'chain-A',
      }),
      ev({
        id: 1,
        event_type: 'job_completed',
        component: 'scheduler',
        result: 'success',
        correlation_id: 'chain-A',
      }),
    ],
    total: 2,
  });

  renderPage();

  await waitFor(() => expect(screen.getByText('Sync failed')).toBeInTheDocument());
  expect(screen.getByText('Job completed')).toBeInTheDocument();
});

test('component filter is sent to the API', async () => {
  renderPage();
  await waitFor(() => expect(getMock).toHaveBeenCalled());

  fireEvent.mouseDown(screen.getByLabelText('Component'));
  fireEvent.click(within(screen.getByRole('listbox')).getByText('scheduler'));

  await waitFor(() =>
    expect(getMock).toHaveBeenLastCalledWith(expect.objectContaining({ component: 'scheduler' })),
  );
});

test('"View related" queries by correlation_id, drops other filters, and shows the chain banner', async () => {
  getMock.mockResolvedValue({
    system_events: [
      ev({ id: 5, correlation_id: 'chain-XYZ', event_type: 'sync_failed', severity: 'error' }),
    ],
    total: 1,
  });

  renderPage();
  await waitFor(() => expect(screen.getByText('Sync failed')).toBeInTheDocument());

  // Narrow to a component first — View related must then show the whole chain,
  // not the chain intersected with the stale component filter.
  fireEvent.mouseDown(screen.getByLabelText('Component'));
  fireEvent.click(within(screen.getByRole('listbox')).getByText('scheduler'));
  await waitFor(() =>
    expect(getMock).toHaveBeenLastCalledWith(expect.objectContaining({ component: 'scheduler' })),
  );

  fireEvent.click(screen.getByRole('button', { name: 'Details' }));
  fireEvent.click(screen.getByRole('button', { name: 'View related' }));

  await waitFor(() =>
    expect(getMock).toHaveBeenLastCalledWith(
      expect.objectContaining({ correlation_id: 'chain-XYZ', component: undefined }),
    ),
  );
  expect(screen.getByText(/chain-XYZ/)).toBeInTheDocument();
});

test('"View N events" on an error bucket queries the timeline by those ids and shows the banner', async () => {
  vi.mocked(getErrorAggregation).mockResolvedValue({
    window_hours: 24,
    since: '2026-08-26T12:00:00Z',
    until: '2026-08-27T12:00:00Z',
    total_events: 9,
    buckets: [
      {
        component: 'contact_sync',
        cause: 'carddav auth rejected (http <n>)',
        sample_error: 'CardDAV auth rejected (HTTP 401)',
        event_types: ['sync_failed'],
        count: 9,
        recurring: true,
        first_seen: '2026-08-27T09:00:00Z',
        last_seen: '2026-08-27T11:00:00Z',
        event_ids: [11, 12, 13],
        event_ids_truncated: false,
      },
    ],
  });

  renderPage();
  await waitFor(() => expect(getMock).toHaveBeenCalled());

  fireEvent.click(await screen.findByRole('button', { name: 'View 3 events' }));

  await waitFor(() =>
    expect(getMock).toHaveBeenLastCalledWith(expect.objectContaining({ ids: [11, 12, 13] })),
  );
  expect(screen.getByText(/Showing 3 events/)).toBeInTheDocument();
});

test('shows the empty state when the API returns nothing', async () => {
  renderPage();
  await waitFor(() =>
    expect(screen.getByText('No system events recorded yet.')).toBeInTheDocument(),
  );
});
