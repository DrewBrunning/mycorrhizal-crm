import { cleanup, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { Circle } from '../api/circles';
import { DateFormatProvider, setDateFormatPreference } from '../DateFormatProvider';
import CircleDetailDialog from './CircleDetailDialog';

const { getCircleTimeline } = vi.hoisted(() => ({
  getCircleTimeline: vi.fn(),
}));
vi.mock('../api/circles', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/circles')>()),
  getCircleTimeline,
}));

const circle: Circle = {
  id: 'c1',
  name: 'Book club',
  description: '',
  color: '',
} as Circle;

afterEach(cleanup);

beforeEach(() => {
  setDateFormatPreference('eu');
  getCircleTimeline.mockReset();
});

function renderDialog() {
  render(
    <MemoryRouter>
      <DateFormatProvider>
        <CircleDetailDialog circle={circle} memberCount={1} onClose={vi.fn()} onSave={vi.fn()} />
      </DateFormatProvider>
    </MemoryRouter>,
  );
}

describe('CircleDetailDialog timeline dates', () => {
  // v1.6.0 review: the circle timeline used the browser locale instead of the
  // user's date-format preference that every other timeline honours.
  test('a note date follows the user date-format preference', async () => {
    getCircleTimeline.mockResolvedValue({
      items: [
        {
          id: 'n1',
          type: 'note',
          date: '2026-12-24',
          data: { content: 'Gift ideas' },
        },
      ],
      next_cursor: '',
      limit: 25,
      member_count: 1,
    });
    renderDialog();
    expect(await screen.findByText('Gift ideas')).toBeTruthy();
    expect(screen.getByText(/24\.12\.2026/)).toBeTruthy();
  });

  test('an activity shows its date range in the preference format', async () => {
    getCircleTimeline.mockResolvedValue({
      items: [
        {
          id: 'a1',
          type: 'activity',
          date: '2026-12-24',
          data: { title: 'Ski trip', end_date: '2026-12-27' },
        },
      ],
      next_cursor: '',
      limit: 25,
      member_count: 1,
    });
    renderDialog();
    expect(await screen.findByText('Ski trip')).toBeTruthy();
    expect(screen.getByText(/24\.12\.2026.*27\.12\.2026/)).toBeTruthy();
  });
});

describe('CircleDetailDialog timeline members', () => {
  // Issue #2361: each entry names the member(s) it is about, linking to them.
  test('each entry names its member(s) as links to the contact', async () => {
    getCircleTimeline.mockResolvedValue({
      items: [
        {
          id: 'n1',
          type: 'note',
          date: '2026-12-24',
          data: { content: 'Called about the move' },
          members: [{ id: 7, firstname: 'Alice', lastname: 'Smith' }],
        },
        {
          id: 'a1',
          type: 'activity',
          date: '2026-12-23',
          data: { title: 'Bouldering' },
          members: [
            { id: 7, firstname: 'Alice', lastname: 'Smith' },
            { id: 8, firstname: 'Bob', lastname: '' },
          ],
        },
      ],
      next_cursor: '',
      limit: 25,
      member_count: 2,
    });
    renderDialog();
    expect(await screen.findByText('Called about the move')).toBeTruthy();
    const alice = screen.getAllByRole('link', { name: 'Alice Smith' });
    expect(alice).toHaveLength(2);
    expect(alice[0].getAttribute('href')).toBe('/contacts/7');
    expect(screen.getByRole('link', { name: 'Bob' }).getAttribute('href')).toBe('/contacts/8');
  });
});
