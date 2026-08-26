import { cleanup, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import RelationshipSuggestionsInbox from './RelationshipSuggestionsInbox';

afterEach(cleanup);

const edge = {
  id: 'edge-1',
  source_id: 'alice-uid',
  target_id: 'bob-uid',
  type: 'parent_of',
  directional: true,
  source: 'graph-inferred',
  confidence: 0.7,
  status: 'suggested',
  sensitivity: 'normal',
  created_at: '2026-08-15T00:00:00Z',
  updated_at: '2026-08-15T00:00:00Z',
};

const contactsPayload = {
  ok: true,
  json: async () => ({
    contacts: [
      { id: 1, uid: 'alice-uid', firstname: 'Alice', lastname: 'Anderson' },
      { id: 2, uid: 'bob-uid', firstname: 'Bob', lastname: 'Brown' },
    ],
  }),
};

function stubFetch(edges: unknown[], acceptCalls: { method: string; url: string }[] = []) {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      const method = init?.method || 'GET';
      const acceptResult = { ok: true, json: async () => ({}) };
      const listResult = {
        ok: true,
        json: async () => ({
          relationship_edges: edges,
          total: edges.length,
          next_cursor: '',
          limit: 100,
        }),
      };
      if (method === 'GET' && String(url).includes('/relationship-edges')) {
        return Promise.resolve(listResult);
      }
      if (method === 'GET') {
        return Promise.resolve(contactsPayload);
      }
      if (method === 'PATCH' || method === 'DELETE') {
        acceptCalls.push({ method, url: String(url) });
        return Promise.resolve(acceptResult);
      }
      return Promise.resolve(listResult);
    }),
  );
}

test('renders the global suggestion inbox with both endpoint names and the relation label', async () => {
  stubFetch([edge]);
  render(<RelationshipSuggestionsInbox loadKey={0} />);

  expect(await screen.findByText(/Alice Anderson · Parent · Bob Brown/)).toBeInTheDocument();
  expect(screen.getByText('Suggested')).toBeInTheDocument();
  vi.unstubAllGlobals();
});

test('shows the empty state when there are no pending suggestions', async () => {
  stubFetch([]);
  render(<RelationshipSuggestionsInbox loadKey={0} />);

  expect(await screen.findByText('No new relationship suggestions right now.')).toBeInTheDocument();
  vi.unstubAllGlobals();
});

test('accept calls the accept endpoint', async () => {
  const acceptCalls: { method: string; url: string }[] = [];
  stubFetch([edge], acceptCalls);
  render(<RelationshipSuggestionsInbox loadKey={0} />);

  const acceptButton = await screen.findByRole('button', { name: 'Accept' });
  acceptButton.click();

  await waitFor(() => {
    expect(acceptCalls).toContainEqual({
      method: 'PATCH',
      url: expect.stringContaining('/relationship-edges/edge-1/accept'),
    });
  });
  vi.unstubAllGlobals();
});

test('reject calls the delete endpoint (reject is DELETE)', async () => {
  const rejectCalls: { method: string; url: string }[] = [];
  stubFetch([edge], rejectCalls);
  render(<RelationshipSuggestionsInbox loadKey={0} />);

  const rejectButton = await screen.findByRole('button', { name: 'Reject' });
  rejectButton.click();

  await waitFor(() => {
    expect(rejectCalls).toContainEqual({
      method: 'DELETE',
      url: expect.stringContaining('/relationship-edges/edge-1'),
    });
  });
  vi.unstubAllGlobals();
});
