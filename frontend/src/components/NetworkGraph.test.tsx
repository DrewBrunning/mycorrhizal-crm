import { createTheme } from '@mui/material/styles';
import { act, cleanup, render, screen } from '@testing-library/react';
import { forwardRef, useImperativeHandle } from 'react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { GraphData, GraphNode } from '../types/graph';
import { GRAPH_RENDER_NODE_CEILING } from '../utils/graphBudget';
import NetworkGraph from './NetworkGraph';

// This codebase's vitest setup does not auto-cleanup between tests -- see
// RelationshipEdgeList.test.tsx's matching comment.
afterEach(cleanup);

// react-force-graph-2d draws to a real <canvas> via HTMLCanvasElement APIs
// jsdom doesn't implement, so it can't render in this environment. Stubbed
// with a forwardRef component exposing the same imperative handle
// (zoom/centerAt/zoomToFit/d3Force) NetworkGraph's effects and pan/zoom
// handlers call, so those code paths still exercise for real.
const zoomFn = vi.fn(() => 1);
const centerAtFn = vi.fn(() => ({ x: 0, y: 0 }));
const zoomToFitFn = vi.fn();
const d3ForceFn = vi.fn(() => ({ strength: vi.fn() }));

type NodeCanvasObjectFn = (
  node: GraphNode,
  ctx: CanvasRenderingContext2D,
  globalScale: number,
) => void;

interface ForceGraphStubProps {
  cooldownTicks?: number;
  nodeCanvasObject?: NodeCanvasObjectFn;
}

// Captured on every render so tests can invoke NetworkGraph's real
// nodeCanvasObject callback directly against a fake canvas context -- the
// stub itself never draws (jsdom has no real <canvas>), but the callback is
// plain JS and safe to call standalone (issue #383's node-fill-color logic).
let capturedNodeCanvasObject: NodeCanvasObjectFn | undefined;

vi.mock('react-force-graph-2d', () => ({
  default: forwardRef<unknown, ForceGraphStubProps>((props, ref) => {
    useImperativeHandle(ref, () => ({
      zoom: zoomFn,
      centerAt: centerAtFn,
      zoomToFit: zoomToFitFn,
      d3Force: d3ForceFn,
    }));
    capturedNodeCanvasObject = props.nodeCanvasObject;
    // Expose cooldownTicks so the reduced-motion tests can assert the graph
    // settles instantly instead of animating (#194, WCAG 2.3.3).
    return <div data-testid="force-graph-stub" data-cooldown-ticks={props.cooldownTicks} />;
  }),
}));

// A minimal CanvasRenderingContext2D fake that records every fillStyle
// assignment in order, so a test can inspect the FIRST one (the node-body
// fill) without the later text-drawing fillStyle overwrites clobbering it.
function createTrackingCtx() {
  const fillStyles: string[] = [];
  const ctx = {
    beginPath: vi.fn(),
    arc: vi.fn(),
    fill: vi.fn(),
    stroke: vi.fn(),
    fillText: vi.fn(),
  } as unknown as CanvasRenderingContext2D;
  Object.defineProperty(ctx, 'fillStyle', {
    set: (v: string) => {
      fillStyles.push(v);
    },
    get: () => fillStyles[fillStyles.length - 1],
  });
  Object.defineProperty(ctx, 'strokeStyle', { set: () => {}, get: () => '' });
  Object.defineProperty(ctx, 'lineWidth', { set: () => {}, get: () => 0 });
  Object.defineProperty(ctx, 'font', { set: () => {}, get: () => '' });
  Object.defineProperty(ctx, 'textAlign', { set: () => {}, get: () => 'center' });
  Object.defineProperty(ctx, 'textBaseline', { set: () => {}, get: () => 'middle' });
  return { ctx, fillStyles };
}

// MUI's useMediaQuery needs window.matchMedia; jsdom provides none.
function mockMatchMedia(matches: boolean) {
  window.matchMedia = vi.fn().mockImplementation(() => ({
    matches,
    media: '',
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  }));
}

