import { cleanup, render, screen } from '@testing-library/react';
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
    <DateFormatProvider>
      <CircleDetailDialog circle={circle} memberCount={1} onClose={vi.fn()} onSave={vi.fn()} />
    </DateFormatProvider>,
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
