import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { getSubsystemHealth, type SubsystemHealth } from '../api/subsystemHealth';
import SubsystemHealthPanel from './SubsystemHealthPanel';

// This codebase's vitest setup has no auto-cleanup and no globals: true.
afterEach(cleanup);

vi.mock('../api/subsystemHealth', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/subsystemHealth')>();
  return { ...actual, getSubsystemHealth: vi.fn() };
});

function row(overrides: Partial<SubsystemHealth> = {}): SubsystemHealth {
  return {
    subsystem: 'contact_sync',
    status: 'unknown',
    last_attempt_at: null,
    last_success_at: null,
    last_failure_at: null,
    incident_first_failure_at: null,
    consecutive_failures: 0,
    last_error: '',
    ...overrides,
  };
}

beforeEach(() => {
  vi.mocked(getSubsystemHealth).mockReset();
});

test('renders one card per subsystem with its status', async () => {
  vi.mocked(getSubsystemHealth).mockResolvedValue({
    subsystems: [
      row({ subsystem: 'contact_sync', status: 'healthy' }),
      row({ subsystem: 'calendar_sync', status: 'unknown' }),
    ],
  });

  render(<SubsystemHealthPanel onSelectComponent={vi.fn()} />);

  await waitFor(() => expect(getSubsystemHealth).toHaveBeenCalled());
  expect(screen.getByText('CardDAV sync')).toBeInTheDocument();
  expect(screen.getByText('CalDAV sync')).toBeInTheDocument();
  expect(screen.getByText('Healthy')).toBeInTheDocument();
  expect(screen.getByText('Unknown')).toBeInTheDocument();
});

test('shows the consecutive-failure count, incident start, and last error when failing', async () => {
  vi.mocked(getSubsystemHealth).mockResolvedValue({
    subsystems: [
      row({
        subsystem: 'contact_sync',
        status: 'failing',
        consecutive_failures: 9,
        incident_first_failure_at: '2026-08-27T17:19:00Z',
        last_failure_at: '2026-08-27T17:40:00Z',
        last_error: 'carddav auth rejected',
      }),
    ],
  });

  render(<SubsystemHealthPanel onSelectComponent={vi.fn()} />);

  await waitFor(() => expect(getSubsystemHealth).toHaveBeenCalled());
  expect(screen.getByText('Consecutive failures: 9')).toBeInTheDocument();
  expect(screen.getByText(/Incident since/)).toBeInTheDocument();
  expect(screen.getByText('carddav auth rejected')).toBeInTheDocument();
});

test('clicking a card selects that subsystem as the component filter', async () => {
  const onSelect = vi.fn();
  vi.mocked(getSubsystemHealth).mockResolvedValue({
    subsystems: [row({ subsystem: 'notification', status: 'failing', consecutive_failures: 2 })],
  });

  render(<SubsystemHealthPanel onSelectComponent={onSelect} />);

  await waitFor(() => expect(getSubsystemHealth).toHaveBeenCalled());
  fireEvent.click(screen.getByTestId('subsystem-health-notification'));
  expect(onSelect).toHaveBeenCalledWith('notification');
});

test('surfaces a load error', async () => {
  vi.mocked(getSubsystemHealth).mockRejectedValue(new Error('the backend is down'));

  render(<SubsystemHealthPanel onSelectComponent={vi.fn()} />);

  await waitFor(() => expect(screen.getByText('the backend is down')).toBeInTheDocument());
});
