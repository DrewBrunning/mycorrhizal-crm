import { cleanup, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import {
  getNotificationChannelHealth,
  type NotificationChannelHealth,
} from '../api/notificationHealth';
import NotificationHealthPanel from './NotificationHealthPanel';

// This codebase's vitest setup has no auto-cleanup and no globals: true.
afterEach(cleanup);

vi.mock('../api/notificationHealth', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/notificationHealth')>();
  return { ...actual, getNotificationChannelHealth: vi.fn() };
});

function row(overrides: Partial<NotificationChannelHealth> = {}): NotificationChannelHealth {
  return {
    channel: 'ntfy',
    status: 'healthy',
    configured: true,
    reachable: true,
    enabled_user_count: 1,
    device_count: 0,
    fcm_configured: false,
    last_attempt_at: null,
    last_sent_at: null,
    last_failed_at: null,
    consecutive_failures: 0,
    last_error: '',
    attempted_count: 0,
    delivered_count: 0,
    ...overrides,
  };
}

beforeEach(() => {
  vi.mocked(getNotificationChannelHealth).mockReset();
});

test('renders one card per channel with its status', async () => {
  vi.mocked(getNotificationChannelHealth).mockResolvedValue({
    channels: [
      row({ channel: 'email', status: 'healthy' }),
      row({ channel: 'push', status: 'no_devices' }),
    ],
  });

  render(<NotificationHealthPanel />);

  await waitFor(() => expect(getNotificationChannelHealth).toHaveBeenCalled());
  expect(screen.getByText('Email')).toBeInTheDocument();
  expect(screen.getByText('Web Push')).toBeInTheDocument();
  expect(screen.getByText('Healthy')).toBeInTheDocument();
  expect(screen.getByText('No devices')).toBeInTheDocument();
});

test('shows the consecutive-failure count, last error, and delivery totals when failing', async () => {
  vi.mocked(getNotificationChannelHealth).mockResolvedValue({
    channels: [
      row({
        channel: 'gotify',
        status: 'failing',
        consecutive_failures: 9,
        last_failed_at: '2026-08-27T17:40:00Z',
        last_error: 'HTTP 401',
        attempted_count: 12,
        delivered_count: 3,
      }),
    ],
  });

  render(<NotificationHealthPanel />);

  await waitFor(() => expect(getNotificationChannelHealth).toHaveBeenCalled());
  expect(screen.getByText('Consecutive failures: 9')).toBeInTheDocument();
  expect(screen.getByText('HTTP 401')).toBeInTheDocument();
  expect(screen.getByText(/3 \/ 12/)).toBeInTheDocument();
});

test('shows the device count and FCM hint for a push channel', async () => {
  vi.mocked(getNotificationChannelHealth).mockResolvedValue({
    channels: [
      row({
        channel: 'push',
        status: 'healthy',
        device_count: 2,
        fcm_configured: true,
      }),
    ],
  });

  render(<NotificationHealthPanel />);

  await waitFor(() => expect(getNotificationChannelHealth).toHaveBeenCalled());
  expect(screen.getByText('Devices: 2')).toBeInTheDocument();
  expect(screen.getByText('FCM configured')).toBeInTheDocument();
});

test('surfaces a load error', async () => {
  vi.mocked(getNotificationChannelHealth).mockRejectedValue(new Error('the backend is down'));

  render(<NotificationHealthPanel />);

  await waitFor(() => expect(screen.getByText('the backend is down')).toBeInTheDocument());
});