function sampleData(): GraphData {
  return {
    nodes: [
      { id: 'c-1', type: 'contact', label: 'Alice' },
      { id: 'c-2', type: 'contact', label: 'Bob' },
    ],
    edges: [{ id: 'e-1', source: 'c-1', target: 'c-2', type: 'relationship', label: 'friend_of' }],
  };
}

test('the canvas wrapper is role=img with a data-driven aria-label, and the pan/zoom controls are NOT nested inside it', () => {
  mockMatchMedia(false);

  render(
    <NetworkGraph
      data={sampleData()}
      onNodeClick={vi.fn()}
      showRelationships
      showActivities
      showCircles={false}
    />,
  );

  const img = screen.getByRole('img');
  expect(img).toHaveAccessibleName(/2 contacts, 0 activities, 0 circles, 1 connections/);

  // Regression guard: WAI-ARIA's own text for the img role says user agents
  // aren't required to expose descendants of a role="img" element -- an
  // earlier version of this component nested the pan/zoom buttons inside
  // it, which risks the buttons this ticket (#190) exists to add becoming
  // unreachable to screen reader users even though they show up fine in
  // Chrome's raw accessibility tree. The buttons must be siblings, not
  // descendants, of the img-role node.
  const panUpButton = screen.getByRole('button', { name: 'Pan up' });
  expect(img).not.toContainElement(panUpButton);
});

function oversizedData(): GraphData {
  const nodes = Array.from({ length: GRAPH_RENDER_NODE_CEILING + 1 }, (_, i) => ({
    id: `c-${i}`,
    type: 'contact' as const,
    label: `Contact ${i}`,
  }));
  return { nodes, edges: [] };
}

test('renders the defined fallback (not the canvas) when the filtered graph exceeds the render ceiling (#556)', () => {
  mockMatchMedia(false);

  render(
    <NetworkGraph
      data={oversizedData()}
      onNodeClick={vi.fn()}
      showRelationships
      showActivities
      showCircles={false}
    />,
  );

  // The over-budget notice is shown, the force-graph canvas is not mounted,
  // and the notice still carries the role=img text alternative.
  const notice = screen.getByTestId('graph-over-budget');
  expect(notice).toBeInTheDocument();
  expect(notice).toHaveTextContent(/too many to lay out/i);
  expect(screen.queryByTestId('force-graph-stub')).not.toBeInTheDocument();
  expect(screen.getByRole('img')).toHaveTextContent(String(GRAPH_RENDER_NODE_CEILING + 1));
});

test('renders the canvas (not the fallback) for a graph under the render ceiling', () => {
  mockMatchMedia(false);

  render(
    <NetworkGraph
      data={sampleData()}
      onNodeClick={vi.fn()}
      showRelationships
      showActivities
      showCircles={false}
    />,
  );

  expect(screen.getByTestId('force-graph-stub')).toBeInTheDocument();
  expect(screen.queryByTestId('graph-over-budget')).not.toBeInTheDocument();
});

test('pan/zoom controls call the graph ref API', () => {
  mockMatchMedia(false);

  render(
    <NetworkGraph
      data={sampleData()}
      onNodeClick={vi.fn()}
      showRelationships
      showActivities
      showCircles={false}
    />,
  );

  screen.getByRole('button', { name: 'Zoom in' }).click();
  expect(zoomFn).toHaveBeenCalled();

  screen.getByRole('button', { name: 'Reset view' }).click();
  expect(zoomToFitFn).toHaveBeenCalled();

  screen.getByRole('button', { name: 'Pan right' }).click();
  expect(centerAtFn).toHaveBeenCalled();
});

// #194 / WCAG 2.3.3 (Animation from Interactions, AAA): the force layout must
// settle instantly and the initial zoom must not animate under the OS's
// reduced-motion preference, not run regardless of it. The source comment at
// NetworkGraph.tsx's `prefersReducedMotion` is the claim; these two tests are
// the check. The control test pins the non-reduced path so a change that just
// hard-codes "instant" everywhere cannot pass both.
test('under prefers-reduced-motion the graph settles instantly and skips the zoom animation (#194)', () => {
  mockMatchMedia(true);
  zoomToFitFn.mockClear();
  vi.useFakeTimers();
  try {
    render(
      <NetworkGraph
        data={sampleData()}
        onNodeClick={vi.fn()}
        showRelationships
        showActivities
        showCircles={false}
      />,
    );

    expect(screen.getByTestId('force-graph-stub')).toHaveAttribute('data-cooldown-ticks', '0');

    act(() => {
      vi.advanceTimersByTime(500);
    });
    // zoomToFit(durationMs, paddingPx): 0 duration = no animation.
    expect(zoomToFitFn).toHaveBeenLastCalledWith(0, 50);
  } finally {
    vi.useRealTimers();
  }
});

