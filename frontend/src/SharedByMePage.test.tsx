import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';
import './i18n/config';
import { AnnouncerProvider } from './context/AnnouncerContext';
import { DateFormatProvider } from './DateFormatProvider';
import SharedByMePage from './SharedByMePage';
import { errorEnvelope, mockApi, setupMswServer } from './test/mswServer';

setupMswServer();
afterEach(cleanup);

function grant(over: Record<string, unknown> = {}) {
  return {
    id: 'g1',
    to_user_id: 2,
    to_username: 'bob',
    entity_type: 'contact',
    entity_id: 'uid-1',
    label: 'Alice Anderson',
    sections: ['emails', 'phones'],
    reconfirm_sections: [],
    status: 'active',
    expires_at: null,
    responded_at: '2026-01-02T00:00:00Z',
    revoked_at: null,
    revoked_reason: null,
    member_count: null,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-02T00:00:00Z',
    revision: 3,
    ...over,
  };
}

const page = (grants: unknown[], next = '') => ({
  share_grants: grants,
  next_cursor: next,
  limit: 25,
});

function renderPage() {
  return render(
    <MemoryRouter>
      <AnnouncerProvider>
        <DateFormatProvider>
          <SharedByMePage />
        </DateFormatProvider>
      </AnnouncerProvider>
    </MemoryRouter>,
  );
}

async function rowFor(label: string): Promise<HTMLElement> {
  const li = (await screen.findByText(label)).closest('li');
  if (!li) throw new Error(`no row for ${label}`);
  return li;
}

