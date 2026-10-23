import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { ReachOutSuggestion } from '../api/reachOutSuggestions';
import { DateFormatProvider } from '../DateFormatProvider';
import ReachOutSuggestionsList from './ReachOutSuggestionsList';

// This codebase's vitest has no auto-cleanup (CLAUDE.md frontend trap #1).
afterEach(cleanup);

function suggestion(overrides: Partial<ReachOutSuggestion> = {}): ReachOutSuggestion {
  return {
    id: 'suggestion-1',
    created_at: '2026-08-01T00:00:00Z',
    updated_at: '2026-08-01T00:00:00Z',
    contact_vcard_uid: 'alice-uid',
    kind: 'organization',
    old_value: 'OldCo',
    new_value: 'NewCo',
    audit_event_id: 1,
    status: 'pending',
    contact_id: 7,
    contact_name: 'Alice Smith',
    ...overrides,
  };
}

function renderList(props: Partial<React.ComponentProps<typeof ReachOutSuggestionsList>> = {}) {
  const defaults: React.ComponentProps<typeof ReachOutSuggestionsList> = {
    suggestions: [],
    loading: false,
    error: null,
    onDismiss: vi.fn(),
    ...props,
  };
  return render(
    <DateFormatProvider>
      <MemoryRouter>
        <ReachOutSuggestionsList {...defaults} />
      </MemoryRouter>
    </DateFormatProvider>,
  );
}

test('renders each suggestion with the old->new value and a link to the contact', () => {
  renderList({ suggestions: [suggestion()] });

  expect(screen.getByText('Alice Smith')).toBeInTheDocument();
  expect(screen.getByText('OldCo → NewCo')).toBeInTheDocument();
  const link = screen.getByRole('link');
  expect(link.getAttribute('href')).toBe('/contacts/7');
  // #196: the decorative avatar initial must not double into the link name.
  expect(link).toHaveAccessibleName(/^Alice Smith/); // not "AAlice Smith"
});

test('shows the empty state when there are no suggestions', () => {
  renderList({ suggestions: [] });
  expect(screen.getByText('No reach-out suggestions right now.')).toBeInTheDocument();
});

test('calls onDismiss with the suggestion id and does not navigate', () => {
  const onDismiss = vi.fn();
  renderList({ suggestions: [suggestion({ id: 'suggestion-42' })], onDismiss });

  fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }));

  expect(onDismiss).toHaveBeenCalledWith('suggestion-42');
});

test('target_behind renders the behind-target caption with the formatted date', () => {
  window.localStorage.setItem('dateFormat', 'eu');
  renderList({
    suggestions: [
      suggestion({
        kind: 'target_behind',
        old_value: 'close',
        new_value: '2026-08-01T10:00:00Z',
      }),
    ],
  });

  expect(
    screen.getByText('Behind your Close target · last contact 01.08.2026'),
  ).toBeInTheDocument();
  expect(screen.getByText('Behind target')).toBeInTheDocument();
});

test('renders a kind chip labeled per the suggestion kind', () => {
  renderList({
    suggestions: [suggestion({ kind: 'address', old_value: '', new_value: '2 New Ave' })],
  });

  expect(screen.getByText('Moved')).toBeInTheDocument();
  expect(screen.getByText('Now: 2 New Ave')).toBeInTheDocument();
});

test('the section header uses a lightbulb icon, not an office building (issue #1761)', () => {
  renderList({ suggestions: [] });

  expect(screen.getByTestId('LightbulbOutlinedIcon')).toBeInTheDocument();
  expect(screen.queryByTestId('BusinessOutlinedIcon')).not.toBeInTheDocument();
});
