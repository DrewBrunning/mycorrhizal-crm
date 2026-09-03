import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import {
  createWebhook,
  deleteWebhook,
  getWebhookDeliveries,
  getWebhooks,
  testWebhook,
  updateWebhook,
  type Webhook,
} from '../api/webhooks';
import WebhooksSettings from './WebhooksSettings';

// This codebase's vitest setup has no auto-cleanup and no globals: true.
afterEach(cleanup);

vi.mock('../api/webhooks', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/webhooks')>();
  return {
    ...actual,
    getWebhooks: vi.fn(),
    createWebhook: vi.fn(),
    updateWebhook: vi.fn(),
    deleteWebhook: vi.fn(),
    testWebhook: vi.fn(),
    getWebhookDeliveries: vi.fn(),
  };
});

function webhook(overrides: Partial<Webhook> = {}): Webhook {
  return {
    id: 1,
    name: 'My webhook',
    url: 'https://example.com/hook',
    events: ['contact.created'],
    is_active: true,
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

beforeEach(() => {
  vi.mocked(getWebhooks).mockReset();
  vi.mocked(createWebhook).mockReset();
  vi.mocked(updateWebhook).mockReset();
  vi.mocked(deleteWebhook).mockReset();
  vi.mocked(testWebhook).mockReset();
  vi.mocked(getWebhookDeliveries).mockReset();
});

test('shows the empty state when no webhooks are configured', async () => {
  vi.mocked(getWebhooks).mockResolvedValue([]);
  render(<WebhooksSettings />);

  await waitFor(() => expect(screen.getByText('No webhooks configured yet.')).toBeInTheDocument());
});

test('lists a webhook with its active status and event count', async () => {
  vi.mocked(getWebhooks).mockResolvedValue([
    webhook({
      name: 'Inactive hook',
      is_active: false,
      events: ['contact.created', 'note.created'],
    }),
  ]);
  render(<WebhooksSettings />);

  await waitFor(() => expect(screen.getByText('Inactive hook')).toBeInTheDocument());
  expect(screen.getByText('Inactive')).toBeInTheDocument();
  expect(screen.getByText('2 events')).toBeInTheDocument();
});

test('creating a webhook posts the form and reveals the secret exactly once', async () => {
  vi.mocked(getWebhooks).mockResolvedValue([]);
  const created = {
    ...webhook({
      id: 9,
      name: 'New Hook',
      url: 'https://example.com/new',
      events: ['contact.created'],
    }),
    secret: 'whsec_abc123',
  };
  vi.mocked(createWebhook).mockResolvedValue(created);

  render(<WebhooksSettings />);
  await waitFor(() => expect(screen.getByText('No webhooks configured yet.')).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: /add webhook/i }));
  fireEvent.change(screen.getByLabelText('Name *'), { target: { value: 'New Hook' } });
  fireEvent.change(screen.getByLabelText('URL *'), {
    target: { value: 'https://example.com/new' },
  });

  fireEvent.mouseDown(screen.getByLabelText(/events \*/i));
  fireEvent.click(await screen.findByRole('option', { name: 'Contact Created' }));

  fireEvent.click(screen.getByRole('button', { name: /^save$/i }));

  await waitFor(() =>
    expect(createWebhook).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'New Hook',
        url: 'https://example.com/new',
        events: ['contact.created'],
      }),
    ),
  );

  // The secret is shown once, right after creation.
  await waitFor(() => expect(screen.getByText('Webhook Secret')).toBeInTheDocument());
  expect(screen.getByDisplayValue('whsec_abc123')).toBeInTheDocument();
});

