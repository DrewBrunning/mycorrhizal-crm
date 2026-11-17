import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';
import './i18n/config';
import { HttpResponse, http } from 'msw';
import { API_BASE_URL } from './api/client';
import { AnnouncerProvider } from './context/AnnouncerContext';
import { DateFormatProvider } from './DateFormatProvider';
import SharedByMePage from './SharedByMePage';
import { errorEnvelope, mockApi, server, setupMswServer } from './test/mswServer';

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

    fireEvent.click(screen.getByRole('button', { name: /Ended shares/ }));
    const revoked = await rowFor('Old');
    expect(within(revoked).getByText('Revoked')).toBeInTheDocument();
    expect(within(revoked).getByText('The shared item was deleted')).toBeInTheDocument();
    expect(within(revoked).queryByRole('button')).toBeNull(); // terminal: no actions

    expect(screen.queryByText(/last (read|seen|viewed|accessed)/i)).toBeNull();
  });

  test('groups live grants first and ended ones in a collapsed section without member counts (#2705)', async () => {
    mockApi(
      'get',
      '/share-grants/outgoing',
      page([
        grant({
          id: 'e1',
          label: 'Old Club',
          entity_type: 'circle',
          sections: [],
          member_count: 6,
          status: 'declined',
        }),
        grant({
          id: 'l1',
          label: 'Book Club',
          entity_type: 'circle',
          sections: [],
          member_count: 12,
        }),
        grant({ id: 'e2', label: 'Gone Contact', status: 'expired' }),
        grant({ id: 'l2', label: 'Pending One', status: 'pending' }),
      ]),
    );
    renderPage();
    await rowFor('Book Club');
    const live = screen.getByRole('heading', { level: 2, name: 'Live shares' });
    expect(live).toBeInTheDocument();
    expect(screen.getByRole('heading', { level: 2, name: /Ended shares/ })).toBeInTheDocument();

    // Live rows come first, and only they are visible before the ended group opens.
    expect(screen.getByText('Pending One')).toBeInTheDocument();
    expect(screen.queryByText('Old Club')).toBeNull();
    expect(screen.queryByText('Gone Contact')).toBeNull();
    const toggle = screen.getByRole('button', { name: /Ended shares/ });
    expect(toggle).toHaveAttribute('aria-expanded', 'false');

    fireEvent.click(toggle);
    expect(toggle).toHaveAttribute('aria-expanded', 'true');
    const ended = await rowFor('Old Club');
    expect(within(ended).getByText('Declined')).toBeInTheDocument();
    // Present-tense member counts only on live grants.
    expect(within(ended).queryByText(/Shares \d+ people right now/)).toBeNull();
    expect(
      within(await rowFor('Book Club')).getByText('Shares 12 people right now'),
    ).toBeInTheDocument();
    expect(screen.queryAllByText(/people right now/)).toHaveLength(1);
    expect(live.compareDocumentPosition(ended) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  test('shows no Ended section when every grant is live, and no Live heading when none is', async () => {
    mockApi('get', '/share-grants/outgoing', page([grant()]));
    renderPage();
    await rowFor('Alice Anderson');
    expect(screen.queryByRole('heading', { name: /Ended shares/ })).toBeNull();
    cleanup();
    mockApi('get', '/share-grants/outgoing', page([grant({ status: 'revoked' })]));
    renderPage();
    expect(await screen.findByRole('button', { name: /Ended shares/ })).toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Live shares' })).toBeNull();
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
    // The revoked grant moves out of the live list into the collapsed Ended section.
    fireEvent.click(await screen.findByRole('button', { name: /Ended shares/ }));
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
    fireEvent.click(
      screen.getByRole('button', { name: /^Review and re-confirm Alice Anderson for bob/ }),
    );
    const dialog = await screen.findByRole('dialog', { name: 'Share these sections again?' });
    expect(reconfirm).toHaveLength(0);
    fireEvent.click(within(dialog).getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(reconfirm).toHaveLength(1));
    await waitFor(() => expect(screen.queryByText(/Hidden from bob/)).toBeNull());
    expect(screen.queryByRole('button', { name: /^Review and re-confirm/ })).toBeNull();
  });

  test('every action button name contains its visible text (#2700)', async () => {
    mockApi('get', '/share-grants/outgoing', page([grant({ reconfirm_sections: ['addresses'] })]));
    renderPage();
    await rowFor('Alice Anderson');
    const buttons = screen.getAllByRole('button').filter((b) => b.hasAttribute('aria-label'));
    expect(buttons.length).toBe(3); // re-confirm, edit, revoke
    for (const b of buttons) {
      const visible = (b.textContent ?? '').trim().toLowerCase();
      const name = (b.getAttribute('aria-label') ?? '').toLowerCase();
      expect(name, `aria-label "${name}" must contain "${visible}"`).toContain(visible);
    }
  });

  test('a grant with nothing to re-confirm has no re-confirm action', async () => {
    mockApi('get', '/share-grants/outgoing', page([grant()]));
    renderPage();
    await screen.findByText('Alice Anderson');
    expect(screen.queryByRole('button', { name: /^Review and re-confirm/ })).toBeNull();
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

  test('Save after a 412 retries with the reloaded revision and closes the dialog (#2694)', async () => {
    let listCalls = 0;
    server.use(
      http.get(`${API_BASE_URL}/share-grants/outgoing`, () => {
        listCalls += 1;
        // The grant was changed elsewhere: the reload returns the next revision.
        return HttpResponse.json(page([grant({ revision: listCalls === 1 ? 3 : 4 })]));
      }),
    );
    const sent: { ifMatch: string | null; body: unknown }[] = [];
    server.use(
      http.patch(`${API_BASE_URL}/share-grants/g1`, async ({ request }) => {
        sent.push({ ifMatch: request.headers.get('If-Match'), body: await request.clone().json() });
        if (sent.length === 1) {
          return errorEnvelope(412, 'PRECONDITION_FAILED', 'stale');
        }
        return HttpResponse.json({ share_grant: grant({ revision: 5, sections: ['emails'] }) });
      }),
    );
    renderPage();
    fireEvent.click(await screen.findByRole('button', { name: /^Edit / }));
    const dialog = await screen.findByRole('dialog', { name: 'Edit share' });
    fireEvent.click(within(dialog).getByRole('checkbox', { name: 'Phones' }));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
    expect(await within(dialog).findByText(/changed elsewhere/)).toBeInTheDocument();
    await waitFor(() => expect(listCalls).toBeGreaterThanOrEqual(2));

    // The user's edit (Phones unticked) survived the reload.
    expect(within(dialog).getByRole('checkbox', { name: 'Phones' })).not.toBeChecked();
    await waitFor(() => expect(within(dialog).getByRole('button', { name: 'Save' })).toBeEnabled());
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));

    await waitFor(() => expect(sent).toHaveLength(2));
    expect(sent[0].ifMatch).toBe('"3"');
    expect(sent[1].ifMatch).toBe('"4"');
    expect(sent[1].body).toEqual({ sections: ['emails'] });
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  });

  test('a 412 for a grant that ended meanwhile closes the dialog and says so (#2694)', async () => {
    let listCalls = 0;
    server.use(
      http.get(`${API_BASE_URL}/share-grants/outgoing`, () => {
        listCalls += 1;
        return HttpResponse.json(
          page([listCalls === 1 ? grant() : grant({ status: 'revoked', revision: 4 })]),
        );
      }),
    );
    mockApi('patch', '/share-grants/g1', errorEnvelope(412, 'PRECONDITION_FAILED', 'stale'));
    renderPage();
    fireEvent.click(await screen.findByRole('button', { name: /^Edit / }));
    const dialog = await screen.findByRole('dialog', { name: 'Edit share' });
    fireEvent.click(within(dialog).getByRole('checkbox', { name: 'Phones' }));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(await screen.findByRole('alert')).toHaveTextContent(/no longer available to edit/);
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
