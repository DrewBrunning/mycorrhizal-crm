import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';
import '../i18n/config';
import { DateFormatProvider } from '../DateFormatProvider';
import { errorEnvelope, mockApi, setupMswServer } from '../test/mswServer';
import StandingShareForm from './StandingShareForm';

setupMswServer();
afterEach(cleanup);

const directory = {
  users: [
    { id: 2, username: 'bob' },
    { id: 3, username: 'carol' },
  ],
};
const ownerView = (over = {}) => ({
  share_grant: { id: 'g1', to_username: 'bob', status: 'pending', ...over },
});

function setup(props: Partial<React.ComponentProps<typeof StandingShareForm>> = {}) {
  const onCreated = vi.fn();
  const onCancel = vi.fn();
  mockApi('get', '/users/directory', directory);
  render(
    <DateFormatProvider>
      <StandingShareForm
        entityType="contact"
        entityId="uid-1"
        onCreated={onCreated}
        onCancel={onCancel}
        {...props}
      />
    </DateFormatProvider>,
  );
  return { onCreated, onCancel };
}

async function pickRecipient(name: string) {
  const input = await screen.findByRole('combobox', { name: 'Recipient' });
  fireEvent.keyDown(input, { key: 'ArrowDown' });
  fireEvent.click(await screen.findByRole('option', { name }));
}

describe('StandingShareForm', () => {
  test('a contact grant starts with NO sections selected and cannot be created until one is chosen', async () => {
    setup();
    await pickRecipient('bob');
    for (const box of screen.getAllByRole('checkbox')) expect(box).not.toBeChecked();
    expect(screen.getByRole('button', { name: 'Share' })).toBeDisabled();
    expect(screen.getByText(/nothing is shared by default/i)).toBeInTheDocument();
  });

  test('does not offer the sensitive reveal flow: private and secret items are never shared', async () => {
    setup();
    await screen.findByRole('combobox', { name: 'Recipient' });
    expect(screen.getByText(/Private and secret items are never shared/)).toBeInTheDocument();
    expect(screen.queryByText(/reveal/i)).not.toBeInTheDocument();
  });

  test('says revocation is not recall and that later additions are visible immediately', async () => {
    setup();
    expect(await screen.findByText(/does not recall/i)).toBeInTheDocument();
    expect(screen.getByText(/visible to them right away/i)).toBeInTheDocument();
  });

  test('previews the exact projection for the chosen sections, then submits only those sections', async () => {
    const preview = mockApi('get', '/export/jscontact', [
      { name: { full: 'Alice Anderson' }, emails: { e1: { address: 'alice@example.com' } } },
    ]);
    const create = mockApi('post', '/share-grants', ownerView());
    const { onCreated } = setup();
    await pickRecipient('bob');

    fireEvent.click(screen.getByRole('checkbox', { name: 'Emails' }));
    const previewRegion = await screen.findByRole('region', { name: 'What they will see' });
    expect(await within(previewRegion).findByText('alice@example.com')).toBeInTheDocument();
    expect(within(previewRegion).getByText('Alice Anderson')).toBeInTheDocument();
    expect(preview[0].search.get('sections')).toBe('emails');
    expect(preview[0].search.get('vcard_uid')).toBe('uid-1');
    expect(preview[0].search.get('include_sensitive')).toBeNull();

    fireEvent.click(screen.getByRole('button', { name: 'Share' }));
    await waitFor(() => expect(onCreated).toHaveBeenCalledTimes(1));
    expect(create[0].body).toEqual({
      to_user_id: 2,
      entity_type: 'contact',
      entity_id: 'uid-1',
      sections: ['emails'],
    });
    expect(create[0].headers.get('Idempotency-Key')).toBeTruthy();
  });

  test('sends the chosen expiry as an end-of-day instant', async () => {
    mockApi('get', '/export/jscontact', [{ emails: { e1: { address: 'a@example.com' } } }]);
    const create = mockApi('post', '/share-grants', ownerView());
    setup();
    await pickRecipient('carol');
    fireEvent.click(screen.getByRole('checkbox', { name: 'Phones' }));
    fireEvent.change(screen.getByLabelText('Expires on (optional)'), {
      target: { value: '04.03.2099' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Share' }));
    await waitFor(() => expect(create).toHaveLength(1));
    const body = create[0].body as { expires_at: string; to_user_id: number };
    expect(body.to_user_id).toBe(3);
    expect(new Date(body.expires_at).getTime()).toBeGreaterThan(Date.parse('2099-03-04T00:00:00'));
    expect(new Date(body.expires_at).getFullYear()).toBe(2099);
  });

  test('note, activity and circle grants carry no sections and no preview request', async () => {
    const create = mockApi('post', '/share-grants', ownerView());
    const exp = mockApi('get', '/export/jscontact', []);
    const { onCreated } = setup({ entityType: 'circle', entityId: '7' });
    await pickRecipient('bob');
    expect(screen.queryAllByRole('checkbox')).toHaveLength(0);
    expect(screen.getByText(/current members/i)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Share' }));
    await waitFor(() => expect(onCreated).toHaveBeenCalled());
    expect(create[0].body).toEqual({ to_user_id: 2, entity_type: 'circle', entity_id: '7' });
    expect(exp).toHaveLength(0);
  });

  test('shows the server message and stays open when create fails (duplicate grant)', async () => {
    mockApi('post', '/share-grants', errorEnvelope(409, 'CONFLICT', 'dup'));
    mockApi('get', '/export/jscontact', [{}]);
    const { onCreated } = setup({ entityType: 'note', entityId: 'n1' });
    await pickRecipient('bob');
    fireEvent.click(screen.getByRole('button', { name: 'Share' }));
    expect(await screen.findByRole('alert')).toBeInTheDocument();
    expect(onCreated).not.toHaveBeenCalled();
  });

  test('cancel calls onCancel', async () => {
    const { onCancel } = setup({ entityType: 'note', entityId: 'n1' });
    await screen.findByRole('combobox', { name: 'Recipient' });
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(onCancel).toHaveBeenCalled();
  });
});

test('says so when the chosen sections hold nothing shareable right now', async () => {
  mockApi('get', '/export/jscontact', [{ name: { full: 'Alice Anderson' } }]);
  setup();
  await pickRecipient('bob');
  fireEvent.click(screen.getByRole('checkbox', { name: 'Emails' }));
  expect(
    await screen.findByText(/Nothing in these sections is shared right now/),
  ).toBeInTheDocument();
});
