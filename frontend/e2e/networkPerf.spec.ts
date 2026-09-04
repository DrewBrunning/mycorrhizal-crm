import { expect, test } from './fixtures';

const GRAPH_ROUTE = '**/api/v1/graph';

/**
 * Network-graph render ceiling (issue #556, recommended actions 2 + 3).
 *
 * The force-directed layout in NetworkGraph.tsx runs on the main thread until
 * it settles; past a measured node/edge ceiling it stops settling and hangs
 * the tab. The defined behaviour above that ceiling is *not* a hang: the
 * canvas is not mounted, a notice points at the filters and the list view,
 * and NetworkListView (always in the DOM) remains the full view.
 *
 * This is the light, per-PR half: it stubs GET /graph with a synthetic
 * over-ceiling payload and asserts the degradation. The nightly
 * @perf-heavy benchmark (networkGraphScale.spec.ts) is what measures where
 * the real cliff is and keeps the ceiling honest.
 */

// Keep in sync with src/utils/graphBudget.ts.
const NODE_CEILING = 2000;

interface StubNode {
  id: string;
  type: 'contact';
  label: string;
}
interface StubEdge {
  id: string;
  source: string;
  target: string;
  type: 'relationship';
  label: string;
}

function buildGraph(contactCount: number): { nodes: StubNode[]; edges: StubEdge[] } {
  const nodes: StubNode[] = [];
  const edges: StubEdge[] = [];
  for (let i = 0; i < contactCount; i++) {
    nodes.push({ id: `c-${i}`, type: 'contact', label: `Perf Contact ${i}` });
    if (i > 0) {
      edges.push({
        id: `e-${i}`,
        source: `c-${i - 1}`,
        target: `c-${i}`,
        type: 'relationship',
        label: 'friend_of',
      });
    }
  }
  return { nodes, edges };
}

test.describe('Network graph render ceiling', { tag: '@perf' }, () => {
  test('degrades to a notice + list view when the graph exceeds the ceiling, and recovers under a filter', async ({
    page,
  }) => {
    const huge = buildGraph(NODE_CEILING + 500);
    const small = buildGraph(40);

    await page.route(GRAPH_ROUTE, (route) => route.fulfill({ json: huge }));

    await page.goto('/network');

    // Degradation is visible; the canvas is not mounted.
    const notice = page.getByTestId('graph-over-budget');
    await expect(notice).toBeVisible();
    await expect(notice).toContainText(String(NODE_CEILING + 499)); // edge count = nodes - 1
    await expect(page.locator('canvas')).toHaveCount(0);

    // The always-in-DOM list view still lists the contacts (NetworkListView
    // renders one ListItemButton per contact) -- first and last, so a
    // truncated render would fail.
    await expect(page.getByRole('heading', { name: /network list view/i })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Perf Contact 0', exact: true })).toBeVisible();
    await expect(
      page.getByRole('button', { name: `Perf Contact ${NODE_CEILING + 499}`, exact: true }),
    ).toBeVisible();

    // Serve a small payload (as a circle/contact filter would narrow it) and
    // confirm the canvas renders instead of the notice.
    await page.unroute(GRAPH_ROUTE);
    await page.route(GRAPH_ROUTE, (route) => route.fulfill({ json: small }));
    await page.reload();

    await expect(page.locator('canvas')).toHaveCount(1);
    await expect(page.getByTestId('graph-over-budget')).toHaveCount(0);
  });
});
