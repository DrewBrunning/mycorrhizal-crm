import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { ApiError } from '../api/client';
import { type ContactEntityLink, createEntity, type Entity } from '../api/entities';
import ContactEntityLinkDialog from './ContactEntityLinkDialog';
import type { EntityChoice } from './EntityPicker';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(cleanup);

vi.mock('../api/entities', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/entities')>();
  return { ...actual, createEntity: vi.fn() };
});

// EntityPicker has its own tests (search, debounce, stale responses); here it is
// a stand-in that offers one existing entity and one "create" choice.
const acme: Entity = {
  id: 'ent-acme',
  kind: 'organisation',
  name: 'Acme Corp',
  core: {},
  sensitivity: 'normal',
  revision: 1,
  created_at: '2026-11-01T00:00:00Z',
  updated_at: '2026-11-01T00:00:00Z',
};
vi.mock('./EntityPicker', () => ({
  default: ({ kind, onChange }: { kind: string; onChange: (c: EntityChoice | null) => void }) => (
    <div>
      <span>picker for {kind}</span>
      <button type="button" onClick={() => onChange({ create: false, entity: acme })}>
        choose existing
      </button>
      <button type="button" onClick={() => onChange({ create: true, name: 'Newco' })}>
        choose new
      </button>
    </div>
  ),
}));

const createEntityMock = vi.mocked(createEntity);

function existingLink(overrides: Partial<ContactEntityLink> = {}): ContactEntityLink {
  return {
    id: 'l1',
    contact_vcard_uid: 'uid-1',
    entity_id: 'ent-acme',
    entity_name: 'Acme Corp',
    kind: 'organisation',
    role: 'owner',
    role_label: 'Founder',
    is_primary: true,
    period: { start: { year: 2019 }, end: { year: 2021 } },
    sensitivity: 'private',
    current: false,
    projects_as: '',
    revision: 3,
    created_at: '2026-11-01T00:00:00Z',
    updated_at: '2026-11-01T00:00:00Z',
    ...overrides,
  } as ContactEntityLink;
}

function renderDialog(
  link: ContactEntityLink | null = null,
  onSave = vi.fn().mockResolvedValue(undefined),
) {
  const onClose = vi.fn();
  render(<ContactEntityLinkDialog open link={link} onClose={onClose} onSave={onSave} />);
  return { onClose, onSave };
}

const saveButton = () => screen.getByRole('button', { name: 'Save' }) as HTMLButtonElement;

beforeEach(() => {
  createEntityMock.mockReset();
});

test('add mode: saving without choosing an entity is refused with an explanation', () => {
  const { onSave } = renderDialog();
  expect(screen.getByText('Link to an organisation, place or pet')).toBeDefined();

  fireEvent.click(saveButton());
  expect(
    screen.getByText('Choose an organisation, place or pet, or type a name to create one.'),
  ).toBeDefined();
  expect(onSave).not.toHaveBeenCalled();
});

test('add mode: linking an existing entity sends its id, the default role and sensitivity, then closes', async () => {
  const { onSave, onClose } = renderDialog();
  fireEvent.click(screen.getByRole('button', { name: 'choose existing' }));
  fireEvent.click(saveButton());

  await waitFor(() => expect(onClose).toHaveBeenCalled());
  expect(onSave).toHaveBeenCalledWith(
    {
      entity_id: 'ent-acme',
      role: 'employee',
      role_label: '',
      is_primary: false,
      sensitivity: 'normal',
    },
    null,
  );
  expect(createEntityMock).not.toHaveBeenCalled();
});

test('add mode: choosing "create" first creates the entity, then links it', async () => {
  createEntityMock.mockResolvedValue({ ...acme, id: 'ent-new', name: 'Newco' });
  const { onSave } = renderDialog();
  fireEvent.click(screen.getByRole('button', { name: 'choose new' }));
  fireEvent.click(screen.getByRole('checkbox'));
  fireEvent.click(saveButton());

  await waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(createEntityMock).toHaveBeenCalledWith(
    expect.objectContaining({ kind: 'organisation', name: 'Newco', sensitivity: 'normal' }),
  );
  expect(onSave.mock.calls[0][0]).toMatchObject({ entity_id: 'ent-new', is_primary: true });
});

test('a failure creating the entity is shown and nothing is linked', async () => {
  createEntityMock.mockRejectedValue(new ApiError('Could not create', 'INTERNAL_ERROR', 500));
  const { onSave, onClose } = renderDialog();
  fireEvent.click(screen.getByRole('button', { name: 'choose new' }));
  fireEvent.click(saveButton());

  expect(await screen.findByText('Could not create')).toBeDefined();
  expect(onSave).not.toHaveBeenCalled();
  expect(onClose).not.toHaveBeenCalled();
  expect(saveButton().disabled).toBe(false);
});

