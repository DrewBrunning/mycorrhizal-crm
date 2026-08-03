import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import '../i18n/config';
import ConversationAgendaList from './ConversationAgendaList';
import { ConversationAgenda } from '../api/conversationAgenda';
import { DateFormatProvider } from '../DateFormatProvider';

// This codebase's vitest setup does not auto-cleanup between tests (no
// `globals: true`, setupTests.ts doesn't register it) -- without this, each
// test's render accumulates in the DOM and later tests see duplicate elements.
afterEach(cleanup);

function item(overrides: Partial<ConversationAgenda> = {}): ConversationAgenda {
  return {
    id: 'agenda-1',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    entity_id: 'alice-uid',
    content: 'Ask about her mother\u2019s surgery',
    ...overrides,
  };
}

function renderList(props: Partial<React.ComponentProps<typeof ConversationAgendaList>> = {}) {
  const defaults: React.ComponentProps<typeof ConversationAgendaList> = {
    items: [],
    onAdd: vi.fn().mockResolvedValue(undefined),
    onEdit: vi.fn(),
    onDiscuss: vi.fn(),
    onDelete: vi.fn(),
    ...props,
  };
  return render(
    <DateFormatProvider>
      <ConversationAgendaList {...defaults} />
    </DateFormatProvider>
  );
}

test('shows the empty state when there are no items', () => {
  renderList();
  expect(screen.getByText(/Nothing on the agenda yet/i)).toBeInTheDocument();
});

test('adding is inline: type + click calls onAdd and clears the input', async () => {
  const onAdd = vi.fn().mockResolvedValue(undefined);
  renderList({ onAdd });

  const input = screen.getByLabelText('Things to bring up next time…');
  fireEvent.change(input, { target: { value: 'Ask about the garden' } });
  fireEvent.click(screen.getByLabelText('Add'));

  await vi.waitFor(() => expect(onAdd).toHaveBeenCalledWith('Ask about the garden'));
  await vi.waitFor(() => expect(input).toHaveValue(''));
});

test('pressing Enter adds the item', async () => {
  const onAdd = vi.fn().mockResolvedValue(undefined);
  renderList({ onAdd });

  const input = screen.getByLabelText('Things to bring up next time…');
  fireEvent.change(input, { target: { value: 'Ask about the new house' } });
  fireEvent.keyDown(input, { key: 'Enter' });

  await vi.waitFor(() => expect(onAdd).toHaveBeenCalledWith('Ask about the new house'));
});

test('an empty input does not call onAdd', () => {
  const onAdd = vi.fn().mockResolvedValue(undefined);
  renderList({ onAdd });

  const input = screen.getByLabelText('Things to bring up next time…');
  fireEvent.change(input, { target: { value: '   ' } });
  fireEvent.keyDown(input, { key: 'Enter' });

  expect(onAdd).not.toHaveBeenCalled();
});

test('open items render with a working mark-discussed action', () => {
  const onDiscuss = vi.fn();
  renderList({ items: [item()], onDiscuss });

  expect(screen.getByText('Ask about her mother’s surgery')).toBeInTheDocument();
  expect(screen.getByLabelText('Mark as discussed')).toBeInTheDocument();
  fireEvent.click(screen.getByLabelText('Mark as discussed'));
  expect(onDiscuss).toHaveBeenCalledWith(item());
});

test('discussed items render resolved: no mark-discussed action, discussed date shown', () => {
  renderList({
    items: [item({ discussed_at: '2026-08-03T12:00:00Z' })],
  });

  expect(screen.getByText('Ask about her mother’s surgery')).toBeInTheDocument();
  expect(screen.getByText('Discussed')).toBeInTheDocument();
  expect(screen.queryByLabelText('Mark as discussed')).not.toBeInTheDocument();
});

test('delete asks for confirmation before calling onDelete', () => {
  vi.stubGlobal('confirm', vi.fn(() => true));
  const onDelete = vi.fn();
  renderList({ items: [item()], onDelete });

  fireEvent.click(screen.getByLabelText('Delete'));
  expect(onDelete).toHaveBeenCalledWith('agenda-1');

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

test('an optional reference link is rendered', () => {
  renderList({ items: [item({ reference_url: 'https://example.com/article' })] });
  expect(screen.getByText('https://example.com/article')).toBeInTheDocument();
});
