import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, within, cleanup } from '@testing-library/react';
import '../i18n/config';
import GiftList from './GiftList';
import { Gift } from '../api/gifts';
import { DateFormatProvider } from '../DateFormatProvider';

// This codebase's vitest setup does not auto-cleanup between tests (no
// `globals: true`, setupTests.ts doesn't register it) -- without this, each
// test's render accumulates in the DOM and later tests see duplicate elements.
afterEach(cleanup);

// Every status section (Ideas / Given / Received) is an accessible region (T46)
// named by its header, so tests can target one section's entry points instead
// of matching the three identical quick-add inputs and "Add with details"
// buttons by accident.
function section(name: string) {
  return within(screen.getByRole('region', { name }));
}

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
    onAddFull: vi.fn(),
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

test('all three sections render their header and add row even when empty', () => {
  // T46: every section always shows its own entry point, so a status has a
  // first quick path before any item of that status exists. The sections are
  // the non-empty content here — this is not a T30 "header over nothing".
  renderList();
  for (const name of ['Ideas', 'Given', 'Received']) {
    const s = section(name);
    expect(s.getByRole('textbox')).toBeInTheDocument();
    expect(s.getByRole('button', { name: /Add with details/i })).toBeInTheDocument();
  }
});

test('a section keeps its entry points once it already has items', () => {
  // T46 "Done when": recording a second gift at a status must not lose the
  // row it was recorded from.
  renderList({
    items: [
      item({ status: 'given', description: 'The espresso machine' }),
      item({ id: 'gift-2', status: 'received', description: 'The scarf they gave me' }),
    ],
  });

  for (const name of ['Given', 'Received']) {
    const s = section(name);
    expect(s.getByRole('textbox')).toBeInTheDocument();
    expect(s.getByRole('button', { name: /Add with details/i })).toBeInTheDocument();
  }
});

test('capturing an idea is inline: type + click calls onAdd with the idea status and clears the input', async () => {
  const onAdd = vi.fn().mockResolvedValue(undefined);
  renderList({ onAdd });

  const input = section('Ideas').getByLabelText('Record a gift idea…');
  fireEvent.change(input, { target: { value: 'A nice book about ceramics' } });
  fireEvent.click(section('Ideas').getByLabelText('Add'));

  await vi.waitFor(() => expect(onAdd).toHaveBeenCalledWith('A nice book about ceramics', 'idea'));
  await vi.waitFor(() => expect(input).toHaveValue(''));
});

test('the Given quick-add records straight as given, never as an idea', async () => {
  const onAdd = vi.fn().mockResolvedValue(undefined);
  renderList({ onAdd });

  const given = section('Given');
  const input = given.getByLabelText('Record something given…');
  fireEvent.change(input, { target: { value: 'The espresso machine' } });
  fireEvent.keyDown(input, { key: 'Enter' });

  await vi.waitFor(() => expect(onAdd).toHaveBeenCalledWith('The espresso machine', 'given'));
  await vi.waitFor(() => expect(input).toHaveValue(''));
});

test('the Received quick-add records straight as received', async () => {
  const onAdd = vi.fn().mockResolvedValue(undefined);
  renderList({ onAdd });

  const input = section('Received').getByLabelText('Record something received…');
  fireEvent.change(input, { target: { value: 'The scarf they gave me' } });
  fireEvent.keyDown(input, { key: 'Enter' });

  await vi.waitFor(() => expect(onAdd).toHaveBeenCalledWith('The scarf they gave me', 'received'));
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

// --- T35: URL, notes, and the full-form entry point ---

test('a gift URL renders as a tappable link that opens in a new tab', () => {
  renderList({ items: [item({ url: 'https://shop.example.com/mug' })] });

  const link = screen.getByRole('link', { name: 'https://shop.example.com/mug' });
  expect(link).toHaveAttribute('href', 'https://shop.example.com/mug');
  expect(link).toHaveAttribute('target', '_blank');
  // Without noopener the opened page gets a handle on this window.
  expect(link).toHaveAttribute('rel', 'noopener noreferrer');
});

test('an unsafe-scheme URL is shown as text, never as an href', () => {
  // A value that predates the httpurl validator (or arrives from a synced
  // replica) must not become a clickable javascript: link.
  renderList({ items: [item({ url: 'javascript:alert(1)' })] });

  expect(screen.queryByRole('link')).not.toBeInTheDocument();
  expect(screen.getByText('javascript:alert(1)')).toBeInTheDocument();
});

test('a non-http scheme that the old validator accepted is shown as text, never as an href', () => {
  // safeurl used to accept mailto: — httpurl (T41) does not, and the render
  // guard must agree with the write validator: a gift URL means a web page.
  renderList({ items: [item({ url: 'mailto:a@b.com' })] });

  expect(screen.queryByRole('link')).not.toBeInTheDocument();
  expect(screen.getByText('mailto:a@b.com')).toBeInTheDocument();
});

test('a URL that is not an absolute URI is shown as text, not linked', () => {
  renderList({ items: [item({ url: 'shop.example.com/mug' })] });

  expect(screen.queryByRole('link')).not.toBeInTheDocument();
  expect(screen.getByText('shop.example.com/mug')).toBeInTheDocument();
});

test('notes render alongside the description', () => {
  renderList({
    items: [item({ notes: 'Size medium. Check she has not bought it herself.' })],
  });

  expect(screen.getByText('She liked the ceramics shop')).toBeInTheDocument();
  expect(screen.getByText(/Size medium/)).toBeInTheDocument();
});

test('a whitespace-only URL or notes value renders nothing at all', () => {
  // The dialog trims, but it is not the only writer — a direct API caller can
  // store "   ", which must not become an empty row. Asserted structurally,
  // because an empty row has no text to query for: the item's content column
  // holds exactly the chip/meta row and the description, nothing else.
  renderList({ items: [item({ url: '   ', notes: '  ' })] });

  const content = screen.getByText('She liked the ceramics shop').parentElement!;
  expect(content.children).toHaveLength(2);
  expect(screen.queryByRole('link')).not.toBeInTheDocument();
});

test('a gift with neither URL nor notes renders neither', () => {
  renderList({ items: [item()] });

  expect(screen.queryByRole('link')).not.toBeInTheDocument();
  expect(screen.getByText('She liked the ceramics shop')).toBeInTheDocument();
});

test('each section\'s full-form button pre-seeds its own status, leaving the quick-add alone', () => {
  const onAddFull = vi.fn();
  const onAdd = vi.fn().mockResolvedValue(undefined);
  renderList({ onAddFull, onAdd });

  fireEvent.click(section('Ideas').getByRole('button', { name: /Add with details/i }));
  fireEvent.click(section('Given').getByRole('button', { name: /Add with details/i }));
  fireEvent.click(section('Received').getByRole('button', { name: /Add with details/i }));

  expect(onAddFull).toHaveBeenNthCalledWith(1, 'idea');
  expect(onAddFull).toHaveBeenNthCalledWith(2, 'given');
  expect(onAddFull).toHaveBeenNthCalledWith(3, 'received');
  expect(onAdd).not.toHaveBeenCalled();
  // The low-friction idea capture stays exactly where it was (T20b's point).
  expect(section('Ideas').getByLabelText('Record a gift idea…')).toBeInTheDocument();
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
