import { expect, test } from 'vitest';
import type { GraphData, GraphEdge, GraphNode } from '../types/graph';
import {
  computeFilteredGraphData,
  edgeEndpointId,
  type NetworkGraphFilters,
} from './networkGraphData';

function contact(id: string, label = id): GraphNode {
  return { id, type: 'contact', label };
}

function activity(id: string, label = id): GraphNode {
  return { id, type: 'activity', label };
}

function rel(source: string, target: string): GraphEdge {
  return {
    id: `rel-${source}-${target}`,
    source,
    target,
    type: 'relationship',
    label: 'related to',
  };
}

function actEdge(source: string, target: string): GraphEdge {
  return { id: `act-${source}-${target}`, source, target, type: 'activity', label: 'met' };
}

const base: GraphData = {
  nodes: [
    contact('c-1', 'Ada'),
    contact('c-2', 'Grace'),
    contact('c-3', 'Alan'),
    activity('a-1', 'Coffee'),
  ],
  edges: [rel('c-1', 'c-2'), actEdge('a-1', 'c-1'), actEdge('a-1', 'c-2')],
};

const noFilters: NetworkGraphFilters = {
  showRelationships: true,
  showActivities: true,
  showCircles: false,
};

function ids(nodes: GraphNode[]) {
  return nodes.map((n) => n.id);
}

test('passthrough with no filters returns everything', () => {
  const out = computeFilteredGraphData(base, noFilters);
  expect(ids(out.nodes)).toEqual(['c-1', 'c-2', 'c-3', 'a-1']);
  expect(out.links).toHaveLength(3);
});

test('hiding relationships drops relationship edges but keeps nodes', () => {
  const out = computeFilteredGraphData(base, { ...noFilters, showRelationships: false });
  expect(ids(out.nodes)).toEqual(['c-1', 'c-2', 'c-3', 'a-1']);
  expect(out.links.map((l) => l.type)).toEqual(['activity', 'activity']);
});

test('hiding activities drops activity nodes and their edges', () => {
  const out = computeFilteredGraphData(base, { ...noFilters, showActivities: false });
  expect(ids(out.nodes)).toEqual(['c-1', 'c-2', 'c-3']);
  expect(out.links.map((l) => l.type)).toEqual(['relationship']);
});

test('edges whose endpoints were filtered out are dropped too', () => {
  const data: GraphData = {
    nodes: [contact('c-1'), activity('a-1')],
    edges: [rel('c-1', 'c-2'), actEdge('a-1', 'c-1')],
  };
  // c-2 is not a node: the relationship edge to it must vanish even though
  // relationships are shown.
  const out = computeFilteredGraphData(data, noFilters);
  expect(out.links).toEqual([actEdge('a-1', 'c-1')]);
});

// --- circle selection ---

test('selectedCircle keeps contacts in the circle', () => {
  const circleNamesByUid = new Map<string, string[]>([
    ['1', ['Family']],
    ['2', ['Family']],
  ]);
  const out = computeFilteredGraphData(base, {
    ...noFilters,
    selectedCircle: 'Family',
    circleNamesByUid,
  });
  expect(ids(out.nodes)).toEqual(['c-1', 'c-2', 'a-1']);
});

test('selectedCircle keeps an activity only when it connects 2+ circle contacts', () => {
  const circleNamesByUid = new Map<string, string[]>([['1', ['Family']]]);
  // a-1 connects c-1 and c-2; only c-1 is in the circle, so the activity must
  // be excluded.
  const out = computeFilteredGraphData(base, {
    ...noFilters,
    selectedCircle: 'Family',
    circleNamesByUid,
  });
  expect(ids(out.nodes)).toEqual(['c-1']);
});

test('selectedCircle requires circleNamesByUid to make any contact eligible', () => {
  const out = computeFilteredGraphData(base, {
    ...noFilters,
    selectedCircle: 'Family',
    circleNamesByUid: undefined,
  });
  expect(ids(out.nodes)).toEqual([]);
});

// --- circle node synthesis ---

test('synthesizes circle nodes for circles with 2+ visible contacts', () => {
  const circleNamesByUid = new Map<string, string[]>([
    ['1', ['Family']],
    ['2', ['Family']],
  ]);
  const out = computeFilteredGraphData(base, {
    ...noFilters,
    showCircles: true,
    circleNamesByUid,
  });

  const circleNode = out.nodes.find((n) => n.type === 'circle');
  expect(circleNode).toBeDefined();
  expect(circleNode?.id).toBe('circle-Family');
  expect(circleNode?.label).toBe('Family');

  const circleEdges = out.links.filter((l) => l.type === 'circle');
  expect(circleEdges).toHaveLength(2);
  expect(circleEdges.map((e) => e.source).sort()).toEqual(['c-1', 'c-2']);
  expect(circleEdges.every((e) => e.target === 'circle-Family')).toBe(true);
});

test('does not synthesize a circle node for a single-member circle', () => {
  const circleNamesByUid = new Map<string, string[]>([['1', ['Solo']]]);
  const out = computeFilteredGraphData(base, {
    ...noFilters,
    showCircles: true,
    circleNamesByUid,
  });
  expect(out.nodes.filter((n) => n.type === 'circle')).toHaveLength(0);
});

test('circle synthesis only counts visible contacts', () => {
  // Only c-1 is in "Family" (c-2 is in "Work"); selecting Family drops the
  // activity (it connects just one circle contact), leaving a single visible
  // circle contact — so no circle node may be synthesized from it.
  const circleNamesByUid = new Map<string, string[]>([
    ['1', ['Family']],
    ['2', ['Work']],
  ]);
  const out = computeFilteredGraphData(base, {
    ...noFilters,
    showCircles: true,
    selectedCircle: 'Family',
    circleNamesByUid,
  });
  expect(out.nodes.filter((n) => n.type === 'circle')).toHaveLength(0);
});

// --- centered node ---

test('centeredNodeId keeps the node and its direct neighbors', () => {
  const out = computeFilteredGraphData(base, { ...noFilters, centeredNodeId: 'c-1' });
  // c-1's direct neighbors are c-2 and a-1; every edge whose endpoints are all
  // in {c-1, c-2, a-1} survives: rel(c-1,c-2), act(a-1,c-1), act(a-1,c-2).
  expect(ids(out.nodes).sort()).toEqual(['a-1', 'c-1', 'c-2']);
  expect(out.links).toHaveLength(3);
});

test('centeredNodeId drops edges that do not touch the centered node', () => {
  const data: GraphData = {
    nodes: [contact('c-1'), contact('c-2'), contact('c-3')],
    edges: [rel('c-1', 'c-2'), rel('c-2', 'c-3')],
  };
  const out = computeFilteredGraphData(data, { ...noFilters, centeredNodeId: 'c-1' });
  expect(ids(out.nodes)).toEqual(['c-1', 'c-2']);
  expect(out.links).toEqual([rel('c-1', 'c-2')]);
});

// --- edgeEndpointId ---

test('edgeEndpointId unwraps both string and object endpoints', () => {
  expect(edgeEndpointId('c-1')).toBe('c-1');
  expect(edgeEndpointId(contact('c-1'))).toBe('c-1');
});
