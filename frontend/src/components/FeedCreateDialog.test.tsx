import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { type Contact, getContacts } from '../api/contacts';
import { createFeed, type Feed } from '../api/feeds';
import FeedCreateDialog from './FeedCreateDialog';

afterEach(cleanup);

vi.mock('../api/feeds', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/feeds')>();
  return { ...actual, createFeed: vi.fn() };
});
vi.mock('../api/contacts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/contacts')>();
  return { ...actual, getContacts: vi.fn() };
});

const created: Feed = {
  id: 'f1',
  name: 'x',
  kind: 'aggregate',
  entity_id: '',
  detail: 'headlines',
  created_at: '2026-01-01T00:00:00Z',
  last_accessed_at: null,
};

const alice = { ID: 1, uid: 'alice-uid', firstname: 'Alice', lastname: 'Smith' } as Contact;

beforeEach(() => {
  vi.mocked(createFeed).mockReset();
  vi.mocked(createFeed).mockResolvedValue({ feed: created, url: '/api/v1/feeds/atom?token=T0K' });
  vi.mocked(getContacts).mockReset();
  vi.mocked(getContacts).mockResolvedValue({ contacts: [alice], next_cursor: '' } as never);
  Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } });
});

function renderDialog(props: Partial<React.ComponentProps<typeof FeedCreateDialog>> = {}) {
  const onClose = vi.fn();
  const onCreated = vi.fn();
  render(<FeedCreateDialog open onClose={onClose} onCreated={onCreated} {...props} />);
  return { onClose, onCreated };
}

const create = () => fireEvent.click(screen.getByRole('button', { name: 'Create' }));

test('defaults to Headlines detail and an aggregate feed named "All contacts"', async () => {
  renderDialog();
  expect(screen.getByRole('radio', { name: /Headlines/ })).toBeChecked();
  expect(screen.getByRole('radio', { name: /Full/ })).not.toBeChecked();
  expect(screen.getByRole('radio', { name: 'All contacts' })).toBeChecked();
  expect(screen.getByLabelText(/Feed name/)).toHaveValue('All contacts');

  create();
  await waitFor(() => expect(createFeed).toHaveBeenCalledTimes(1));
  expect(createFeed).toHaveBeenCalledWith({
    name: 'All contacts',
    kind: 'aggregate',
    detail: 'headlines',
  });
});

test('always shows the reader-keeps-a-copy notice', () => {
  renderDialog();
  expect(
    screen.getByText(/keeps its own copy of everything the feed contains/),
  ).toBeInTheDocument();
  expect(screen.getByText(/self-hosted or local feed reader/)).toBeInTheDocument();
});

test('the Full warning appears only while Full is selected', () => {
  renderDialog();
  const warning = /will be copied into your feed reader/;
  expect(screen.queryByText(warning)).not.toBeInTheDocument();

  fireEvent.click(screen.getByRole('radio', { name: /Full/ }));
  expect(screen.getByText(warning)).toBeInTheDocument();

  fireEvent.click(screen.getByRole('radio', { name: /Headlines/ }));
  expect(screen.queryByText(warning)).not.toBeInTheDocument();
});

test('opened from a contact page it pre-sets kind and entity_id and hides the pickers', async () => {
  renderDialog({ contact: { uid: 'alice-uid', name: 'Alice Smith' } });
  expect(screen.getByLabelText(/Feed name/)).toHaveValue('Alice Smith');
  expect(screen.queryByRole('radio', { name: 'All contacts' })).not.toBeInTheDocument();
  expect(screen.queryByLabelText(/^Contact/)).not.toBeInTheDocument();

  create();
  await waitFor(() => expect(createFeed).toHaveBeenCalledTimes(1));
  expect(createFeed).toHaveBeenCalledWith({
    name: 'Alice Smith',
    kind: 'contact',
    detail: 'headlines',
    entity_id: 'alice-uid',
  });
  expect(getContacts).not.toHaveBeenCalled();
});

test('a contact feed from settings needs a picked contact, then sends its uid', async () => {
  renderDialog();
  fireEvent.click(screen.getByRole('radio', { name: 'One contact' }));
  // Name is cleared until a contact is picked, so Create stays disabled.
  expect(screen.getByRole('button', { name: 'Create' })).toBeDisabled();

  const picker = await screen.findByLabelText(/^Contact/);
  fireEvent.mouseDown(picker);
  fireEvent.click(await screen.findByRole('option', { name: 'Alice Smith' }));

  await waitFor(() => expect(screen.getByLabelText(/Feed name/)).toHaveValue('Alice Smith'));
  create();
  await waitFor(() => expect(createFeed).toHaveBeenCalledTimes(1));
  expect(createFeed).toHaveBeenCalledWith({
    name: 'Alice Smith',
    kind: 'contact',
    detail: 'headlines',
    entity_id: 'alice-uid',
  });
});

