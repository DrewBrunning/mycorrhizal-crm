import { describe, test, expect, vi, afterEach } from 'vitest';
import {
  getWebhooks,
  createWebhook,
  updateWebhook,
  deleteWebhook,
  testWebhook,
  getWebhookDeliveries,
  Webhook,
  WebhookCreateResponse,
  WebhookDelivery,
} from './webhooks';

afterEach(() => {
  vi.unstubAllGlobals();
});

const webhook: Webhook = {
  id: 1,
  name: 'Contact created',
  url: 'https://hooks.example.com/contact',
  events: ['contact.created'],
  is_active: true,
  created_at: '2026-01-01T00:00:00Z',
};

const webhookWithSecret: WebhookCreateResponse = {
  ...webhook,
  secret: 'sec-1',
};

const delivery: WebhookDelivery = {
  id: 1,
  event_type: 'contact.created',
  status_code: 200,
  error: null,
  attempts: 1,
  created_at: '2026-01-01T00:00:00Z',
};

const okResponse = (body: unknown) => ({
  ok: true,
  status: 200,
  text: async () => JSON.stringify(body),
});

const errorResponse = () => ({
  ok: false,
  status: 400,
  statusText: 'Bad Request',
  text: async () => JSON.stringify({ error: { message: 'Invalid URL' } }),
});

describe('getWebhooks', () => {
  test('GETs the webhooks URL and unwraps the webhooks array', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse({ webhooks: [webhook] }));
    vi.stubGlobal('fetch', fetchMock);

    const result = await getWebhooks();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/webhooks');
    expect(init.method).toBe('GET');
    expect(result).toEqual([webhook]);
  });

  test('returns an empty array when the response has no webhooks key', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(okResponse({})));
    const result = await getWebhooks();
    expect(result).toEqual([]);
  });

  test('throws the parsed error message when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(getWebhooks()).rejects.toThrow('Invalid URL');
  });
});

describe('createWebhook', () => {
  test('POSTs the input payload and returns the created webhook with secret', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse(webhookWithSecret));
    vi.stubGlobal('fetch', fetchMock);

    const input = {
      name: 'Contact created',
      url: 'https://hooks.example.com/contact',
      events: ['contact.created'],
      is_active: true,
    };
    const result = await createWebhook(input);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/webhooks');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual(input);
    expect(result).toEqual(webhookWithSecret);
  });

  test('throws the parsed error message when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(
      createWebhook({ name: 'x', url: 'not-a-url', events: [], is_active: true })
    ).rejects.toThrow('Invalid URL');
  });
});

describe('updateWebhook', () => {
  test('PUTs the input payload and returns the updated webhook', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse(webhook));
    vi.stubGlobal('fetch', fetchMock);

    const input = {
      name: 'Renamed',
      url: 'https://hooks.example.com/new',
      events: ['contact.updated'],
      is_active: false,
    };
    const result = await updateWebhook(1, input);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/webhooks/1');
    expect(init.method).toBe('PUT');
    expect(JSON.parse(init.body)).toEqual(input);
    expect(result).toEqual(webhook);
  });

  test('throws the parsed error message when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(
      updateWebhook(1, { name: 'x', url: 'not-a-url', events: [], is_active: true })
    ).rejects.toThrow('Invalid URL');
  });
});

describe('deleteWebhook', () => {
  test('DELETEs the webhook URL', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse({}));
    vi.stubGlobal('fetch', fetchMock);

    await deleteWebhook(1);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/webhooks/1');
    expect(init.method).toBe('DELETE');
  });

  test('throws the parsed error message when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(deleteWebhook(1)).rejects.toThrow('Invalid URL');
  });
});

describe('testWebhook', () => {
  test('POSTs to the test URL and returns the delivery', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse({ delivery }));
    vi.stubGlobal('fetch', fetchMock);

    const result = await testWebhook(1);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/webhooks/1/test');
    expect(init.method).toBe('POST');
    expect(result.delivery).toEqual(delivery);
  });

  test('throws the parsed error message when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(testWebhook(1)).rejects.toThrow('Invalid URL');
  });
});

describe('getWebhookDeliveries', () => {
  test('GETs the deliveries URL and unwraps the deliveries array', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse({ deliveries: [delivery] }));
    vi.stubGlobal('fetch', fetchMock);

    const result = await getWebhookDeliveries(1);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/webhooks/1/deliveries');
    expect(init.method).toBe('GET');
    expect(result).toEqual([delivery]);
  });

  test('throws the parsed error message when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(getWebhookDeliveries(1)).rejects.toThrow('Invalid URL');
  });
});
