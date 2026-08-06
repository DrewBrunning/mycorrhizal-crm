import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import '../i18n/config';
import LifeEventList from './LifeEventList';
import { LifeEvent } from '../api/lifeEvents';

afterEach(cleanup);

function baseEvent(overrides: Partial<LifeEvent> = {}): LifeEvent {
  return {
    id: 'evt-1',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    entity_id: 'contact-1',
    type: 'married',
    ...overrides,
  };
}

function renderList(events: LifeEvent[]) {
  return render(
    <LifeEventList
      events={events}
      contactsByUid={new Map()}
      onEdit={vi.fn()}
      onDelete={vi.fn()}
    />
  );
}

test('shows the empty state when there are no events', () => {
  renderList([]);
  expect(screen.getByText('No life events yet')).toBeInTheDocument();
});

// T36: a backfilled event's category renders as a chip alongside its type
// label — the visible confirmation that a pre-migration `type: married` row
// now shows under Family & Relationships after the migration's backfill.
test('renders a category chip for a categorized event', () => {
  renderList([baseEvent({ category: 'family_relationships' })]);

  expect(screen.getByText('Married')).toBeInTheDocument();
  expect(screen.getByText('Family & Relationships')).toBeInTheDocument();
});

// The Edit/Delete IconButtons had no aria-label (unlike RelationshipEdgeList's
// equivalents), which also made them untargetable by an e2e edit flow.
test('edit and delete actions are reachable by an accessible label', () => {
  renderList([baseEvent()]);
  expect(screen.getByLabelText('Edit')).toBeInTheDocument();
  expect(screen.getByLabelText('Delete')).toBeInTheDocument();
});

test('omits the category chip for an uncategorized (pre-T36) event', () => {
  renderList([baseEvent({ type: 'started a podcast', category: undefined })]);

  expect(screen.getByText('started a podcast')).toBeInTheDocument();
  // None of the five real category labels, nor the pseudo "uncategorized"
  // label, should render — there is simply no chip for a missing category.
  expect(screen.queryByText('Family & Relationships')).not.toBeInTheDocument();
  expect(screen.queryByText('Other / Uncategorized')).not.toBeInTheDocument();
});
