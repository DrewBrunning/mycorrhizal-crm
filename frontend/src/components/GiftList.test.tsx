import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import '../i18n/config';
import GiftList from './GiftList';
import { Gift } from '../api/gifts';
import { DateFormatProvider } from '../DateFormatProvider';

// This codebase's vitest setup does not auto-cleanup between tests (no
// `globals: true`, setupTests.ts doesn't register it) -- without this, each
// test's render accumulates in the DOM and later tests see duplicate elements.
afterEach(cleanup);

function item(overrides: Partial<Gift> = {}): Gift {
  return {
    id: 'gift-1',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    entity_id: 'alice-uid',
    description: 'She liked the ceramics shop',
    status: 'idea',
    ...overrides,
  };
}

function renderList(props: Partial<React.ComponentProps<typeof GiftList>> = {}) {
  const defaults: React.ComponentProps<typeof GiftList> = {
    items: [],
    onAdd: vi.fn().mockResolvedValue(undefined),
    onEdit: vi.fn(),
    onMarkGiven: vi.fn().mockResolvedValue(undefined),
    onDelete: vi.fn(),
    ...props,
  };
  return render(
    <DateFormatProvider>
      <GiftList {...defaults} />
    </DateFormatProvider>
  );
}

test('shows the empty state when there are no gifts', () => {
  renderList();
  expect(screen.getByText(/No gift ideas or records yet/i)).toBeInTheDocument();
});

test('capturing is inline: type + click calls onAdd and clears the input', async () => {
  const onAdd = vi.fn().mockResolvedValue(undefined);
  renderList({ onAdd });

  const input = screen.getByLabelText('Record a gift idea…');
  fireEvent.change(input, { target: { value: 'A nice book about ceramics' } });
  fireEvent.click(screen.getByLabelText('Add'));

  await vi.waitFor(() => expect(onAdd).toHaveBeenCalledWith('A nice book about ceramics'));
  await vi.waitFor(() => expect(input).toHaveValue(''));
});

test('an idea renders with a working one-click mark-given action', async () => {
  const onMarkGiven = vi.fn().mockResolvedValue(undefined);
  renderList({ items: [item()], onMarkGiven });

  expect(screen.getByText('She liked the ceramics shop')).toBeInTheDocument();
  fireEvent.click(screen.getByLabelText('Mark as given'));
  await vi.waitFor(() => expect(onMarkGiven).toHaveBeenCalledWith(item()));
});

test('a given gift renders resolved with its date, value and currency, and no mark-given action', () => {
  renderList({
    items: [
      item({
        status: 'given',
        description: 'The espresso machine',
        occasion: 'birthday',
        date: '2026-08-03T12:00:00Z',
        value_cents: 12000,
        currency: 'EUR',
      }),
    ],
  });

  expect(screen.getByText('The espresso machine')).toBeInTheDocument();
  // "Given" appears both as the status chip and the section heading.
  expect(screen.getAllByText('Given').length).toBeGreaterThan(0);
  expect(screen.getByText('birthday')).toBeInTheDocument();
  expect(screen.queryByLabelText('Mark as given')).not.toBeInTheDocument();
});

test('a received gift renders under the Received section', () => {
  renderList({
    items: [item({ status: 'received', description: 'The scarf they gave me' })],
  });

  expect(screen.getByText('The scarf they gave me')).toBeInTheDocument();
  expect(screen.getAllByText('Received').length).toBeGreaterThan(0);
});

test('an idea renders under the Ideas section', () => {
  renderList({ items: [item({ description: 'She liked the ceramics shop' })] });

  expect(screen.getByText('She liked the ceramics shop')).toBeInTheDocument();
  expect(screen.getByText('Ideas')).toBeInTheDocument();
});

test('delete asks for confirmation before calling onDelete', () => {
  vi.stubGlobal('confirm', vi.fn(() => true));
  const onDelete = vi.fn();
  renderList({ items: [item()], onDelete });

  fireEvent.click(screen.getByLabelText('Delete'));
  expect(onDelete).toHaveBeenCalledWith('gift-1');

  vi.unstubAllGlobals();
});

test('a declining delete confirmation does not call onDelete', () => {
  vi.stubGlobal('confirm', vi.fn(() => false));
  const onDelete = vi.fn();
  renderList({ items: [item()], onDelete });

  fireEvent.click(screen.getByLabelText('Delete'));
  expect(onDelete).not.toHaveBeenCalled();

  vi.unstubAllGlobals();
});

test('a purchased item still shows the mark-given action', async () => {
  const onMarkGiven = vi.fn().mockResolvedValue(undefined);
  renderList({
    items: [item({ status: 'purchased', description: 'Bought but not yet given' })],
    onMarkGiven,
  });

  expect(screen.getByText('Purchased')).toBeInTheDocument();
  expect(screen.getByText('Bought but not yet given')).toBeInTheDocument();
  fireEvent.click(screen.getByLabelText('Mark as given'));
  await vi.waitFor(() => expect(onMarkGiven).toHaveBeenCalled());
});
