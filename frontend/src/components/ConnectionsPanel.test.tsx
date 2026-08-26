import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, expect, test, vi } from 'vitest';
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
      steps: [
        {
          contact_id: 2,
          contact_vcard_uid: 'sister-uid',
          contact_name: 'Sister',
          relation: 'sibling_of',
        },
      ],
    },
    {
      target_id: 3,
      target_vcard_uid: 'husband-uid',
      target_name: 'Husband',
      depth: 2,
      steps: [
        {
          contact_id: 2,
          contact_vcard_uid: 'sister-uid',
          contact_name: 'Sister',
          relation: 'sibling_of',
        },
        {
          contact_id: 3,
          contact_vcard_uid: 'husband-uid',
          contact_name: 'Husband',
          relation: 'spouse_of',
        },
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
    }),
  );
}

function renderPanel(contactUid = 'john-uid') {
  return render(
    <MemoryRouter>
      <ConnectionsPanel contactUid={contactUid} />
    </MemoryRouter>,
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

// Issue #187: the relation caption and the between-step arrow used to render
// at text.disabled (2.62:1 on parchment, MUI's default rgba(0,0,0,0.38)) --
// too low-contrast for content that gives the chain its meaning, not a
// disabled/greyed-out affordance. No axe/e2e scan exercises this panel by
// default (it needs a contact with a confirmed relationship edge, which the
// e2e seed data doesn't have), so this is the only regression coverage for
// the fix -- confirm both render at text.secondary (rgba(0,0,0,0.6) under
// MUI's default palette, distinct from text.disabled) instead.
test('renders the relation caption and chain arrow at text.secondary, not text.disabled', async () => {
  mockGraph('/graph/connections?', chainsResponse);
  renderPanel();

  const relationLabel = await screen.findByText('(Spouse)');
  expect(getComputedStyle(relationLabel).color).toBe('rgba(0, 0, 0, 0.6)');

  const arrow = document.querySelector('[data-testid="ArrowForwardIcon"]');
  expect(arrow).not.toBeNull();
  expect(getComputedStyle(arrow as Element).color).toBe('rgba(0, 0, 0, 0.6)');
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
    }),
  );
  renderPanel();

  await waitFor(() => expect(requested.length).toBeGreaterThanOrEqual(1));

  // The panel defaults to 1 hop (testing feedback), so the initial fetch must
  // request it — a regression back to the old depth-3 default would show here.
  expect(requested[0]).toContain('depth=1');

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
    }),
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
    }),
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
    expect(requested.some((u) => u.includes('depth=2') && u.includes('relation=brother'))).toBe(
      true,
    );
  });
});

// T114: more than five chains render only five by default, with a "View all"
// toggle revealing the rest (the timeline's T78 pattern).
test('previews the first five chains and reveals the rest on demand', async () => {
  const manyChains = (from: number, to: number) =>
    Array.from({ length: to - from + 1 }, (_, i) => ({
      target_id: from + i,
      target_vcard_uid: `c${from + i}-uid`,
      target_name: `Contact ${from + i}`,
      depth: 1,
      steps: [
        {
          contact_id: from + i,
          contact_vcard_uid: `c${from + i}-uid`,
          contact_name: `Contact ${from + i}`,
          relation: 'sibling_of',
        },
      ],
    }));
  mockGraph('/graph/connections?', () => ({
    from_vcard_uid: 'john-uid',
    from_name: 'John',
    depth: 1,
    chains: manyChains(1, 7),
  }));
  renderPanel();

  await waitFor(() => {
    expect(screen.getAllByText('Contact 1').length).toBeGreaterThan(0);
  });

  // Only the first five render before expanding (each contact appears twice:
  // as the chain's target_name and as its own step link).
  expect(screen.getAllByText('Contact 5').length).toBeGreaterThan(0);
  expect(screen.queryAllByText('Contact 6').length).toBe(0);
  expect(screen.queryAllByText('Contact 7').length).toBe(0);
  expect(screen.getByRole('button', { name: 'View all' })).toBeInTheDocument();

  fireEvent.click(screen.getByRole('button', { name: 'View all' }));

  expect(screen.getAllByText('Contact 6').length).toBeGreaterThan(0);
  expect(screen.getAllByText('Contact 7').length).toBeGreaterThan(0);
  expect(screen.getByRole('button', { name: 'Show less' })).toBeInTheDocument();
});
