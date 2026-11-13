import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import {
  adoptContactEntityLink,
  deleteEntity,
  type Entity,
  type EntityContactLink,
  type EntityDeleteResponse,
} from '../api/entities';
import EntityDeleteDialog from './EntityDeleteDialog';

vi.mock('../api/entities', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/entities')>();
  return { ...actual, deleteEntity: vi.fn(), adoptContactEntityLink: vi.fn() };
});
const deleteMock = vi.mocked(deleteEntity);
const adoptMock = vi.mocked(adoptContactEntityLink);

const entity: Entity = {
  id: 'e1',
  kind: 'organisation',
  name: 'Acme',
  core: {},
  sensitivity: 'normal',
  revision: 3,
  link_count: 2,
  projection_count: 2,
  created_at: '',
  updated_at: '',
};

const contact = (id: number): EntityContactLink => ({
  link_id: `l${id}`,
  contact_id: id,
  contact_vcard_uid: `u${id}`,
  firstname: 'C',
  lastname: String(id),
  role: 'employee',
  role_label: '',
  is_primary: true,
  sensitivity: 'normal',
});

const response: EntityDeleteResponse = { message: 'ok', link_count: 2, projection_count: 2 };

function renderDialog(props: Partial<React.ComponentProps<typeof EntityDeleteDialog>> = {}) {
  const onClose = vi.fn();
  const onDeleted = vi.fn();
  render(
    <EntityDeleteDialog
      open
      entity={entity}
      contacts={[contact(1), contact(2)]}
      onClose={onClose}
      onDeleted={onDeleted}
      {...props}
    />,
  );
  return { onClose, onDeleted };
}

beforeEach(() => {
  deleteMock.mockReset();
  adoptMock.mockReset();
});
afterEach(cleanup);

// issue #2570: the busy and error states of the delete confirmation.
test('while the delete is in flight both buttons are disabled, then onDeleted fires with the result', async () => {
  let resolve!: (r: EntityDeleteResponse) => void;
  deleteMock.mockReturnValue(new Promise((r) => (resolve = r)));
  const { onDeleted } = renderDialog();
  fireEvent.click(screen.getByRole('button', { name: 'Delete' }));

  await waitFor(() => expect(screen.getByRole('button', { name: 'Delete' })).toBeDisabled());
  expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled();
  expect(deleteMock).toHaveBeenCalledWith('e1', 3);
  expect(onDeleted).not.toHaveBeenCalled();

  await act(async () => resolve(response));
  expect(onDeleted).toHaveBeenCalledWith(response);
});

test('a failed delete shows the error, re-enables the buttons and does not report a delete', async () => {
  deleteMock.mockRejectedValue(new Error('revision conflict'));
  const { onDeleted } = renderDialog();
  fireEvent.click(screen.getByRole('button', { name: 'Delete' }));

  expect(await screen.findByRole('alert')).toHaveTextContent('revision conflict');
  expect(onDeleted).not.toHaveBeenCalled();
  expect(screen.getByRole('button', { name: 'Delete' })).toBeEnabled();
  expect(screen.getByRole('button', { name: 'Cancel' })).toBeEnabled();
});

test('a failed adopt reports the count and never deletes the entity', async () => {
  adoptMock.mockRejectedValueOnce(new Error('nope')).mockResolvedValueOnce({} as never);
  const { onDeleted } = renderDialog();
  fireEvent.click(screen.getByRole('checkbox'));
  fireEvent.click(screen.getByRole('button', { name: 'Delete' }));

  expect(await screen.findByRole('alert')).toHaveTextContent(
    "Couldn't copy it to 1 contact, so nothing was deleted.",
  );
  expect(adoptMock).toHaveBeenCalledTimes(2);
  expect(deleteMock).not.toHaveBeenCalled();
  expect(onDeleted).not.toHaveBeenCalled();
  expect(screen.getByRole('button', { name: 'Delete' })).toBeEnabled();
});

test('without ticking adopt, nothing is copied before the delete', async () => {
  deleteMock.mockResolvedValue(response);
  const { onDeleted } = renderDialog();
  fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
  await waitFor(() => expect(onDeleted).toHaveBeenCalled());
  expect(adoptMock).not.toHaveBeenCalled();
});
