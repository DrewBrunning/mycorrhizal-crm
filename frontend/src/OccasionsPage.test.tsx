import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import './i18n/config';
import type { OccasionEvent } from './api/occasionEvents';
import {
  downloadOccasionCardListCSV,
  type GiftShoppingItem,
  getGiftShoppingList,
} from './api/occasionObligations';
import { useOccasionEventAttendees, useOccasionEvents } from './hooks/useOccasionEvents';
import OccasionsPage from './OccasionsPage';

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

vi.mock('./api/occasionObligations', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/occasionObligations')>();
  return {
    ...actual,
    getGiftShoppingList: vi.fn(),
    downloadOccasionCardListCSV: vi.fn(),
  };
});

vi.mock('./hooks/useOccasionEvents', () => ({
  useOccasionEvents: vi.fn(),
  useOccasionEventAttendees: vi.fn(),
}));

const getGiftShoppingListMock = vi.mocked(getGiftShoppingList);
const downloadMock = vi.mocked(downloadOccasionCardListCSV);
const useOccasionEventsMock = vi.mocked(useOccasionEvents);

const sampleEvent: OccasionEvent = {
  id: 'ev-1',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  title: 'Summer BBQ',
  starts_at: '2026-07-04T15:00:00Z',
  sensitivity: 'normal',
};

beforeEach(() => {
  getGiftShoppingListMock.mockReset();
  downloadMock.mockReset();
  getGiftShoppingListMock.mockResolvedValue({ gift_shopping_list: [], days: 30 });
  downloadMock.mockResolvedValue(undefined);
  useOccasionEventsMock.mockReturnValue({
    events: [],
    loading: false,
    error: null,
    refresh: vi.fn(),
    handleSave: vi.fn(),
    handleDelete: vi.fn(),
  });
  vi.mocked(useOccasionEventAttendees).mockReturnValue({
    attendees: [],
    loading: false,
    error: null,
    refresh: vi.fn(),
    handleAdd: vi.fn(),
    handleUpdateRsvp: vi.fn(),
    handleRemove: vi.fn(),
    suggestions: [],
    suggestionsLoading: false,
    loadSuggestions: vi.fn(),
    clearSuggestions: vi.fn(),
  });
});

function renderPage() {
  return render(
    <MemoryRouter>
      <OccasionsPage />
    </MemoryRouter>,
  );
}

test('fetches and shows the empty gift shopping list on mount', async () => {
  renderPage();
  await waitFor(() => expect(getGiftShoppingListMock).toHaveBeenCalledWith({ days: 30 }));
  expect(await screen.findByText('No gifts needed in this window.')).toBeInTheDocument();
});

test('renders a gift shopping item with its contact, label, and status', async () => {
  const items: GiftShoppingItem[] = [
    {
      contact_id: 7,
      contact_name: 'Alice',
      obligation_id: 'ob-1',
      label: 'Christmas card',
      date: '2026-12-25',
      days_until: 5,
      status: 'needed',
    },
  ];
  getGiftShoppingListMock.mockResolvedValue({ gift_shopping_list: items, days: 30 });
  renderPage();

  expect(await screen.findByText('Alice — Christmas card')).toBeInTheDocument();
  expect(screen.getByText('in 5 days')).toBeInTheDocument();
  expect(screen.getByText('Needed')).toBeInTheDocument();
});

test('shows "Today" for an item due today', async () => {
  const items: GiftShoppingItem[] = [
    {
      contact_id: 8,
      contact_name: 'Bob',
      obligation_id: 'ob-2',
      label: 'Birthday gift',
      date: '2026-09-24',
      days_until: 0,
      status: 'idea',
    },
  ];
  getGiftShoppingListMock.mockResolvedValue({ gift_shopping_list: items, days: 30 });
  renderPage();

  expect(await screen.findByText('Today')).toBeInTheDocument();
});

test('re-fetches with days=90 when the 90-day toggle is clicked', async () => {
  renderPage();
  await waitFor(() => expect(getGiftShoppingListMock).toHaveBeenCalledWith({ days: 30 }));

  fireEvent.click(screen.getByRole('button', { name: '90 days' }));

  await waitFor(() => expect(getGiftShoppingListMock).toHaveBeenCalledWith({ days: 90 }));
});

test('shows an error message when the gift shopping list fetch fails', async () => {
  getGiftShoppingListMock.mockRejectedValue(new Error('boom'));
  renderPage();

  expect(await screen.findByText(/boom/i)).toBeInTheDocument();
});

test('downloads the card list CSV when the download button is clicked', async () => {
  renderPage();
  await waitFor(() => expect(getGiftShoppingListMock).toHaveBeenCalled());

  fireEvent.click(screen.getByRole('button', { name: /download csv/i }));

  await waitFor(() => expect(downloadMock).toHaveBeenCalledWith({ kind: 'card' }));
});

test('shows an error message when the card list download fails', async () => {
  downloadMock.mockRejectedValue(new Error('download boom'));
  renderPage();
  await waitFor(() => expect(getGiftShoppingListMock).toHaveBeenCalled());

  fireEvent.click(screen.getByRole('button', { name: /download csv/i }));

  expect(await screen.findByText('Failed to download the card list.')).toBeInTheDocument();
});

test('shows the events empty state when there are no events', () => {
  renderPage();
  expect(screen.getByRole('heading', { name: 'Events' })).toBeInTheDocument();
  expect(
    screen.getByText('No events yet. Create one to start planning who to invite.'),
  ).toBeInTheDocument();
});

test('renders an event and opens the create dialog', async () => {
  useOccasionEventsMock.mockReturnValue({
    events: [sampleEvent],
    loading: false,
    error: null,
    refresh: vi.fn(),
    handleSave: vi.fn(),
    handleDelete: vi.fn(),
  });
  renderPage();

  expect(screen.getByText('Summer BBQ')).toBeInTheDocument();

  fireEvent.click(screen.getByRole('button', { name: 'New event' }));
  expect(await screen.findByText('Create an event')).toBeInTheDocument();
});

test('opens the attendees dialog from an event row', async () => {
  useOccasionEventsMock.mockReturnValue({
    events: [sampleEvent],
    loading: false,
    error: null,
    refresh: vi.fn(),
    handleSave: vi.fn(),
    handleDelete: vi.fn(),
  });
  renderPage();

  fireEvent.click(screen.getByRole('button', { name: 'Attendees' }));
  expect(await screen.findByText('Attendees — Summer BBQ')).toBeInTheDocument();
});

test('shows an events fetch error', () => {
  useOccasionEventsMock.mockReturnValue({
    events: [],
    loading: false,
    error: 'events boom',
    refresh: vi.fn(),
    handleSave: vi.fn(),
    handleDelete: vi.fn(),
  });
  renderPage();
  expect(screen.getByText('events boom')).toBeInTheDocument();
});
