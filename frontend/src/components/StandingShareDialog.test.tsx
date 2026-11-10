import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';
import '../i18n/config';
import { DateFormatProvider } from '../DateFormatProvider';
import { mockApi, setupMswServer } from '../test/mswServer';
import ShareContactDialog from './ShareContactDialog';
import StandingShareDialog from './StandingShareDialog';

setupMswServer();
afterEach(cleanup);

const directory = { users: [{ id: 2, username: 'bob' }] };
const grant = { share_grant: { id: 'g1', to_username: 'bob', status: 'pending' } };

async function pickBob() {
  const input = await screen.findByRole('combobox', { name: 'Recipient' });
  fireEvent.keyDown(input, { key: 'ArrowDown' });
  fireEvent.click(await screen.findByRole('option', { name: 'bob' }));
}

describe('StandingShareDialog (note, activity, circle)', () => {
  test.each([
    ['note', 'Share this note'],
    ['activity', 'Share this activity'],
    ['circle', 'Share this circle'],
  ] as const)('%s dialog is titled, creates a grant and reports it', async (type, title) => {
    mockApi('get', '/users/directory', directory);
    const create = mockApi('post', '/share-grants', grant);
    const onClose = vi.fn();
    const onShared = vi.fn();
    render(
      <DateFormatProvider>
        <StandingShareDialog
          open
          onClose={onClose}
          entityType={type}
          entityId="e-1"
          onShared={onShared}
        />
      </DateFormatProvider>,
    );
    expect(await screen.findByRole('heading', { name: title })).toBeInTheDocument();
    await pickBob();
    fireEvent.click(screen.getByRole('button', { name: 'Share' }));
    await waitFor(() =>
      expect(onShared).toHaveBeenCalledWith(expect.objectContaining({ id: 'g1' })),
    );
    expect(onClose).toHaveBeenCalled();
    expect(create[0].body).toEqual({ to_user_id: 2, entity_type: type, entity_id: 'e-1' });
  });
});

describe('ShareContactDialog standing mode', () => {
  test('defaults to the one-time copy and never contacts the share-grant API', async () => {
    mockApi('get', '/users/directory', directory);
    const grants = mockApi('post', '/share-grants', grant);
    render(
      <DateFormatProvider>
        <ShareContactDialog open onClose={vi.fn()} vcardUID="uid-1" />
      </DateFormatProvider>,
    );
    expect(await screen.findByRole('radio', { name: 'One-time copy' })).toBeChecked();
    expect(screen.getByText(/can't be recalled once sent/i)).toBeInTheDocument();
    expect(grants).toHaveLength(0);
  });

  test('choosing a standing share swaps the form, creates a grant and leaves the one-time API alone', async () => {
    mockApi('get', '/users/directory', directory);
    mockApi('get', '/export/jscontact', [{ emails: { e1: { address: 'a@example.com' } } }]);
    const grants = mockApi('post', '/share-grants', grant);
    const oneTime = mockApi('post', '/contact-shares', {});
    const onClose = vi.fn();
    const onStandingShared = vi.fn();
    render(
      <DateFormatProvider>
        <ShareContactDialog
          open
          onClose={onClose}
          vcardUID="uid-1"
          onStandingShared={onStandingShared}
        />
      </DateFormatProvider>,
    );
    fireEvent.click(await screen.findByRole('radio', { name: 'Standing share' }));
    expect(screen.queryByText(/can't be recalled once sent/i)).not.toBeInTheDocument();
    await pickBob();
    fireEvent.click(screen.getByRole('checkbox', { name: 'Emails' }));
    fireEvent.click(screen.getByRole('button', { name: 'Share' }));
    await waitFor(() => expect(onStandingShared).toHaveBeenCalled());
    expect(onClose).toHaveBeenCalled();
    expect(grants[0].body).toEqual({
      to_user_id: 2,
      entity_type: 'contact',
      entity_id: 'uid-1',
      sections: ['emails'],
    });
    expect(oneTime).toHaveLength(0);
  });
});
