import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent, waitFor } from '@testing-library/react';
import '../i18n/config';
import ShareContactDialog from './ShareContactDialog';
import { EXPORT_FIELD_SECTIONS } from '../api/export';

afterEach(cleanup);
afterEach(() => vi.unstubAllGlobals());

function renderDialog(props: Partial<React.ComponentProps<typeof ShareContactDialog>> = {}) {
  const defaults: React.ComponentProps<typeof ShareContactDialog> = {
    open: true,
    onClose: vi.fn(),
    vcardUID: 'alice-uid',
    ...props,
  };
  return render(<ShareContactDialog {...defaults} />);
}

function mockFetchByUrl(handlers: Record<string, (url: string, init?: RequestInit) => unknown>) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      for (const [pattern, respond] of Object.entries(handlers)) {
        if (url.includes(pattern)) {
          return { ok: true, json: async () => respond(url, init) };
        }
      }
      throw new Error(`unexpected fetch: ${url}`);
    })
  );
}

const directoryResponse = () => ({
  users: [
    { id: 2, username: 'bob' },
    { id: 3, username: 'carol' },
  ],
});

test('fetches and lists the user directory as recipient options', async () => {
  mockFetchByUrl({ '/users/directory': directoryResponse });
  renderDialog();

  await waitFor(() => expect(fetch).toHaveBeenCalled());
  const select = await screen.findByLabelText('Recipient');
  fireEvent.mouseDown(select);
  expect(await screen.findByText('bob')).toBeInTheDocument();
  expect(screen.getByText('carol')).toBeInTheDocument();
});

test('share button is disabled until a recipient is chosen', async () => {
  mockFetchByUrl({ '/users/directory': directoryResponse });
  renderDialog();

  await waitFor(() => expect(fetch).toHaveBeenCalled());
  expect(screen.getByRole('button', { name: 'Share' })).toBeDisabled();
});

test('warns the sender the share is frozen and cannot be recalled once sent', async () => {
  mockFetchByUrl({ '/users/directory': directoryResponse });
  renderDialog();

  await waitFor(() => expect(fetch).toHaveBeenCalled());
  expect(
    screen.getByText(/can't be recalled once sent/i)
  ).toBeInTheDocument();
});

test('sensitive sections stay locked by default, same guard as export', async () => {
  mockFetchByUrl({ '/users/directory': directoryResponse });
  renderDialog();

  await waitFor(() => expect(fetch).toHaveBeenCalled());
  const relationships = screen.getByRole('checkbox', { name: 'Relationships' });
  expect(relationships).toBeDisabled();
  expect(relationships).not.toBeChecked();
});

test('sharing sends the selected recipient, sections, and includeSensitive=false by default', async () => {
  let sentBody: Record<string, unknown> | undefined;
  mockFetchByUrl({
    '/users/directory': directoryResponse,
    '/contact-shares': (_url, init) => {
      sentBody = JSON.parse(init!.body as string);
      return { message: 'Share created', contact_share: { id: 'share-1' } };
    },
  });
  const onClose = vi.fn();
  renderDialog({ onClose });

  await waitFor(() => expect(fetch).toHaveBeenCalled());

  const select = await screen.findByLabelText('Recipient');
  fireEvent.mouseDown(select);
  fireEvent.click(await screen.findByText('bob'));

  fireEvent.click(screen.getByRole('button', { name: 'Share' }));

  // Waiting on onClose guarantees handleShare's await chain -- including the
  // mocked response.json() that populates sentBody -- has fully resolved.
  await waitFor(() => expect(onClose).toHaveBeenCalled());

  expect(sentBody?.to_user_id).toBe(2);
  expect(sentBody?.vcard_uid).toBe('alice-uid');
  expect(sentBody?.include_sensitive).toBe(false);
  expect(sentBody?.sections).toEqual(
    expect.arrayContaining(EXPORT_FIELD_SECTIONS.filter((s) => !s.sensitive).map((s) => s.token))
  );
});

test('revealing and selecting a sensitive section sends the explicit opt-in', async () => {
  let sentBody: Record<string, unknown> | undefined;
  mockFetchByUrl({
    '/users/directory': directoryResponse,
    '/contact-shares': (_url, init) => {
      sentBody = JSON.parse(init!.body as string);
      return { message: 'Share created', contact_share: { id: 'share-1' } };
    },
  });
  const onClose = vi.fn();
  renderDialog({ onClose });

  await waitFor(() => expect(fetch).toHaveBeenCalled());
  const select = await screen.findByLabelText('Recipient');
  fireEvent.mouseDown(select);
  fireEvent.click(await screen.findByText('bob'));

  fireEvent.click(screen.getByRole('button', { name: 'Enable sensitive fields' }));
  fireEvent.click(screen.getByRole('button', { name: 'Enable' }));

  const relationships = await screen.findByRole('checkbox', { name: 'Relationships' });
  fireEvent.click(relationships);

  fireEvent.click(screen.getByRole('button', { name: 'Share' }));
  await waitFor(() => expect(onClose).toHaveBeenCalled());

  expect(sentBody?.include_sensitive).toBe(true);
  expect(sentBody?.sections).toContain('related_to');
});