test('an edited name is kept when the kind changes', async () => {
  renderDialog();
  fireEvent.change(screen.getByLabelText(/Feed name/), { target: { value: 'My reader' } });
  fireEvent.click(screen.getByRole('radio', { name: 'One contact' }));
  fireEvent.click(screen.getByRole('radio', { name: 'All contacts' }));
  expect(screen.getByLabelText(/Feed name/)).toHaveValue('My reader');
});

test('a blank name disables Create', () => {
  renderDialog();
  fireEvent.change(screen.getByLabelText(/Feed name/), { target: { value: '   ' } });
  expect(screen.getByRole('button', { name: 'Create' })).toBeDisabled();
});

test('sends the Full detail when selected', async () => {
  renderDialog();
  fireEvent.click(screen.getByRole('radio', { name: /Full/ }));
  create();
  await waitFor(() => expect(createFeed).toHaveBeenCalledTimes(1));
  expect(vi.mocked(createFeed).mock.calls[0][0].detail).toBe('full');
});

test('on success shows the one-time URL with the origin prefixed when relative', async () => {
  const { onCreated } = renderDialog();
  create();

  const field = await screen.findByLabelText('Feed URL');
  expect(field).toHaveValue(`${window.location.origin}/api/v1/feeds/atom?token=T0K`);
  expect(field).toHaveAttribute('readonly');
  expect(screen.getByText(/This URL is shown once/)).toBeInTheDocument();
  expect(onCreated).toHaveBeenCalledWith(expect.objectContaining({ url: expect.any(String) }));
  // The form is gone -- the plaintext replaces it.
  expect(screen.queryByLabelText(/Feed name/)).not.toBeInTheDocument();
});

test('an absolute URL is shown as returned', async () => {
  vi.mocked(createFeed).mockResolvedValue({
    feed: created,
    url: 'https://crm.example.com/api/v1/feeds/atom?token=T0K',
  });
  renderDialog();
  create();
  expect(await screen.findByLabelText('Feed URL')).toHaveValue(
    'https://crm.example.com/api/v1/feeds/atom?token=T0K',
  );
});

test('Copy puts the absolute URL on the clipboard', async () => {
  renderDialog();
  create();
  fireEvent.click(await screen.findByRole('button', { name: 'Copy' }));
  expect(navigator.clipboard.writeText).toHaveBeenCalledWith(
    `${window.location.origin}/api/v1/feeds/atom?token=T0K`,
  );
  expect(await screen.findByRole('button', { name: 'Copied!' })).toBeInTheDocument();
});

test('Done closes the dialog', async () => {
  const { onClose } = renderDialog();
  create();
  fireEvent.click(await screen.findByRole('button', { name: /Done/ }));
  expect(onClose).toHaveBeenCalled();
});

test('a create failure shows the error and keeps the form', async () => {
  vi.mocked(createFeed).mockRejectedValue(new Error('feed limit reached'));
  const { onCreated } = renderDialog();
  create();
  expect(await screen.findByText('feed limit reached')).toBeInTheDocument();
  expect(screen.getByLabelText(/Feed name/)).toBeInTheDocument();
  expect(onCreated).not.toHaveBeenCalled();
});

test('Cancel closes without creating', () => {
  const { onClose } = renderDialog();
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
  expect(onClose).toHaveBeenCalled();
  expect(createFeed).not.toHaveBeenCalled();
});

test('a contact-search failure leaves the picker empty instead of throwing', async () => {
  vi.mocked(getContacts).mockRejectedValue(new Error('down'));
  renderDialog();
  fireEvent.click(screen.getByRole('radio', { name: 'One contact' }));
  fireEvent.mouseDown(await screen.findByLabelText(/^Contact/));
  expect(await screen.findByText('No matching contacts')).toBeInTheDocument();
});

test('reopening resets the form', async () => {
  const { rerender } = render(<FeedCreateDialog open onClose={() => {}} />);
  fireEvent.click(screen.getByRole('radio', { name: /Full/ }));
  rerender(<FeedCreateDialog open={false} onClose={() => {}} />);
  rerender(<FeedCreateDialog open onClose={() => {}} />);
  expect(screen.getByRole('radio', { name: /Headlines/ })).toBeChecked();
});
