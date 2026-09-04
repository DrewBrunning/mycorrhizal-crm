import { describe, expect, test } from 'vitest';
import {
  exceedsGraphRenderBudget,
  GRAPH_RENDER_EDGE_CEILING,
  GRAPH_RENDER_NODE_CEILING,
} from './graphBudget';

const arr = (n: number) => new Array(Math.max(0, n)).fill(0);

describe('exceedsGraphRenderBudget', () => {
  test('is false at exactly the ceiling on both axes', () => {
    expect(
      exceedsGraphRenderBudget({
        nodes: arr(GRAPH_RENDER_NODE_CEILING),
        links: arr(GRAPH_RENDER_EDGE_CEILING),
      }),
    ).toBe(false);
  });

  test('is true one node over the node ceiling', () => {
    expect(exceedsGraphRenderBudget({ nodes: arr(GRAPH_RENDER_NODE_CEILING + 1), links: [] })).toBe(
      true,
    );
  });

  test('is true one edge over the edge ceiling', () => {
    expect(exceedsGraphRenderBudget({ nodes: [], links: arr(GRAPH_RENDER_EDGE_CEILING + 1) })).toBe(
      true,
    );
  });

  test('is false for a small graph', () => {
    expect(exceedsGraphRenderBudget({ nodes: arr(50), links: arr(80) })).toBe(false);
  });

  test('the #468 typical profile (~900 contacts) stays under the ceiling', () => {
    // typical: 900 contacts + a few dense hubs + a depth-12 chain + circle and
    // activity nodes. Even generously doubled for synthetic nodes/edges this
    // is well under 2000/6000 -- typical must always render the canvas.
    expect(exceedsGraphRenderBudget({ nodes: arr(1800), links: arr(4000) })).toBe(false);
  });

  test('the #468 large profile (45k contacts) always degrades', () => {
    expect(exceedsGraphRenderBudget({ nodes: arr(45000), links: arr(90000) })).toBe(true);
  });
});