test('without the preference the graph still animates (control for #194)', () => {
  mockMatchMedia(false);
  zoomToFitFn.mockClear();
  vi.useFakeTimers();
  try {
    render(
      <NetworkGraph
        data={sampleData()}
        onNodeClick={vi.fn()}
        showRelationships
        showActivities
        showCircles={false}
      />,
    );

    expect(screen.getByTestId('force-graph-stub')).toHaveAttribute('data-cooldown-ticks', '100');

    act(() => {
      vi.advanceTimersByTime(500);
    });
    expect(zoomToFitFn).toHaveBeenLastCalledWith(400, 80);
  } finally {
    vi.useRealTimers();
  }
});

// --- Issue #383/ADR-0023: relationship health score node coloring ---------

test("colors a contact node's fill by its health_band", () => {
  mockMatchMedia(false);
  const theme = createTheme();
  const data: GraphData = {
    nodes: [
      { id: 'c-1', type: 'contact', label: 'Alice', health_band: 'moss' },
      { id: 'c-2', type: 'contact', label: 'Bob', health_band: 'chanterelle' },
      { id: 'c-3', type: 'contact', label: 'Carol', health_band: 'russula' },
    ],
    edges: [],
  };

  render(
    <NetworkGraph
      data={data}
      onNodeClick={vi.fn()}
      showRelationships
      showActivities
      showCircles={false}
    />,
  );

  expect(capturedNodeCanvasObject).toBeDefined();

  const moss = createTrackingCtx();
  capturedNodeCanvasObject?.(data.nodes[0], moss.ctx, 1);
  expect(moss.fillStyles[0]).toBe(theme.palette.success.main);

  const chanterelle = createTrackingCtx();
  capturedNodeCanvasObject?.(data.nodes[1], chanterelle.ctx, 1);
  expect(chanterelle.fillStyles[0]).toBe(theme.palette.warning.main);

  const russula = createTrackingCtx();
  capturedNodeCanvasObject?.(data.nodes[2], russula.ctx, 1);
  expect(russula.fillStyles[0]).toBe(theme.palette.error.main);
});

test('falls back to the old uniform node color when health_band is absent', () => {
  mockMatchMedia(false);
  const theme = createTheme();
  const data: GraphData = {
    nodes: [{ id: 'c-1', type: 'contact', label: 'Alice' }],
    edges: [],
  };

  render(
    <NetworkGraph
      data={data}
      onNodeClick={vi.fn()}
      showRelationships
      showActivities
      showCircles={false}
    />,
  );

  expect(capturedNodeCanvasObject).toBeDefined();

  const fallback = createTrackingCtx();
  capturedNodeCanvasObject?.(data.nodes[0], fallback.ctx, 1);
  expect(fallback.fillStyles[0]).toBe(theme.palette.primary.main);
});

test('does not color activity nodes by health_band even if one were present', () => {
  mockMatchMedia(false);
  const theme = createTheme();
  const data: GraphData = {
    nodes: [
      // health_band should never appear on an activity node per the backend
      // contract, but the render path must not accidentally key off it if it
      // did -- activities always use activityNodeColor (secondary.main).
      { id: 'a-1', type: 'activity', label: 'Dinner', health_band: 'moss' } as GraphNode,
    ],
    edges: [],
  };

  render(
    <NetworkGraph
      data={data}
      onNodeClick={vi.fn()}
      showRelationships
      showActivities
      showCircles={false}
    />,
  );

  expect(capturedNodeCanvasObject).toBeDefined();

  const ctx = createTrackingCtx();
  capturedNodeCanvasObject?.(data.nodes[0], ctx.ctx, 1);
  expect(ctx.fillStyles[0]).toBe(theme.palette.secondary.main);
});