test('years must be four digits with the end not before the start; valid years become the period', async () => {
  const { onSave } = renderDialog();
  fireEvent.click(screen.getByRole('button', { name: 'choose existing' }));

  fireEvent.change(screen.getByLabelText('From (year)'), { target: { value: '20x9' } });
  fireEvent.click(saveButton());
  expect(
    screen.getByText('Enter years as four digits, with the end not before the start.'),
  ).toBeDefined();

  fireEvent.change(screen.getByLabelText('From (year)'), { target: { value: '2022' } });
  fireEvent.change(screen.getByLabelText('To (year)'), { target: { value: '2020' } });
  fireEvent.click(saveButton());
  expect(
    screen.getByText('Enter years as four digits, with the end not before the start.'),
  ).toBeDefined();
  expect(onSave).not.toHaveBeenCalled();

  fireEvent.change(screen.getByLabelText('To (year)'), { target: { value: '2024' } });
  fireEvent.click(saveButton());
  await waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave.mock.calls[0][0].period).toEqual({ start: { year: 2022 }, end: { year: 2024 } });
});

test("switching the kind clears the chosen entity and resets the role to that kind's first role", () => {
  renderDialog();
  fireEvent.click(screen.getByRole('button', { name: 'choose existing' }));
  expect(screen.getByText('picker for organisation')).toBeDefined();

  fireEvent.mouseDown(screen.getByLabelText('Kind'));
  fireEvent.click(screen.getByRole('option', { name: 'Place' }));

  expect(screen.getByText('picker for place')).toBeDefined();
  // The role list is now the place vocabulary, starting with its first role.
  expect(screen.getByLabelText('Role').textContent).toBe('Resident');
  // The previously chosen entity does not carry over to the new kind.
  fireEvent.click(saveButton());
  expect(
    screen.getByText('Choose an organisation, place or pet, or type a name to create one.'),
  ).toBeDefined();
});

test('a pet link has no primary option and never sends is_primary', async () => {
  const { onSave } = renderDialog();
  fireEvent.click(screen.getByRole('checkbox')); // primary on, as an organisation
  fireEvent.mouseDown(screen.getByLabelText('Kind'));
  fireEvent.click(screen.getByRole('option', { name: 'Pet' }));

  expect(screen.queryByRole('checkbox')).toBeNull();
  fireEvent.click(screen.getByRole('button', { name: 'choose existing' }));
  fireEvent.click(saveButton());
  await waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave.mock.calls[0][0]).toMatchObject({ role: 'owner', is_primary: false });
});

test('edit mode: shows the link prefilled with no entity picker or kind, and saves with the link', async () => {
  const link = existingLink();
  const { onSave, onClose } = renderDialog(link);

  expect(screen.getByText('Edit link to Acme Corp')).toBeDefined();
  expect(screen.queryByLabelText('Kind')).toBeNull();
  expect(screen.queryByText(/picker for/)).toBeNull();
  expect((screen.getByLabelText('Role detail (optional)') as HTMLInputElement).value).toBe(
    'Founder',
  );
  expect((screen.getByLabelText('From (year)') as HTMLInputElement).value).toBe('2019');
  expect((screen.getByLabelText('To (year)') as HTMLInputElement).value).toBe('2021');
  expect((screen.getByRole('checkbox') as HTMLInputElement).checked).toBe(true);

  fireEvent.change(screen.getByLabelText('Role detail (optional)'), {
    target: { value: '  CEO  ' },
  });
  fireEvent.click(saveButton());

  await waitFor(() => expect(onClose).toHaveBeenCalled());
  expect(onSave).toHaveBeenCalledWith(
    {
      entity_id: 'ent-acme',
      role: 'owner',
      role_label: 'CEO',
      is_primary: true,
      period: { start: { year: 2019 }, end: { year: 2021 } },
      sensitivity: 'private',
    },
    link,
  );
});

test('a failed save shows the error, keeps the dialog open, and can be retried', async () => {
  const onSave = vi
    .fn()
    .mockRejectedValueOnce(new ApiError('The link could not be saved', 'CONFLICT', 409))
    .mockResolvedValueOnce(undefined);
  const { onClose } = renderDialog(existingLink(), onSave);

  fireEvent.click(saveButton());
  expect(await screen.findByText('The link could not be saved')).toBeDefined();
  expect(onClose).not.toHaveBeenCalled();

  fireEvent.click(saveButton());
  await waitFor(() => expect(onClose).toHaveBeenCalled());
  expect(onSave).toHaveBeenCalledTimes(2);
});

test('editing a field clears a stale error', async () => {
  renderDialog();
  fireEvent.click(saveButton());
  expect(screen.getByText(/Choose an organisation, place or pet/)).toBeDefined();
  fireEvent.change(screen.getByLabelText('Role detail (optional)'), { target: { value: 'x' } });
  expect(screen.queryByText(/Choose an organisation, place or pet/)).toBeNull();
});

test('Cancel closes at once when nothing changed, and asks before discarding edits', () => {
  const first = renderDialog();
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
  expect(first.onClose).toHaveBeenCalledTimes(1);
  cleanup();

  const second = renderDialog();
  fireEvent.change(screen.getByLabelText('Role detail (optional)'), { target: { value: 'x' } });
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
  expect(screen.getByText('Discard unsaved changes?')).toBeDefined();
  expect(second.onClose).not.toHaveBeenCalled();
});
