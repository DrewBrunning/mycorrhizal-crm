import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { type Entity, listEntities } from '../api/entities';
import LinkFromOwnValueDialog, { type OwnValue } from './LinkFromOwnValueDialog';

vi.mock('../api/entities', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/entities')>();
  return { ...actual, listEntities: vi.fn() };
});
const listMock = vi.mocked(listEntities);

const org: OwnValue = { kind: 'organization', id: 'org-1', index: 0, label: 'Acme' };
const existing: Entity = {
  id: 'e9',
  kind: 'organisation',
  name: 'Acme Inc',
  core: {},
  sensitivity: 'normal',
  revision: 1,
  created_at: '',
  updated_at: '',
};

function renderDialog(onLink = vi.fn().mockResolvedValue(undefined), values = [org]) {
  const onClose = vi.fn();
  render(<LinkFromOwnValueDialog open values={values} onClose={onClose} onLink={onLink} />);
  return { onClose, onLink };
}

beforeEach(() => {
  listMock.mockReset();
  listMock.mockResolvedValue({ entities: [], next_cursor: '', limit: 5 } as never);
});
afterEach(cleanup);

// issue #2570: saving and error states of "link from existing".
test('while linking, Link and Cancel are disabled; success closes the dialog', async () => {
  let resolve!: () => void;
  const onLink = vi.fn().mockReturnValue(new Promise<void>((r) => (resolve = r)));
  const { onClose } = renderDialog(onLink);
  fireEvent.click(screen.getByRole('button', { name: 'Link' }));

  await waitFor(() => expect(screen.getByRole('button', { name: 'Link' })).toBeDisabled());
  expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled();
  expect(onLink).toHaveBeenCalledWith({
    role: 'employee',
    source: { kind: 'organization', id: 'org-1' },
  });
  expect(onClose).not.toHaveBeenCalled();

  await act(async () => resolve());
  expect(onClose).toHaveBeenCalled();
});

test('a failed link shows the error, stays open and re-enables the buttons', async () => {
  const onLink = vi.fn().mockRejectedValue(new Error('link refused'));
  const { onClose } = renderDialog(onLink);
  fireEvent.click(screen.getByRole('button', { name: 'Link' }));

  expect(await screen.findByRole('alert')).toHaveTextContent('link refused');
  expect(onClose).not.toHaveBeenCalled();
  expect(screen.getByRole('button', { name: 'Link' })).toBeEnabled();
  expect(screen.getByRole('button', { name: 'Cancel' })).toBeEnabled();
});

test('a failed match lookup leaves only the create-new choice; a found match can be picked', async () => {
  listMock.mockRejectedValueOnce(new Error('lookup down'));
  renderDialog();
  await waitFor(() =>
    expect(listMock).toHaveBeenCalledWith({ kind: 'organisation', q: 'Acme', limit: 5 }),
  );
  expect(screen.getAllByRole('radio')).toHaveLength(1);
  cleanup();

  listMock.mockResolvedValueOnce({ entities: [existing], next_cursor: '', limit: 5 } as never);
  const onLink = vi.fn().mockResolvedValue(undefined);
  renderDialog(onLink);
  fireEvent.click(await screen.findByRole('radio', { name: /Use the existing organisation/ }));
  fireEvent.click(screen.getByRole('button', { name: 'Link' }));
  await waitFor(() => expect(onLink).toHaveBeenCalled());
  expect(onLink.mock.calls[0][0]).toMatchObject({ entity_id: 'e9', role: 'employee' });
});
