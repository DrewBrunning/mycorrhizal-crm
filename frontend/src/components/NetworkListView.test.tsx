import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import '../i18n/config';
import NetworkListView from './NetworkListView';
import { computeFilteredGraphData } from '../utils/networkGraphData';
import { GraphData } from '../types/graph';

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
    edges: [
      { id: 'e-1', source: 'c-1', target: 'c-2', type: 'relationship', label: 'friend_of' },
    ],
  };
}

test('renders one entry per contact node, with connection text derived from the relationship edge', () => {
  const filtered = computeFilteredGraphData(sampleData(), {
    showRelationships: true,
    showActivities: true,
    showCircles: false,
  });

  render(<NetworkListView nodes={filtered.nodes} links={filtered.links} onContactClick={vi.fn()} />);

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

  render(<NetworkListView nodes={filtered.nodes} links={filtered.links} onContactClick={vi.fn()} />);

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

  render(<NetworkListView nodes={filtered.nodes} links={filtered.links} onContactClick={onContactClick} />);
  screen.getByRole('button', { name: 'Alice' }).click();

  expect(onContactClick).toHaveBeenCalledWith(expect.objectContaining({ id: 'c-1', label: 'Alice' }));
});