test('editing a webhook prefills the form and saves via update', async () => {
  vi.mocked(getWebhooks).mockResolvedValue([webhook({ id: 4, name: 'Original' })]);
  vi.mocked(updateWebhook).mockResolvedValue(webhook({ id: 4, name: 'Renamed' }));

  render(<WebhooksSettings />);
  await waitFor(() => expect(screen.getByText('Original')).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
  const nameInput = screen.getByLabelText('Name *') as HTMLInputElement;
  expect(nameInput.value).toBe('Original');

  fireEvent.change(nameInput, { target: { value: 'Renamed' } });
  fireEvent.click(screen.getByRole('button', { name: /^save$/i }));

  await waitFor(() =>
    expect(updateWebhook).toHaveBeenCalledWith(4, expect.objectContaining({ name: 'Renamed' })),
  );
});

test('deleting a webhook requires confirmation', async () => {
  vi.mocked(getWebhooks).mockResolvedValue([webhook({ id: 6, name: 'To Delete' })]);
  vi.mocked(deleteWebhook).mockResolvedValue(undefined);

  render(<WebhooksSettings />);
  await waitFor(() => expect(screen.getByText('To Delete')).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
  expect(deleteWebhook).not.toHaveBeenCalled();
  expect(screen.getByText(/delete/i, { selector: 'h2' })).toBeInTheDocument();

  fireEvent.click(screen.getAllByRole('button', { name: /delete/i }).slice(-1)[0]);

  await waitFor(() => expect(deleteWebhook).toHaveBeenCalledWith(6));
  await waitFor(() => expect(screen.queryByText('To Delete')).not.toBeInTheDocument());
});

test('testing a webhook reports a successful delivery', async () => {
  vi.mocked(getWebhooks).mockResolvedValue([webhook({ id: 2, name: 'Live Hook' })]);
  vi.mocked(testWebhook).mockResolvedValue({
    delivery: {
      id: 1,
      event_type: 'contact.created',
      status_code: 200,
      error: null,
      attempts: 1,
      created_at: '2026-01-01T00:00:00Z',
    },
  });

  render(<WebhooksSettings />);
  await waitFor(() => expect(screen.getByText('Live Hook')).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: 'Test' }));

  await waitFor(() => expect(screen.getByText('Test delivered (200)')).toBeInTheDocument());
});

test('testing a webhook reports a failed delivery without throwing', async () => {
  vi.mocked(getWebhooks).mockResolvedValue([webhook({ id: 3, name: 'Flaky Hook' })]);
  vi.mocked(testWebhook).mockResolvedValue({
    delivery: {
      id: 2,
      event_type: 'contact.created',
      status_code: 500,
      error: 'connection refused',
      attempts: 1,
      created_at: '2026-01-01T00:00:00Z',
    },
  });

  render(<WebhooksSettings />);
  await waitFor(() => expect(screen.getByText('Flaky Hook')).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: 'Test' }));

  await waitFor(() =>
    expect(screen.getByText(/test failed: connection refused/i)).toBeInTheDocument(),
  );
});

test('expanding deliveries fetches them once and shows recent attempts', async () => {
  vi.mocked(getWebhooks).mockResolvedValue([webhook({ id: 8, name: 'Hook With History' })]);
  vi.mocked(getWebhookDeliveries).mockResolvedValue([
    {
      id: 1,
      event_type: 'contact.created',
      status_code: 200,
      error: null,
      attempts: 1,
      created_at: '2026-01-01T00:00:00Z',
    },
  ]);

  render(<WebhooksSettings />);
  await waitFor(() => expect(screen.getByText('Hook With History')).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: /recent deliveries/i }));

  await waitFor(() => expect(getWebhookDeliveries).toHaveBeenCalledWith(8));
  await waitFor(() => expect(screen.getByText('contact.created')).toBeInTheDocument());

  // Collapsing and re-expanding does not refetch.
  fireEvent.click(screen.getByRole('button', { name: /recent deliveries/i }));
  fireEvent.click(screen.getByRole('button', { name: /recent deliveries/i }));
  expect(getWebhookDeliveries).toHaveBeenCalledTimes(1);
});

test('shows a "will not retry" badge for a webhook whose deliveries fail permanently (INT-04)', async () => {
  vi.mocked(getWebhooks).mockResolvedValue([
    webhook({
      id: 3,
      name: 'Dead Receiver',
      delivery_health: {
        last_delivery_at: '2026-02-01T00:00:00Z',
        last_status_code: 404,
        last_error: 'unexpected status 404',
        failed_permanently: true,
        terminal_reason: 'remote-resource-deleted',
        retrying: false,
      },
    }),
  ]);

  render(<WebhooksSettings />);
  await waitFor(() => expect(screen.getByText('Dead Receiver')).toBeInTheDocument());
  expect(screen.getByText('Will not retry')).toBeInTheDocument();
});

test('marks an individual permanently-failed delivery row', async () => {
  vi.mocked(getWebhooks).mockResolvedValue([webhook({ id: 9, name: 'Hook' })]);
  vi.mocked(getWebhookDeliveries).mockResolvedValue([
    {
      id: 1,
      event_type: 'contact.created',
      status_code: 401,
      error: 'unexpected status 401',
      attempts: 1,
      created_at: '2026-01-01T00:00:00Z',
      failed_permanently: true,
      terminal_reason: 'auth-expiry',
    },
  ]);

  render(<WebhooksSettings />);
  await waitFor(() => expect(screen.getByText('Hook')).toBeInTheDocument());
  fireEvent.click(screen.getByRole('button', { name: /recent deliveries/i }));

  await waitFor(() => expect(screen.getByText('contact.created')).toBeInTheDocument());
  expect(screen.getByText('Will not retry')).toBeInTheDocument();
});
