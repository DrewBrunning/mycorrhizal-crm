import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import './i18n/config';
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

test('shows the empty state when the API returns nothing', async () => {
  renderPage();
  await waitFor(() =>
    expect(screen.getByText('No system events recorded yet.')).toBeInTheDocument(),
  );
});
