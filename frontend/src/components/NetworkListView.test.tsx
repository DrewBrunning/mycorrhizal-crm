import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { GraphData } from '../types/graph';
import { computeFilteredGraphData } from '../utils/networkGraphData';
import NetworkListView from './NetworkListView';

// This codebase's vitest setup does not auto-cleanup between tests -- see
// RelationshipEdgeList.test.tsx's matching comment.
afterEach(cleanup);

function sampleData(): GraphData {
  return {
    nodes: [
      { id: 'c-1', type: 'contact', label: 'Alice' },
      { id: 'c-2', type: 'contact', label: 'Bob' },
      { id: 'c-3', type: 'contact', label: 'Carol' },
    ],
    edges: [{ id: 'e-1', source: 'c-1', target: 'c-2', type: 'relationship', label: 'friend_of' }],
  };
}

test('renders one entry per contact node, with connection text derived from the relationship edge', () => {
  const filtered = computeFilteredGraphData(sampleData(), {
    showRelationships: true,
    showActivities: true,
    showCircles: false,
  });

  render(
    <NetworkListView nodes={filtered.nodes} links={filtered.links} onContactClick={vi.fn()} />,
  );

  expect(screen.getByRole('button', { name: 'Alice' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Bob' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Carol' })).toBeInTheDocument();

  // friend_of is symmetric, so both sides read "Friend: <other>".
  expect(screen.getByText(/Friend: Bob/)).toBeInTheDocument();
  expect(screen.getByText(/Friend: Alice/)).toBeInTheDocument();
});

test('honours selectedCircle: only contacts in the selected circle are listed', () => {
  const circleNamesByUid = new Map<string, string[]>([
    ['1', ['Book Club']],
    ['2', ['Book Club']],
    // Carol (contact ID 3) is deliberately not a member of any circle.
  ]);

  const filtered = computeFilteredGraphData(sampleData(), {
    selectedCircle: 'Book Club',
    showRelationships: true,
    showActivities: true,
    showCircles: false,
    circleNamesByUid,
  });

  render(
    <NetworkListView nodes={filtered.nodes} links={filtered.links} onContactClick={vi.fn()} />,
  );

  expect(screen.getByRole('button', { name: 'Alice' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Bob' })).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Carol' })).not.toBeInTheDocument();
});

test('clicking a contact entry invokes onContactClick with that node', () => {
  const filtered = computeFilteredGraphData(sampleData(), {
    showRelationships: true,
    showActivities: true,
    showCircles: false,
  });
  const onContactClick = vi.fn();

  render(
    <NetworkListView
      nodes={filtered.nodes}
      links={filtered.links}
      onContactClick={onContactClick}
    />,
  );
  screen.getByRole('button', { name: 'Alice' }).click();

  expect(onContactClick).toHaveBeenCalledWith(
    expect.objectContaining({ id: 'c-1', label: 'Alice' }),
  );
});

// --- Issue #383/ADR-0023: relationship health indicator, dot + text -------
// (WCAG 1.4.1 Use of Color -- the dot alone must never be the only signal).

function dataWithHealthBands(): GraphData {
  return {
    nodes: [
      { id: 'c-1', type: 'contact', label: 'Alice', health_band: 'moss' },
      { id: 'c-2', type: 'contact', label: 'Bob', health_band: 'chanterelle' },
      { id: 'c-3', type: 'contact', label: 'Carol', health_band: 'russula' },
      { id: 'c-4', type: 'contact', label: 'Dave' }, // no band -- no indicator
    ],
    edges: [],
  };
}

test('shows a non-color text indicator of the health band alongside each contact, not color alone', () => {
  const filtered = computeFilteredGraphData(dataWithHealthBands(), {
    showRelationships: true,
    showActivities: true,
    showCircles: false,
  });

  render(
    <NetworkListView nodes={filtered.nodes} links={filtered.links} onContactClick={vi.fn()} />,
  );

  expect(screen.getByText('(Healthy)')).toBeInTheDocument();
  expect(screen.getByText('(Needs Attention)')).toBeInTheDocument();
  expect(screen.getByText('(Neglected)')).toBeInTheDocument();
  // Dave has no health_band -- no band text at all, not a default label, and
  // the button's accessible name stays exactly "Dave" (no stray "(undefined)").
  expect(screen.getByRole('button', { name: 'Dave' })).toBeInTheDocument();
});

test('renders a colored dot per health band', () => {
  const filtered = computeFilteredGraphData(dataWithHealthBands(), {
    showRelationships: true,
    showActivities: true,
    showCircles: false,
  });

  const { container } = render(
    <NetworkListView nodes={filtered.nodes} links={filtered.links} onContactClick={vi.fn()} />,
  );

  // The dot is a decorative aria-hidden span with an inline background color.
  const dots = container.querySelectorAll('span[aria-hidden="true"]');
  expect(dots.length).toBe(3); // Alice, Bob, Carol -- not Dave (no band)

  const dotColors = Array.from(dots).map((el) => (el as HTMLElement).style.backgroundColor);
  // All three bands resolve to distinct, non-empty colors.
  expect(new Set(dotColors).size).toBe(3);
  for (const c of dotColors) {
    expect(c).not.toBe('');
  }
});
