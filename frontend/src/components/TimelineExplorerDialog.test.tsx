import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { getTimeline, type TimelineItem } from '../api/timeline';
import { DateFormatProvider } from '../DateFormatProvider';
import TimelineExplorerDialog from './TimelineExplorerDialog';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(cleanup);

vi.mock('../api/timeline', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/timeline')>();
  return { ...actual, getTimeline: vi.fn() };
});

beforeEach(() => {
  vi.mocked(getTimeline).mockReset();
});

const noteItem = (overrides: Partial<TimelineItem> = {}): TimelineItem => ({
  type: 'note',
  id: '1',
  date: '2026-08-12T10:00:00Z',
  data: {
    ID: 1,
    content: 'First note',
    date: '2026-08-12T10:00:00Z',
    CreatedAt: '2026-08-12T10:00:00Z',
    UpdatedAt: '2026-08-12T10:00:00Z',
  },
  ...overrides,
});

const giftItem = (overrides: Partial<TimelineItem> = {}): TimelineItem => ({
  type: 'gift',
  id: 'gift-1',
  date: '2026-08-11T10:00:00Z',
  data: {
    id: 'gift-1',
    created_at: '2026-08-11T10:00:00Z',
    updated_at: '2026-08-11T10:00:00Z',
    entity_id: 'uid-1',
    status: 'given',
    description: 'The scarf',
    date: '2026-08-11T10:00:00Z',
  },
  ...overrides,
});

function renderDialog(
  overrides: Partial<React.ComponentProps<typeof TimelineExplorerDialog>> = {},
) {
  return render(
    <MemoryRouter>
      <DateFormatProvider>
        <TimelineExplorerDialog
          open
          onClose={vi.fn()}
          contactId={42}
          onEditItem={vi.fn()}
          onDeleteCompletion={vi.fn()}
          revision={0}
          {...overrides}
        />
      </DateFormatProvider>
    </MemoryRouter>,
  );
}

test('fetches the timeline when opened and renders the rows', async () => {
  vi.mocked(getTimeline).mockResolvedValue({
    items: [noteItem(), giftItem()],
    next_cursor: '',
    limit: 25,
  });

  renderDialog();

  await waitFor(() => {
    expect(screen.getByText('First note')).toBeInTheDocument();
  });
  expect(screen.getByText('The scarf')).toBeInTheDocument();
  expect(vi.mocked(getTimeline)).toHaveBeenCalledTimes(1);
  expect(vi.mocked(getTimeline)).toHaveBeenCalledWith({
    contactId: 42,
    types: expect.any(Array),
    bucket: 'all',
  });
});

test('renders an empty state (not a crash) when the endpoint returns no items', async () => {
  vi.mocked(getTimeline).mockResolvedValue({ items: [], next_cursor: '', limit: 25 });

  renderDialog();

  await waitFor(() => {
    expect(screen.getByText('No timeline events')).toBeInTheDocument();
  });
});

test('changing the type filter refetches with the new subset', async () => {
  vi.mocked(getTimeline).mockResolvedValue({ items: [], next_cursor: '', limit: 25 });

  renderDialog();
  await waitFor(() => expect(screen.getByText('No timeline events')).toBeInTheDocument());

  const typeSelect = screen.getByRole('combobox', { name: 'Event type' });
  fireEvent.mouseDown(typeSelect);
  // Uncheck "gift" by clicking its menu option (the checkbox toggles via the
  // ListItemText as well).
  fireEvent.click(screen.getByRole('option', { name: /^Gift$/ }));
  fireEvent.keyDown(typeSelect, { key: 'Escape' });

  await waitFor(() => {
    const lastCall = vi.mocked(getTimeline).mock.calls.at(-1)![0];
    expect(lastCall.types).toEqual([
      'note',
      'activity',
      'completion',
      'life_event',
      'external_activity',
    ]);
  });
});

test('changing the recency bucket refetches with the new token', async () => {
  vi.mocked(getTimeline).mockResolvedValue({ items: [], next_cursor: '', limit: 25 });

  renderDialog();
  await waitFor(() => expect(screen.getByText('No timeline events')).toBeInTheDocument());

  fireEvent.mouseDown(screen.getByRole('combobox', { name: 'How long ago' }));
  fireEvent.click(screen.getByRole('option', { name: /Last 7 days/ }));
  fireEvent.keyDown(screen.getByRole('combobox', { name: 'How long ago' }), { key: 'Escape' });

  await waitFor(() => {
    const lastCall = vi.mocked(getTimeline).mock.calls.at(-1)![0];
    expect(lastCall.bucket).toBe('last_7_days');
  });
});

test('loads the next cursor page via "Load more" instead of refetching from scratch', async () => {
  vi.mocked(getTimeline)
    .mockResolvedValueOnce({ items: [noteItem()], next_cursor: 'cursor-1', limit: 25 })
    .mockResolvedValueOnce({ items: [giftItem()], next_cursor: '', limit: 25 });

  renderDialog();

  await waitFor(() => expect(screen.getByText('First note')).toBeInTheDocument());
  fireEvent.click(screen.getByRole('button', { name: /Load more/ }));

  await waitFor(() => expect(screen.getByText('The scarf')).toBeInTheDocument());
  expect(vi.mocked(getTimeline)).toHaveBeenNthCalledWith(
    2,
    expect.objectContaining({ cursor: 'cursor-1' }),
  );
  // No "Load more" once the cursor is exhausted.
  expect(screen.queryByRole('button', { name: /Load more/ })).not.toBeInTheDocument();
});

test('routes edit/delete through the page-level handlers', async () => {
  vi.mocked(getTimeline).mockResolvedValue({
    items: [
      noteItem({
        type: 'completion',
        id: '5',
        data: { ID: 5, contact_id: 42, message: 'Call back', completed_at: '2026-08-12T10:00:00Z' },
      }),
    ],
    next_cursor: '',
    limit: 25,
  });
  const onDeleteCompletion = vi.fn();

  renderDialog({ onDeleteCompletion });

  await waitFor(() => expect(screen.getByText('Call back')).toBeInTheDocument());

  // The completion row's delete action is hover-gated via opacity, but
  // fireEvent clicks regardless of visibility.
  fireEvent.click(screen.getByLabelText('Delete'));
  expect(onDeleteCompletion).toHaveBeenCalledWith(5);
});

test('does not fetch while closed', () => {
  renderDialog({ open: false });
  expect(vi.mocked(getTimeline)).not.toHaveBeenCalled();
});
