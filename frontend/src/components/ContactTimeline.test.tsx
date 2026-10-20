import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { Gift } from '../api/gifts';
import { DateFormatProvider } from '../DateFormatProvider';
import ContactTimeline from './ContactTimeline';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(cleanup);

const givenGift: Gift = {
  id: 'gift-1',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  entity_id: 'alice-uid',
  status: 'given',
  description: 'The espresso machine',
  occasion: 'birthday',
  date: '2026-08-03T12:00:00Z',
};

function renderTimeline(items: React.ComponentProps<typeof ContactTimeline>['timelineItems']) {
  return render(
    <MemoryRouter>
      <DateFormatProvider>
        <ContactTimeline timelineItems={items} onEditItem={vi.fn()} />
      </DateFormatProvider>
    </MemoryRouter>,
  );
}

// #196 (audit gap N-1): a timeline item's edit/delete icon was revealed only
// on `&:hover .action-icon` -- no `:focus-within` -- so keyboard focus landed
// on a fully transparent control.
test('edit/delete action icon is keyboard-revealable', () => {
  renderTimeline([
    {
      type: 'note',
      date: '2026-01-01T00:00:00Z',
      data: {
        ID: 1,
        content: 'Met at the conference',
        date: '2026-01-01T00:00:00Z',
        CreatedAt: '2026-01-01T00:00:00Z',
        UpdatedAt: '2026-01-01T00:00:00Z',
      },
    },
  ]);

  const css = Array.from(document.querySelectorAll('style'))
    .map((s) => s.textContent || '')
    .join('\n');
  expect(css).toContain(':focus-within .action-icon');
});

test('renders a given gift with its label, description, and occasion', () => {
  renderTimeline([{ type: 'gift', data: givenGift, date: givenGift.date! }]);

  expect(screen.getByText('Gift given')).toBeInTheDocument();
  expect(screen.getByText('The espresso machine')).toBeInTheDocument();
  expect(screen.getByText('birthday')).toBeInTheDocument();
});

test('renders a received gift with its own label', () => {
  renderTimeline([
    {
      type: 'gift',
      data: { ...givenGift, status: 'received', description: 'The scarf' },
      date: givenGift.date!,
    },
  ]);

  expect(screen.getByText('Gift received')).toBeInTheDocument();
  expect(screen.getByText('The scarf')).toBeInTheDocument();
});

test('a gift without an occasion does not render the occasion line', () => {
  const { description, ...rest } = givenGift;
  renderTimeline([
    { type: 'gift', data: { ...rest, description, occasion: undefined }, date: givenGift.date! },
  ]);

  expect(screen.getByText('Gift given')).toBeInTheDocument();
  expect(screen.getByText(description)).toBeInTheDocument();
  expect(screen.queryByText('birthday')).not.toBeInTheDocument();
});

// #1783: an activity with no description renders no body text -- not a
// "No description" placeholder on every such card -- and each card carries a
// date caption of its own (the phone layout hides the opposite-content date
// column, so the caption is what shows the date below `sm`).
test('an activity without a description renders no placeholder and carries a date caption', () => {
  renderTimeline([
    {
      type: 'activity',
      date: '2026-08-03T12:00:00Z',
      data: {
        ID: 7,
        title: 'Dinner with Nadia',
        description: '',
        location: '',
        date: '2026-08-03T12:00:00Z',
        CreatedAt: '2026-08-03T12:00:00Z',
        UpdatedAt: '2026-08-03T12:00:00Z',
      },
    },
  ]);

  expect(screen.getByText('Dinner with Nadia')).toBeInTheDocument();
  expect(screen.queryByText('No description')).not.toBeInTheDocument();
  // The date appears twice: the opposite-content column and the per-card
  // caption shown below `sm`.
  expect(screen.getAllByText(/2026/)).toHaveLength(2);
});