describe('SharedByMePage', () => {
  test('lists status, recipient, expiry and granted sections; shows no access history', async () => {
    mockApi(
      'get',
      '/share-grants/outgoing',
      page([
        grant({ expires_at: '2099-02-03T10:00:00Z' }),
        grant({
          id: 'g2',
          label: 'Book Club',
          entity_type: 'circle',
          sections: [],
          member_count: 12,
        }),
        grant({ id: 'g3', label: 'Old', status: 'revoked', revoked_reason: 'entity_deleted' }),
      ]),
    );
    renderPage();
    const row = await rowFor('Alice Anderson');
    expect(within(row).getByText('To bob')).toBeInTheDocument();
    expect(within(row).getByText('Active')).toBeInTheDocument();
    expect(within(row).getByText('Sections: Emails, Phones')).toBeInTheDocument();
    expect(within(row).getByText(/^Expires /)).toBeInTheDocument();

    const circle = await rowFor('Book Club');
    expect(within(circle).getByText('Shares 12 people right now')).toBeInTheDocument();
    expect(within(circle).getByText('No expiry')).toBeInTheDocument();

    const revoked = await rowFor('Old');
    expect(within(revoked).getByText('Revoked')).toBeInTheDocument();
    expect(within(revoked).getByText('The shared item was deleted')).toBeInTheDocument();
    expect(within(revoked).queryByRole('button')).toBeNull(); // terminal: no actions

    expect(screen.queryByText(/last (read|seen|viewed|accessed)/i)).toBeNull();
  });

  test('a grant the recipient blocked still reads as pending, exactly as the API returns it', async () => {
    mockApi(
      'get',
      '/share-grants/outgoing',
      page([grant({ status: 'pending', responded_at: null })]),
    );
    renderPage();
    const row = await rowFor('Alice Anderson');
    expect(within(row).getByText('Pending')).toBeInTheDocument();
  });

  test('revoking confirms with the not-recall warning, then posts revoke and announces it', async () => {
    mockApi('get', '/share-grants/outgoing', page([grant()]));
    const revoke = mockApi('post', '/share-grants/g1/revoke', {
      share_grant: grant({ status: 'revoked', revoked_reason: 'owner_revoked' }),
    });
    renderPage();
    fireEvent.click(await screen.findByRole('button', { name: /^Revoke Alice Anderson for bob/ }));
    const dialog = await screen.findByRole('dialog', { name: 'Revoke this share?' });
    expect(
      within(dialog).getByText(/does not recall anything they have already seen/i),
    ).toBeInTheDocument();
    expect(revoke).toHaveLength(0);
    fireEvent.click(within(dialog).getByRole('button', { name: 'Revoke' }));
    await waitFor(() => expect(revoke).toHaveLength(1));
    const row = await rowFor('Alice Anderson');
    await waitFor(() => expect(within(row).getByText('Revoked')).toBeInTheDocument());
    expect(within(row).getByText('You revoked it')).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.getByRole('status')).toHaveTextContent('Revoked Alice Anderson for bob.'),
    );
  });

  test('cancelling the revoke dialog sends nothing', async () => {
    mockApi('get', '/share-grants/outgoing', page([grant()]));
    const revoke = mockApi('post', '/share-grants/g1/revoke', {});
    renderPage();
    fireEvent.click(await screen.findByRole('button', { name: /^Revoke / }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(revoke).toHaveLength(0);
  });

  test('sections awaiting re-confirmation are flagged, and confirming posts reconfirm', async () => {
    mockApi('get', '/share-grants/outgoing', page([grant({ reconfirm_sections: ['addresses'] })]));
    const reconfirm = mockApi('post', '/share-grants/g1/reconfirm', {
      share_grant: grant({ reconfirm_sections: [] }),
    });
    renderPage();
    expect(
      await screen.findByText(/Hidden from bob until you confirm: Addresses/),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /^Re-confirm Alice Anderson for bob/ }));
    const dialog = await screen.findByRole('dialog', { name: 'Share these sections again?' });
    expect(reconfirm).toHaveLength(0);
    fireEvent.click(within(dialog).getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(reconfirm).toHaveLength(1));
    await waitFor(() => expect(screen.queryByText(/Hidden from bob/)).toBeNull());
    expect(screen.queryByRole('button', { name: /^Re-confirm/ })).toBeNull();
  });

  test('a grant with nothing to re-confirm has no re-confirm action', async () => {
    mockApi('get', '/share-grants/outgoing', page([grant()]));
    renderPage();
    await screen.findByText('Alice Anderson');
    expect(screen.queryByRole('button', { name: /^Re-confirm/ })).toBeNull();
  });

  test('edit is narrow-only: it lists only current sections, sends only the change, with If-Match', async () => {
    mockApi('get', '/share-grants/outgoing', page([grant()]));
    const patch = mockApi('patch', '/share-grants/g1', {
      share_grant: grant({ sections: ['emails'], revision: 4 }),
    });
    renderPage();
    fireEvent.click(await screen.findByRole('button', { name: /^Edit Alice Anderson for bob/ }));
    const dialog = await screen.findByRole('dialog', { name: 'Edit share' });
    expect(within(dialog).getByText(/To share more, create a new share/)).toBeInTheDocument();
    const boxes = within(dialog).getAllByRole('checkbox');
    expect(
      boxes.map(
        (b) => b.getAttribute('aria-label') ?? (b as HTMLInputElement).labels?.[0]?.textContent,
      ),
    ).toEqual(['Emails', 'Phones']);
    fireEvent.click(within(dialog).getByRole('checkbox', { name: 'Phones' }));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(patch).toHaveLength(1));
    expect(patch[0].body).toEqual({ sections: ['emails'] });
    expect(patch[0].headers.get('If-Match')).toBe('"3"');
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(await screen.findByText('Sections: Emails')).toBeInTheDocument();
  });

  test('edit can set and clear the expiry, and cannot save with no sections left', async () => {
    mockApi('get', '/share-grants/outgoing', page([grant({ expires_at: '2099-02-03T10:00:00Z' })]));
    const patch = mockApi('patch', '/share-grants/g1', { share_grant: grant() });
    renderPage();
    fireEvent.click(await screen.findByRole('button', { name: /^Edit / }));
    const dialog = await screen.findByRole('dialog', { name: 'Edit share' });
    const save = within(dialog).getByRole('button', { name: 'Save' });
    expect(save).toBeDisabled(); // nothing changed yet
    fireEvent.click(within(dialog).getByRole('checkbox', { name: 'Emails' }));
    fireEvent.click(within(dialog).getByRole('checkbox', { name: 'Phones' }));
    expect(save).toBeDisabled(); // would leave zero sections
    fireEvent.click(within(dialog).getByRole('checkbox', { name: 'Emails' }));
    fireEvent.click(within(dialog).getByRole('checkbox', { name: 'Phones' }));
    fireEvent.change(within(dialog).getByLabelText('Expires on (optional)'), {
      target: { value: '' },
    });
    fireEvent.click(save);
    await waitFor(() => expect(patch).toHaveLength(1));
    expect(patch[0].body).toEqual({ expires_at: null });
  });

  test('a stale-revision 412 says so and reloads the list', async () => {
    const list = mockApi('get', '/share-grants/outgoing', page([grant()]));
    mockApi('patch', '/share-grants/g1', errorEnvelope(412, 'PRECONDITION_FAILED', 'stale'));
    renderPage();
    fireEvent.click(await screen.findByRole('button', { name: /^Edit / }));
    const dialog = await screen.findByRole('dialog', { name: 'Edit share' });
    fireEvent.click(within(dialog).getByRole('checkbox', { name: 'Phones' }));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
    expect(await within(dialog).findByText(/changed elsewhere/)).toBeInTheDocument();
    await waitFor(() => expect(list.length).toBeGreaterThanOrEqual(2));
  });

  test('a circle grant edit offers expiry only (no sections)', async () => {
    mockApi(
      'get',
      '/share-grants/outgoing',
      page([grant({ entity_type: 'circle', label: 'Book Club', sections: [], member_count: 3 })]),
    );
    renderPage();
    fireEvent.click(await screen.findByRole('button', { name: /^Edit Book Club/ }));
    const dialog = await screen.findByRole('dialog', { name: 'Edit share' });
    expect(within(dialog).queryAllByRole('checkbox')).toHaveLength(0);
    expect(within(dialog).getByLabelText('Expires on (optional)')).toBeInTheDocument();
  });

  test('says revocation is not recall in the page description', async () => {
    mockApi('get', '/share-grants/outgoing', page([]));
    renderPage();
    expect(
      await screen.findByText('You have not shared anything with a standing share.'),
    ).toBeInTheDocument();
    expect(screen.getByText(/does not recall what someone has already seen/i)).toBeInTheDocument();
  });

  test('a load failure offers retry instead of the empty state', async () => {
    mockApi('get', '/share-grants/outgoing', errorEnvelope(500, 'INTERNAL', 'boom'));
    renderPage();
    expect(await screen.findByRole('button', { name: 'Retry' })).toBeInTheDocument();
    expect(screen.queryByText('You have not shared anything with a standing share.')).toBeNull();
  });

  test('an action error is shown and the row is unchanged', async () => {
    mockApi('get', '/share-grants/outgoing', page([grant()]));
    mockApi('post', '/share-grants/g1/revoke', errorEnvelope(409, 'CONFLICT', 'nope'));
    renderPage();
    fireEvent.click(await screen.findByRole('button', { name: /^Revoke / }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Revoke' }));
    expect(await screen.findByRole('alert')).toBeInTheDocument();
    expect(screen.getByText('Active')).toBeInTheDocument();
  });

  test('load more appends the next page', async () => {
    mockApi('get', '/share-grants/outgoing', page([grant()], 'c2'));
    renderPage();
    await screen.findByText('Alice Anderson');
    mockApi('get', '/share-grants/outgoing', page([grant({ id: 'g9', label: 'Zed' })]));
    fireEvent.click(screen.getByRole('button', { name: 'Load more' }));
    expect(await screen.findByText('Zed')).toBeInTheDocument();
    expect(screen.getByText('Alice Anderson')).toBeInTheDocument();
  });
});
