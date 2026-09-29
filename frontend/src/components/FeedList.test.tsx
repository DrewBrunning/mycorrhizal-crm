import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { Feed } from '../api/feeds';
import FeedList from './FeedList';

afterEach(cleanup);

function feed(overrides: Partial<Feed> = {}): Feed {
  return {
    id: 'f1',
    name: 'Everyone',
    kind: 'aggregate',
    entity_id: '',
    detail: 'headlines',
    created_at: '2026-01-01T00:00:00Z',
    last_accessed_at: null,
    ...overrides,
  };
}

function renderList(feeds: Feed[], names = new Map<string, string>()) {
  const handlers = {
    onRotate: vi.fn().mockResolvedValue(undefined),
    onRevoke: vi.fn().mockResolvedValue(undefined),
    onRevokeAll: vi.fn().mockResolvedValue(undefined),
  };
  render(<FeedList feeds={feeds} contactNames={names} {...handlers} />);
  return handlers;
}

test('renders "Never" for a null last_accessed_at and a date otherwise', () => {
  renderList([
    feed({ id: 'a', name: 'Unread' }),
    feed({ id: 'b', name: 'Read', last_accessed_at: '2026-02-03T04:05:06Z' }),
  ]);
  const unread = screen.getByRole('row', { name: /Unread/ });
  expect(within(unread).getByText('Never')).toBeInTheDocument();
  const read = screen.getByRole('row', { name: /Read/ });
  expect(within(read).queryByText('Never')).not.toBeInTheDocument();
});

test('shows kind (contact name for a contact feed) and detail', () => {
  renderList(
    [
      feed({ id: 'a', name: 'A', kind: 'contact', entity_id: 'alice-uid', detail: 'full' }),
      feed({ id: 'b', name: 'B', kind: 'contact', entity_id: 'gone-uid' }),
      feed({ id: 'c', name: 'C' }),
    ],
    new Map([['alice-uid', 'Alice Smith']]),
  );
  const a = screen.getByRole('row', { name: /^A / });
  expect(within(a).getByText('Contact: Alice Smith')).toBeInTheDocument();
  expect(within(a).getByText('Full')).toBeInTheDocument();
  expect(screen.getByText('Contact: Unknown contact')).toBeInTheDocument();
  const c = screen.getByRole('row', { name: /^C / });
  expect(within(c).getByText('All contacts')).toBeInTheDocument();
  expect(within(c).getByText('Headlines')).toBeInTheDocument();
});

test('an empty list shows the empty state and disables Revoke All', () => {
  renderList([]);
  expect(screen.getByText('No feeds yet')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Revoke All' })).toBeDisabled();
});

test('Revoke calls the API only after confirmation', async () => {
  const { onRevoke } = renderList([feed()]);
  fireEvent.click(screen.getByRole('button', { name: 'Revoke feed Everyone' }));
  expect(onRevoke).not.toHaveBeenCalled();
  expect(screen.getByText(/Are you sure you want to revoke "Everyone"/)).toBeInTheDocument();

  fireEvent.click(screen.getByRole('button', { name: 'Revoke' }));
  await waitFor(() => expect(onRevoke).toHaveBeenCalledTimes(1));
  expect(onRevoke).toHaveBeenCalledWith(expect.objectContaining({ id: 'f1' }));
  await waitFor(() => expect(screen.queryByText(/Are you sure/)).not.toBeInTheDocument());
});

test('cancelling the Revoke confirmation never calls the API', async () => {
  const { onRevoke } = renderList([feed()]);
  fireEvent.click(screen.getByRole('button', { name: 'Revoke feed Everyone' }));
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
  await waitFor(() => expect(screen.queryByText(/Are you sure/)).not.toBeInTheDocument());
  expect(onRevoke).not.toHaveBeenCalled();
});

test('Revoke All calls the API only after confirmation', async () => {
  const { onRevokeAll } = renderList([feed({ id: 'a' }), feed({ id: 'b', name: 'Two' })]);
  fireEvent.click(screen.getByRole('button', { name: 'Revoke All' }));
  expect(onRevokeAll).not.toHaveBeenCalled();
  expect(screen.getByText(/revoke all 2 feeds/)).toBeInTheDocument();

  fireEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Revoke All' }));
  await waitFor(() => expect(onRevokeAll).toHaveBeenCalledTimes(1));
});

test('Rotate calls the API only after confirmation', async () => {
  const { onRotate } = renderList([feed()]);
  fireEvent.click(screen.getByRole('button', { name: 'Rotate feed Everyone' }));
  expect(onRotate).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button', { name: 'Rotate' }));
  await waitFor(() => expect(onRotate).toHaveBeenCalledWith(expect.objectContaining({ id: 'f1' })));
});

test('a failed action keeps the confirmation open for a retry', async () => {
  const onRevoke = vi.fn().mockRejectedValue(new Error('boom'));
  render(
    <FeedList
      feeds={[feed()]}
      contactNames={new Map()}
      onRotate={vi.fn()}
      onRevoke={onRevoke}
      onRevokeAll={vi.fn()}
    />,
  );
  fireEvent.click(screen.getByRole('button', { name: 'Revoke feed Everyone' }));
  fireEvent.click(screen.getByRole('button', { name: 'Revoke' }));
  await waitFor(() => expect(onRevoke).toHaveBeenCalled());
  expect(screen.getByText(/Are you sure/)).toBeInTheDocument();
});
