// Synthetic network-graph payloads at the #468 scale profiles (issue #556).
//
// HAND-SYNCED MIRROR of backend/internal/largedata/profiles.go. There is no
// dynamic profile endpoint (frontend trap #4) and the Go generator's exact
// graph topology can't be reproduced in the browser -- but the layout cost
// that matters for react-force-graph-2d is driven by node count, edge count,
// and the presence of dense hubs + a deep chain, so this reproduces those.
// If profiles.go's Contacts / Hubs / HubFanout / ChainDepth change, update
// these to match.
//
// Used by networkGraphScale.spec.ts to find the render cliff, and available
// for any other client scale measurement so results line up with the server.

export interface GraphScaleProfile {
  name: string;
  contacts: number;
  hubs: number;
  hubFanout: number;
  chainDepth: number;
}

// Single-user shapes (the /graph endpoint is per-user). `large` uses the
// per-user contact count from profiles.go (15000), not the multi-user total.
export const GRAPH_SCALE_PROFILES: GraphScaleProfile[] = [
  { name: 'smoke', contacts: 150, hubs: 2, hubFanout: 8, chainDepth: 6 },
  { name: 'typical', contacts: 900, hubs: 5, hubFanout: 25, chainDepth: 12 },
  // Intermediate steps to bracket the cliff between `typical` and `large`.
  { name: 'typical+', contacts: 1500, hubs: 8, hubFanout: 40, chainDepth: 16 },
  { name: 'mid', contacts: 3000, hubs: 12, hubFanout: 60, chainDepth: 24 },
  { name: 'large', contacts: 15000, hubs: 25, hubFanout: 150, chainDepth: 40 },
];

export interface SyntheticGraph {
  nodes: { id: string; type: 'contact'; label: string }[];
  edges: { id: string; source: string; target: string; type: 'relationship'; label: string }[];
}

/**
 * Build a GraphResponse-shaped payload for a profile: every contact linked to
 * the next (a baseline ring of ~contacts edges), a deep `parent_of` chain, and
 * `hubs` dense `friend_of` hubs each fanning out to `hubFanout` other contacts.
 */
export function buildScaleGraph(p: GraphScaleProfile): SyntheticGraph {
  const nodes: SyntheticGraph['nodes'] = [];
  const edges: SyntheticGraph['edges'] = [];
  const n = p.contacts;

  for (let i = 0; i < n; i++) {
    nodes.push({ id: `c-${i}`, type: 'contact', label: `Perf ${p.name} ${i}` });
  }

  const addEdge = (a: number, b: number, label: string) => {
    if (a === b || a < 0 || b < 0 || a >= n || b >= n) return;
    edges.push({
      id: `e-${edges.length}`,
      source: `c-${a}`,
      target: `c-${b}`,
      type: 'relationship',
      label,
    });
  };

  // Baseline ring: one edge per contact.
  for (let i = 0; i < n; i++) addEdge(i, (i + 1) % n, 'knows');

  // Deep chain.
  const step = Math.max(1, Math.floor(n / (p.chainDepth + 1)));
  for (let h = 0; h < p.chainDepth; h++) addEdge(h * step, (h + 1) * step, 'parent_of');

  // Dense hubs spread evenly.
  const hubStep = Math.max(1, Math.floor(n / (p.hubs + 1)));
  for (let h = 1; h <= p.hubs; h++) {
    const hub = h * hubStep;
    for (let f = 1; f <= p.hubFanout; f++) addEdge(hub, (hub + f * 7) % n, 'friend_of');
  }

  return { nodes, edges };
}