// #1783: the body text is now computed once per item; notes and completed
// reminders still show their own text, and a blank one renders no empty line.
test('notes and completed reminders render their body text; a blank note renders none', () => {
  renderTimeline([
    {
      type: 'note',
      date: '2026-08-03T12:00:00Z',
      data: {
        ID: 1,
        content: 'Mentioned their new job',
        date: '2026-08-03T12:00:00Z',
        CreatedAt: '2026-08-03T12:00:00Z',
        UpdatedAt: '2026-08-03T12:00:00Z',
      },
    },
    {
      type: 'completion',
      date: '2026-08-02T12:00:00Z',
      data: {
        ID: 2,
        contact_id: 1,
        message: 'Called back',
        completed_at: '2026-08-02T12:00:00Z',
      },
    },
    {
      type: 'note',
      date: '2026-08-01T12:00:00Z',
      data: {
        ID: 3,
        content: '',
        date: '2026-08-01T12:00:00Z',
        CreatedAt: '2026-08-01T12:00:00Z',
        UpdatedAt: '2026-08-01T12:00:00Z',
      },
    },
  ]);

  expect(screen.getByText('Mentioned their new job')).toBeInTheDocument();
  expect(screen.getByText('Called back')).toBeInTheDocument();
  expect(document.querySelectorAll('p.MuiTypography-body2')).toHaveLength(2);
});

test('an activity shows its location and linked contacts; edit and delete actions fire', () => {
  const onEditItem = vi.fn();
  const onDeleteCompletion = vi.fn();
  render(
    <MemoryRouter>
      <DateFormatProvider>
        <ContactTimeline
          onEditItem={onEditItem}
          onDeleteCompletion={onDeleteCompletion}
          timelineItems={[
            {
              type: 'activity',
              date: 'not-a-date',
              data: {
                ID: 9,
                title: 'Hike',
                description: 'Up the hill',
                location: 'Ridge trail',
                date: 'not-a-date',
                contacts: [
                  { ID: 1, firstname: 'Ada', lastname: 'Lovelace', nickname: 'Countess' },
                  { ID: 2, firstname: 'Bob', lastname: 'Builder' },
                ],
                CreatedAt: '2026-08-03T12:00:00Z',
                UpdatedAt: '2026-08-03T12:00:00Z',
              } as never,
            },
            {
              type: 'completion',
              date: '2026-08-02T12:00:00Z',
              data: { ID: 5, contact_id: 1, message: 'Called back', completed_at: '2026-08-02' },
            },
          ]}
        />
      </DateFormatProvider>
    </MemoryRouter>,
  );

  expect(screen.getByText(/Ridge trail/)).toBeInTheDocument();
  expect(screen.getByText(/Countess/)).toBeInTheDocument();
  expect(screen.getByText(/Bob\s+Builder/)).toBeInTheDocument();

  fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
  expect(onEditItem).toHaveBeenCalledWith('activity', expect.objectContaining({ ID: 9 }));
  fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
  expect(onDeleteCompletion).toHaveBeenCalledWith(5);
});

test('an external activity shows its source and the date caption', () => {
  renderTimeline([
    {
      type: 'external_activity',
      date: '2026-08-03T12:00:00Z',
      data: {
        id: 'ext-1',
        type: 'photo-appearance',
        source_system: 'immich',
        payload: { person_name: 'Ada' },
      } as never,
    },
  ]);

  expect(screen.getAllByText(/2026/).length).toBeGreaterThanOrEqual(2);
});

test('a life event shows its date caption alongside the date column', () => {
  renderTimeline([
    {
      type: 'life_event',
      date: '2026-08-03T12:00:00Z',
      data: {
        id: 'le-1',
        created_at: '2026-08-03T12:00:00Z',
        updated_at: '2026-08-03T12:00:00Z',
        entity_id: 'alice-uid',
        type: 'birth',
        date: { year: 2026, month: 8, day: 3 },
        description: 'A new arrival',
      },
    },
  ]);

  expect(screen.getByText('A new arrival')).toBeInTheDocument();
  expect(screen.getAllByText(/2026/).length).toBeGreaterThanOrEqual(2);
});
