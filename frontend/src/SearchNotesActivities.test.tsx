import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import './i18n/config';
import type { SearchResult } from './api/search';
import SearchNotesActivities from './components/SearchNotesActivities';
import { DateFormatProvider } from './DateFormatProvider';

afterEach(cleanup);

const emptyResult: SearchResult = {
  query: 'x',
  resolved_relation: '',
  contacts: [],
  notes: [],
  activities: [],
};

function renderSection(result: SearchResult, onOpenContact?: (id: number) => void) {
  return render(
    <DateFormatProvider>
      <SearchNotesActivities query="x" result={result} onOpenContact={onOpenContact} />
    </DateFormatProvider>,
  );
}

test('renders nothing when there are no hits and no resolved relation', () => {
  const { container } = renderSection(emptyResult);
  expect(container).toBeEmptyDOMElement();
});

test('renders nothing when the result is null', () => {
  const { container } = renderSection(null as unknown as SearchResult);
  expect(container).toBeEmptyDOMElement();
});

test('surfaces the resolved relation info line even with no notes or activities', () => {
  renderSection({ ...emptyResult, resolved_relation: 'sibling_of' });
  expect(screen.getByText(/Matched relationship: Sibling/)).toBeInTheDocument();
});

test('shows the count header, collapsed by default', () => {
  renderSection({
    ...emptyResult,
    notes: [{ id: 1, content: 'secret note body', date: '2026-08-03T10:00:00Z' }],
    activities: [{ id: 9, title: 'A walk', date: '2026-08-01T19:00:00Z' }],
  });

  expect(screen.getByText('2 matches in notes and activities')).toBeInTheDocument();
  // Collapsed by default: the hit bodies are rendered but hidden.
  expect(screen.getByText('secret note body')).not.toBeVisible();
  expect(screen.getByText('A walk')).not.toBeVisible();
});

test('expanding reveals the note and activity hits', () => {
  renderSection({
    ...emptyResult,
    notes: [
      {
        id: 1,
        content: 'secret note body',
        date: '2026-08-03T10:00:00Z',
        contact_id: 7,
        contact_name: 'Wolfgang',
      },
    ],
    activities: [{ id: 9, title: 'A walk', date: '2026-08-01T19:00:00Z' }],
  });

  fireEvent.click(screen.getByText('2 matches in notes and activities'));

  expect(screen.getByText('secret note body')).toBeInTheDocument();
  expect(screen.getByText('A walk')).toBeInTheDocument();
});

test('clicking a note contact chip opens the contact', () => {
  const onOpenContact = vi.fn();
  renderSection(
    {
      ...emptyResult,
      notes: [
        {
          id: 1,
          content: 'secret note body',
          date: '2026-08-03T10:00:00Z',
          contact_id: 7,
          contact_name: 'Wolfgang',
        },
      ],
    },
    onOpenContact,
  );

  fireEvent.click(screen.getByText('1 matches in notes and activities'));
  fireEvent.click(screen.getByText('Wolfgang'));
  expect(onOpenContact).toHaveBeenCalledWith(7);
});

test('a note without a contact is labelled unfiled', () => {
  renderSection({
    ...emptyResult,
    notes: [{ id: 1, content: 'secret note body', date: '2026-08-03T10:00:00Z' }],
  });

  fireEvent.click(screen.getByText('1 matches in notes and activities'));
  expect(screen.getByText('Unfiled')).toBeInTheDocument();
});
