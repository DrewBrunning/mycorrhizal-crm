// Network-graph render budget (issue #556, PERF web counterpart to #468-#471).
//
// NetworkGraph.tsx lays the relationship graph out with react-force-graph-2d /
// d3-force. Force-directed layout is O(n log n) per tick at best, runs every
// tick until it settles, and does it on the main thread. A dense, long-lived
// personal CRM -- exactly what this app is for -- is the one place in the web
// client that can hang a browser tab.
//
// Rather than let that happen, the graph has a hard ceiling: above it,
// NetworkGraph renders a defined fallback (a notice + the always-present
// NetworkListView) instead of mounting the canvas. The check runs on the
// *filtered* graph data, so narrowing by circle or centring on a contact
// brings the set back under the ceiling and the canvas returns.
//
// The numbers below are the measured cliff, not a guess in principle: the
// nightly Playwright benchmark (frontend/e2e/networkGraphScale.spec.ts, tagged
// @perf-heavy) drives synthetic graphs at the #468 scale profiles and records
// where layout stops settling in reasonable time. They MUST stay in sync with
// docs/development/web-perf-budgets.md -- update both together.
//
// Chosen so the #468 `typical` profile (900 contacts, a handful of dense hubs,
// a depth-12 chain -> well under 2000 nodes) always renders, while `large`
// (45k contacts) always degrades. See the doc for the full rationale.

export const GRAPH_RENDER_NODE_CEILING = 2000;
export const GRAPH_RENDER_EDGE_CEILING = 6000;

export interface GraphRenderBudgetInput {
  nodes: readonly unknown[];
  links: readonly unknown[];
}

/**
 * True when the (already filtered) graph is too large to lay out interactively
 * and NetworkGraph should render its fallback instead of the canvas.
 */
export function exceedsGraphRenderBudget({ nodes, links }: GraphRenderBudgetInput): boolean {
  return nodes.length > GRAPH_RENDER_NODE_CEILING || links.length > GRAPH_RENDER_EDGE_CEILING;
}
