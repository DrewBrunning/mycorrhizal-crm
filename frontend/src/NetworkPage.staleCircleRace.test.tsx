import { act, cleanup, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import './i18n/config';
import { listCircles } from './api/circles';
import type { GraphData, GraphNode } from './types/graph';

// This file deliberately does NOT mock ./hooks/useCircles (unlike
// NetworkPage.test.tsx), because the bug it guards against is a race
// between useCircles' *own* mount effect and NetworkPage's stale-circle
// cleanup effect within the *same* initial React commit -- mocking the hook
// away hides exactly the interaction under test. Only the network call it
// makes (listCircles) and the graph (out of scope, canvas-based) are mocked.
vi.mock('./components/NetworkGraph', () => ({
  default: (props: { onNodeClick: (node: GraphNode) => void; selectedCircle?: string }) => (
    <div data-testid="network-graph-stub">
      <div data-testid="selected-circle">{props.selectedCircle ?? ''}</div>
    </div>
  ),
}));

const mockUseGraphResult = {
  data: { nodes: [{ id: 'c-1', type: 'contact', label: 'Alice' }], edges: [] } as GraphData,
  loading: false,
  error: null as string | null,
  refetch: vi.fn(),
};

vi.mock('./hooks/useGraph', () => ({
  useGraph: () => mockUseGraphResult,
}));

vi.mock('./api/circles', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/circles')>();
  return { ...actual, listCircles: vi.fn() };
});

beforeEach(() => {
  localStorage.setItem(
    'user_info',
    JSON.stringify({ user_id: 1, username: 'test', is_admin: false }),
  );
});

afterEach(() => {
  cleanup();
  localStorage.clear();
  vi.clearAllMocks();
});

async function renderPage() {
  const { default: NetworkPage } = await import('./NetworkPage');
  return render(
    <MemoryRouter>
      <NetworkPage />
    </MemoryRouter>,
  );
}

test('a persisted circle survives the initial render while the real circle list is still loading', async () => {
  localStorage.setItem('network-selected-circle', 'Family');
  // Controlled: resolves only when the test calls resolveCircles() below, so
  // the assertion right after render lands inside the window where useCircles
  // has started fetching (loading flipped true by its own mount effect) but
  // has not yet resolved -- exactly the window the initial-loading-state fix
  // (frontend/src/hooks/useCircles.ts) exists to protect.
  let resolveCircles!: (v: Awaited<ReturnType<typeof listCircles>>) => void;
  vi.mocked(listCircles).mockReturnValue(
    new Promise((resolve) => {
      resolveCircles = resolve;
    }),
  );

  await act(async () => {
    await renderPage();
  });

  // Still fetching -- must not have been wiped out despite circleNames
  // being empty right now.
  expect(screen.getByTestId('selected-circle').textContent).toBe('Family');
  expect(localStorage.getItem('network-selected-circle')).toBe('Family');

  await act(async () => {
    resolveCircles({
      circles: [{ id: 'c-1', name: 'Family', created_at: '', updated_at: '' }],
      members: [],
      total: 1,
      next_cursor: '',
      limit: 200,
    });
  });

  // Now genuinely loaded and the circle still exists -- still there.
  await waitFor(() => {
    expect(screen.getByTestId('selected-circle').textContent).toBe('Family');
  });
});

test('a persisted circle is dropped once the real circle list has loaded and no longer contains it', async () => {
  localStorage.setItem('network-selected-circle', 'Deleted Circle');
  vi.mocked(listCircles).mockResolvedValue({
    circles: [{ id: 'c-1', name: 'Family', created_at: '', updated_at: '' }],
    members: [],
    total: 1,
    next_cursor: '',
    limit: 200,
  });

  await act(async () => {
    await renderPage();
  });

  await waitFor(() => {
    expect(screen.getByTestId('selected-circle').textContent).toBe('');
  });
  expect(localStorage.getItem('network-selected-circle')).toBe('');
});
