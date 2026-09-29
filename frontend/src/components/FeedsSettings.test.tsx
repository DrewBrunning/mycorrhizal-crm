import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { Contact } from '../api/contacts';
import { getContactsByUid } from '../api/contacts';
import {
  createFeed,
  type Feed,
  listFeeds,
  revokeAllFeeds,
  revokeFeed,
  rotateFeed,
} from '../api/feeds';
import { SnackbarProvider } from '../context/SnackbarContext';
import FeedsSettings from './FeedsSettings';

afterEach(cleanup);

vi.mock('../api/feeds', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/feeds')>();
  return {
    ...actual,
    listFeeds: vi.fn(),
    createFeed: vi.fn(),
    rotateFeed: vi.fn(),
    revokeFeed: vi.fn(),
    revokeAllFeeds: vi.fn(),
  };
});
vi.mock('../api/contacts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/contacts')>();
  return { ...actual, getContactsByUid: vi.fn(), getContacts: vi.fn() };
});

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

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(listFeeds).mockReset();
  vi.mocked(listFeeds).mockResolvedValue([feed()]);
  vi.mocked(createFeed).mockReset();
  vi.mocked(rotateFeed).mockReset();
  vi.mocked(revokeFeed).mockReset();
  vi.mocked(revokeAllFeeds).mockReset();
  vi.mocked(getContactsByUid).mockReset();
  vi.mocked(getContactsByUid).mockResolvedValue(new Map<string, Contact>());
  Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } });
});

function renderSettings() {
  render(
    <SnackbarProvider>
      <FeedsSettings />
    </SnackbarProvider>,
  );
}

test('loads and lists the feeds under a Feeds heading', async () => {
  renderSettings();
  expect(screen.getByRole('heading', { name: 'Feeds' })).toBeInTheDocument();
  expect(await screen.findByText('Everyone')).toBeInTheDocument();
});

test('a load failure shows an error instead of the list', async () => {
  vi.mocked(listFeeds).mockRejectedValue(new Error('down'));
  renderSettings();
  expect(await screen.findByText('Failed to load feeds')).toBeInTheDocument();
  expect(screen.queryByRole('table')).not.toBeInTheDocument();
});

test('creating a feed refreshes the list and shows the one-time URL', async () => {
  vi.mocked(createFeed).mockResolvedValue({ feed: feed(), url: '/api/v1/feeds/atom?token=T' });
  renderSettings();
  await screen.findByText('Everyone');

  fireEvent.click(screen.getByRole('button', { name: 'Create Feed' }));
  const dialog = await screen.findByRole('dialog');
  fireEvent.click(within(dialog).getByRole('button', { name: 'Create' }));

  expect(await screen.findByLabelText('Feed URL')).toBeInTheDocument();
  await waitFor(() => expect(listFeeds).toHaveBeenCalledTimes(2));
});

test('rotating shows the replacement URL once', async () => {
  vi.mocked(rotateFeed).mockResolvedValue({ feed: feed(), url: '/api/v1/feeds/atom?token=NEW' });
  renderSettings();
  await screen.findByText('Everyone');

  fireEvent.click(screen.getByRole('button', { name: 'Rotate feed Everyone' }));
  fireEvent.click(screen.getByRole('button', { name: 'Rotate' }));

  const field = await screen.findByLabelText('Feed URL');
  expect(field).toHaveValue(`${window.location.origin}/api/v1/feeds/atom?token=NEW`);
  expect(rotateFeed).toHaveBeenCalledWith('f1');
  expect(screen.getByRole('heading', { name: 'Feed Rotated' })).toBeInTheDocument();

  fireEvent.click(screen.getByRole('button', { name: /Done/ }));
  await waitFor(() => expect(screen.queryByLabelText('Feed URL')).not.toBeInTheDocument());
});

test('a failed rotate surfaces the error and shows no URL', async () => {
  vi.mocked(rotateFeed).mockRejectedValue(new Error('rotate failed'));
  renderSettings();
  await screen.findByText('Everyone');

  fireEvent.click(screen.getByRole('button', { name: 'Rotate feed Everyone' }));
  fireEvent.click(screen.getByRole('button', { name: 'Rotate' }));

  expect(await screen.findByText('rotate failed')).toBeInTheDocument();
  expect(screen.queryByLabelText('Feed URL')).not.toBeInTheDocument();
});

test('revoking confirms, calls the API, toasts and refreshes', async () => {
  vi.mocked(revokeFeed).mockResolvedValue(undefined);
  renderSettings();
  await screen.findByText('Everyone');

  fireEvent.click(screen.getByRole('button', { name: 'Revoke feed Everyone' }));
  expect(revokeFeed).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button', { name: 'Revoke' }));

  await waitFor(() => expect(revokeFeed).toHaveBeenCalledWith('f1'));
  expect(await screen.findByText('Feed revoked')).toBeInTheDocument();
  await waitFor(() => expect(listFeeds).toHaveBeenCalledTimes(2));
});

test('revoke all confirms, calls the API and toasts the count', async () => {
  vi.mocked(revokeAllFeeds).mockResolvedValue({ revoked: 1 });
  renderSettings();
  await screen.findByText('Everyone');

  fireEvent.click(screen.getByRole('button', { name: 'Revoke All' }));
  expect(revokeAllFeeds).not.toHaveBeenCalled();
  fireEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Revoke All' }));

  await waitFor(() => expect(revokeAllFeeds).toHaveBeenCalledTimes(1));
  expect(await screen.findByText('1 feed revoked')).toBeInTheDocument();
});
