import { cleanup, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { DateFormatProvider, setDateFormatPreference } from '../DateFormatProvider';
import ContactTimeline from './ContactTimeline';

// The gallery is irrelevant to date rendering; left real, its image fetch can
// settle after the test environment is torn down and its late setState surfaces
// as an unhandled "window is not defined" rejection that fails the whole run
// (issue #2897). Same guard as calendarDayRendering.test.tsx.
vi.mock('./ImageGallery', async (importActual) => ({
  ...(await importActual<typeof import('./ImageGallery')>()),
  default: () => null,
}));

// Issue #2006: a multi-day activity shows its range in the user's date format.

afterEach(() => {
  cleanup();
  setDateFormatPreference('eu');
  localStorage.clear();
});

function renderActivity(endDate?: string) {
  return render(
    <MemoryRouter>
      <DateFormatProvider>
        <ContactTimeline
          timelineItems={[
            {
              type: 'activity',
              date: '2026-05-12T00:00:00Z',
              data: {
                ID: 7,
                title: 'Conference',
                description: '',
                location: '',
                date: '2026-05-12T00:00:00Z',
                ...(endDate ? { end_date: endDate } : {}),
                CreatedAt: '2026-05-01T00:00:00Z',
                UpdatedAt: '2026-05-01T00:00:00Z',
              },
            },
          ]}
          onEditItem={vi.fn()}
        />
      </DateFormatProvider>
    </MemoryRouter>,
  );
}

test('a multi-day activity shows its range in both date renders', () => {
  renderActivity('2026-05-15T00:00:00Z');
  expect(screen.getAllByText('12.05.2026 – 15.05.2026')).toHaveLength(2);
});

test('the range follows the date-format preference', () => {
  setDateFormatPreference('us-mmm');
  renderActivity('2026-05-15T00:00:00Z');
  expect(screen.getAllByText('May 12–15, 2026')).toHaveLength(2);
});

test('a single-day activity still shows one date', () => {
  renderActivity();
  expect(screen.getAllByText('12.05.2026')).toHaveLength(2);
});
