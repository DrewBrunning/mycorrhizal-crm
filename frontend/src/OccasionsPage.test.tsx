import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import './i18n/config';
import {
  downloadOccasionCardListCSV,
  type GiftShoppingItem,
  getGiftShoppingList,
} from './api/occasionObligations';
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

const getGiftShoppingListMock = vi.mocked(getGiftShoppingList);
const downloadMock = vi.mocked(downloadOccasionCardListCSV);

beforeEach(() => {
  getGiftShoppingListMock.mockReset();
  downloadMock.mockReset();
  getGiftShoppingListMock.mockResolvedValue({ gift_shopping_list: [], days: 30 });
  downloadMock.mockResolvedValue(undefined);
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
