import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import './i18n/config';
import { ApiError } from './api/client';
import type { OccasionEvent } from './api/occasionEvents';
import {
  downloadOccasionCardListCSV,
  type GiftShoppingItem,
  getGiftShoppingList,
  getOccasionCardListSummary,
  type OccasionCardListSummary,
} from './api/occasionObligations';
import { useOccasionEventAttendees, useOccasionEvents } from './hooks/useOccasionEvents';
import OccasionsPage from './OccasionsPage';

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  vi.useRealTimers();
});

// Countdown labels follow the viewer's local day (issue #2679), so those tests pin it.
function pinToday(year: number, monthIndex: number, day: number) {
  vi.useFakeTimers({ toFake: ['Date'] });
  vi.setSystemTime(new Date(year, monthIndex, day, 12, 0));
}

vi.mock('./api/occasionObligations', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/occasionObligations')>();
  return {
    ...actual,
    getGiftShoppingList: vi.fn(),
    downloadOccasionCardListCSV: vi.fn(),
    getOccasionCardListSummary: vi.fn(),
  };
});

vi.mock('./hooks/useOccasionEvents', () => ({
  useOccasionEvents: vi.fn(),
  useOccasionEventAttendees: vi.fn(),
}));

const getGiftShoppingListMock = vi.mocked(getGiftShoppingList);
const downloadMock = vi.mocked(downloadOccasionCardListCSV);
const summaryMock = vi.mocked(getOccasionCardListSummary);

function summary(
  included: number,
  noAddress = 0,
  privateOnly = 0,
  total = included + noAddress + privateOnly,
): OccasionCardListSummary {
  return {
    total,
    included,
    skipped: noAddress + privateOnly,
    skipped_reasons: { no_address: noAddress, private_address_only: privateOnly },
  };
}
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
  summaryMock.mockReset();
  summaryMock.mockResolvedValue(summary(2));
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

test('explains what occasions are and how they differ from activities and reminders', async () => {
  renderPage();

  expect(
    await screen.findByText(
      'Occasions are the standing things you do for people, like a yearly card or gift, and one-off events with an invitee list. Active obligations create reminders automatically; reminders are the nudge, not a second occasion. To log something that already happened, use Activities.',
    ),
  ).toBeInTheDocument();
  expect(
    screen.getByText(
      'Download a CSV of contacts with an active card occasion: one row per active obligation, with the contact name, their normal-sensitivity address, and the occasion. Private and secret addresses are left out.',
    ),
  ).toBeInTheDocument();
});

test('fetches and shows the empty gift shopping list on mount', async () => {
  renderPage();
  await waitFor(() => expect(getGiftShoppingListMock).toHaveBeenCalledWith({ days: 30 }));
  expect(await screen.findByText('No gifts needed in this window.')).toBeInTheDocument();
});

test('renders a gift shopping item with its contact, label, and status', async () => {
  pinToday(2026, 11, 20);
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

test('shows "Today" for an item due today, by the viewer day not the server days_until', async () => {
  pinToday(2026, 8, 24);
  // The server's days_until may be a day ahead of the viewer's (issue #2679).
  const items: GiftShoppingItem[] = [
    {
      contact_id: 8,
      contact_name: 'Bob',
      obligation_id: 'ob-2',
      label: 'Birthday gift',
      date: '2026-09-24',
      days_until: 1,
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

// --- Card list summary (#2676 / #2786) ---

test('asks for the card-list summary of the same query the CSV uses', async () => {
  renderPage();
  await waitFor(() =>
    expect(summaryMock).toHaveBeenCalledWith({ kind: 'card', includeSensitive: false }),
  );
});

test('announces how many recipients the CSV will hold when none are skipped', async () => {
  summaryMock.mockResolvedValue(summary(2));
  renderPage();
  expect(await screen.findByRole('status')).toHaveTextContent('2 recipients');
  expect(screen.getByRole('status')).not.toHaveTextContent('skipped');
});

test('uses the singular for one recipient', async () => {
  summaryMock.mockResolvedValue(summary(1));
  renderPage();
  await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent(/^1 recipient$/));
});

test('breaks down who was skipped and why', async () => {
  summaryMock.mockResolvedValue(summary(3, 2, 1));
  renderPage();
  await waitFor(() =>
    expect(screen.getByRole('status')).toHaveTextContent(
      '3 recipients; 3 skipped (no postal address: 2, private address only: 1)',
    ),
  );
});

test('lists only the skip reasons that apply', async () => {
  summaryMock.mockResolvedValue(summary(1, 0, 4));
  renderPage();
  await waitFor(() =>
    expect(screen.getByRole('status')).toHaveTextContent(
      '1 recipient; 4 skipped (private address only: 4)',
    ),
  );
});

test('says nobody has a postal address and offers no download when nothing would be included', async () => {
  summaryMock.mockResolvedValue(summary(0, 2, 0));
  renderPage();
  expect(await screen.findByText('No recipients have a postal address.')).toBeInTheDocument();
  expect(screen.getByRole('status')).toHaveTextContent('2 skipped (no postal address: 2)');
  expect(screen.queryByRole('button', { name: /download csv/i })).not.toBeInTheDocument();
});

test('says there are no card occasions yet when the summary is empty', async () => {
  summaryMock.mockResolvedValue(summary(0));
  renderPage();
  expect(await screen.findByText(/No active card occasions yet/)).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /download csv/i })).not.toBeInTheDocument();
});

test('keeps the download button while the summary is loading', async () => {
  summaryMock.mockReturnValue(new Promise(() => {}));
  renderPage();
  expect(screen.getByRole('status')).toHaveTextContent('Checking who is on the list');
  expect(screen.getByRole('button', { name: /download csv/i })).toBeEnabled();
});

test('shows the localized reason when the summary is rejected and keeps the download', async () => {
  summaryMock.mockRejectedValue(
    new ApiError('English', 'VALIDATION_ERROR', 400, undefined, undefined, 'occasion_kind_invalid'),
  );
  renderPage();
  const alert = await screen.findByRole('alert');
  expect(alert).toHaveTextContent('kind must not contain control characters');
  expect(alert).toHaveTextContent("Couldn't load this data");
  expect(screen.getByRole('button', { name: /download csv/i })).toBeEnabled();
});

test('Retry refetches the summary in place', async () => {
  summaryMock.mockRejectedValueOnce(new Error('network down'));
  summaryMock.mockResolvedValue(summary(2));
  renderPage();
  await screen.findByRole('alert');
  fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
  await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('2 recipients'));
  expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  expect(summaryMock).toHaveBeenCalledTimes(2);
});
