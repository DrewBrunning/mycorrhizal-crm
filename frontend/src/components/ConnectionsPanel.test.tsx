import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import '../i18n/config';
import ConnectionsPanel from './ConnectionsPanel';

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const chainsResponse = () => ({
  from_vcard_uid: 'john-uid',
  from_name: 'John',
  depth: 3,
  chains: [
    {
      target_id: 2,
      target_vcard_uid: 'sister-uid',
      target_name: 'Sister',
      depth: 1,
      steps: [{ contact_id: 2, contact_vcard_uid: 'sister-uid', contact_name: 'Sister', relation: 'sibling_of' }],
    },
    {
      target_id: 3,
      target_vcard_uid: 'husband-uid',
      target_name: 'Husband',
      depth: 2,
      steps: [
        { contact_id: 2, contact_vcard_uid: 'sister-uid', contact_name: 'Sister', relation: 'sibling_of' },
        { contact_id: 3, contact_vcard_uid: 'husband-uid', contact_name: 'Husband', relation: 'spouse_of' },
      ],
    },
  ],
});

function mockGraph(urlContains: string, respond: () => unknown) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      if (url.includes(urlContains)) {
        return { ok: true, json: async () => respond() };
      }
      throw new Error(`unexpected fetch: ${url}`);
    })
  );
}

function renderPanel(contactUid = 'john-uid') {
  return render(
    <MemoryRouter>
      <ConnectionsPanel contactUid={contactUid} />
    </MemoryRouter>
  );
}

test('renders reachable chains with the relation path', async () => {
  mockGraph('/graph/connections?', chainsResponse);
  renderPanel();

  await waitFor(() => {
    expect(screen.getAllByText('Husband').length).toBeGreaterThanOrEqual(1);
  });

  // The two-hop chain's relation labels render (localized tokens).
  expect(screen.getAllByText('(Sibling)').length).toBeGreaterThanOrEqual(1);
  expect(screen.getByText('(Spouse)')).toBeInTheDocument();
});

test('empty result shows the no-connections message', async () => {
  mockGraph('/graph/connections?', () => ({
    from_vcard_uid: 'john-uid',
    from_name: 'John',
    depth: 3,
    chains: [],
  }));
  renderPanel();

  await waitFor(() => {
    expect(screen.getByText('No connections found within this depth.')).toBeInTheDocument();
  });
});

test('changing the depth reloads with the new value', async () => {
  const requested: string[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      if (url.includes('/graph/connections?')) {
        requested.push(url);
        return { ok: true, json: async () => chainsResponse() };
      }
      throw new Error(`unexpected fetch: ${url}`);
    })
  );
  renderPanel();

  await waitFor(() => expect(requested.length).toBeGreaterThanOrEqual(1));

  fireEvent.mouseDown(screen.getByLabelText('Depth'));
  fireEvent.click(screen.getByText('2'));
  await waitFor(() => {
    expect(requested.some((u) => u.includes('depth=2'))).toBe(true);
  });
});

test('applying a relation filter sends it as a query param', async () => {
  const requested: string[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      if (url.includes('/graph/connections?')) {
        requested.push(url);
        return { ok: true, json: async () => chainsResponse() };
      }
      throw new Error(`unexpected fetch: ${url}`);
    })
  );
  renderPanel();

  await waitFor(() => expect(requested.length).toBeGreaterThanOrEqual(1));

  fireEvent.change(screen.getByLabelText('Relation filter'), { target: { value: 'brother' } });
  fireEvent.click(screen.getByRole('button', { name: 'Apply' }));
  await waitFor(() => {
    expect(requested.some((u) => u.includes('relation=brother'))).toBe(true);
  });
});

test('changing the depth after applying a filter keeps the filter', async () => {
  const requested: string[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      if (url.includes('/graph/connections?')) {
        requested.push(url);
        return { ok: true, json: async () => chainsResponse() };
      }
      throw new Error(`unexpected fetch: ${url}`);
    })
  );
  renderPanel();

  await waitFor(() => expect(requested.length).toBeGreaterThanOrEqual(1));

  fireEvent.change(screen.getByLabelText('Relation filter'), { target: { value: 'brother' } });
  fireEvent.click(screen.getByRole('button', { name: 'Apply' }));
  await waitFor(() => expect(requested.some((u) => u.includes('relation=brother'))).toBe(true));

  // Change depth — the applied filter must survive.
  fireEvent.mouseDown(screen.getByLabelText('Depth'));
  fireEvent.click(screen.getByText('2'));
  await waitFor(() => {
    expect(requested.some((u) => u.includes('depth=2') && u.includes('relation=brother'))).toBe(true);
  });
});
