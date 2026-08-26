import { cleanup, render, screen } from '@testing-library/react';
import { forwardRef, useImperativeHandle } from 'react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { GraphData } from '../types/graph';
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

vi.mock('react-force-graph-2d', () => ({
  default: forwardRef((_props: unknown, ref: React.Ref<unknown>) => {
    useImperativeHandle(ref, () => ({
      zoom: zoomFn,
      centerAt: centerAtFn,
      zoomToFit: zoomToFitFn,
      d3Force: d3ForceFn,
    }));
    return <div data-testid="force-graph-stub" />;
  }),
}));

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
