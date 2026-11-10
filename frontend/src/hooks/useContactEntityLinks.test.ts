import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import {
  adoptContactEntityLink,
  type ContactEntityLink,
  createContactEntityLink,
  deleteContactEntityLink,
  linkContactEntityFromExisting,
  listContactEntityLinks,
  updateContactEntityLink,
} from '../api/entities';
import { useContactEntityLinks } from './useContactEntityLinks';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock('../api/entities', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/entities')>();
  return {
    ...actual,
    listContactEntityLinks: vi.fn(),
    createContactEntityLink: vi.fn(),
    updateContactEntityLink: vi.fn(),
    deleteContactEntityLink: vi.fn(),
    adoptContactEntityLink: vi.fn(),
    linkContactEntityFromExisting: vi.fn(),
  };
});
const listMock = vi.mocked(listContactEntityLinks);

function link(overrides: Partial<ContactEntityLink> = {}): ContactEntityLink {
  return {
    id: 'l1',
    contact_vcard_uid: 'u',
    entity_id: 'e1',
    entity_name: 'Acme',
    kind: 'organisation',
    role: 'employee',
    role_label: '',
    is_primary: false,
    sensitivity: 'normal',
    current: true,
    projects_as: '',
    revision: 2,
    created_at: '',
    updated_at: '',
    ...overrides,
  };
}

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.clearAllMocks();
  listMock.mockResolvedValue([link()]);
});

test('loads the contact’s links and does nothing without a contact id', async () => {
  const { result } = renderHook(() => useContactEntityLinks(7));
  await waitFor(() => expect(result.current.links).toHaveLength(1));
  expect(listMock).toHaveBeenCalledWith(7);

  listMock.mockClear();
  renderHook(() => useContactEntityLinks(undefined));
  expect(listMock).not.toHaveBeenCalled();
});

test('a load failure is exposed and does not throw', async () => {
  listMock.mockRejectedValueOnce(new Error('boom'));
  const { result } = renderHook(() => useContactEntityLinks(7));
  await waitFor(() => expect(result.current.error).toBeTruthy());
  expect(result.current.links).toEqual([]);
});

test('create and edit refresh the links and tell the page its record changed', async () => {
  const onRecordChanged = vi.fn();
  const { result } = renderHook(() => useContactEntityLinks(7, { onRecordChanged }));
  await waitFor(() => expect(result.current.links).toHaveLength(1));

  vi.mocked(createContactEntityLink).mockResolvedValue(link({ id: 'l2' }));
  await act(async () => {
    await result.current.save({ entity_id: 'e2', role: 'owner', is_primary: true });
  });
  expect(createContactEntityLink).toHaveBeenCalledWith(7, {
    entity_id: 'e2',
    role: 'owner',
    is_primary: true,
  });
  expect(listMock).toHaveBeenCalledTimes(2);
  expect(onRecordChanged).toHaveBeenCalledTimes(1);

  vi.mocked(updateContactEntityLink).mockResolvedValue(link());
  await act(async () => {
    await result.current.save({ entity_id: 'e1', role: 'owner' }, link());
  });
  expect(updateContactEntityLink).toHaveBeenCalledWith(
    7,
    'l1',
    { entity_id: 'e1', role: 'owner' },
    2,
  );
  expect(onRecordChanged).toHaveBeenCalledTimes(2);
});

test('toggling primary re-sends the whole link so no field is lost', async () => {
  listMock.mockResolvedValue([
    link({ role_label: 'CTO', period: { start: { year: 2019 } }, sensitivity: 'private' }),
  ]);
  const { result } = renderHook(() => useContactEntityLinks(7));
  await waitFor(() => expect(result.current.links).toHaveLength(1));
  vi.mocked(updateContactEntityLink).mockResolvedValue(link());
  await act(async () => {
    await result.current.setPrimary(result.current.links[0], true);
  });
  expect(updateContactEntityLink).toHaveBeenCalledWith(
    7,
    'l1',
    {
      entity_id: 'e1',
      role: 'employee',
      role_label: 'CTO',
      is_primary: true,
      period: { start: { year: 2019 } },
      sensitivity: 'private',
    },
    2,
  );
});

test('remove, adopt and link-from-existing each refresh and notify', async () => {
  const onRecordChanged = vi.fn();
  const { result } = renderHook(() => useContactEntityLinks(7, { onRecordChanged }));
  await waitFor(() => expect(result.current.links).toHaveLength(1));

  vi.mocked(deleteContactEntityLink).mockResolvedValue(undefined);
  await act(async () => {
    await result.current.remove(result.current.links[0]);
  });
  expect(deleteContactEntityLink).toHaveBeenCalledWith(7, 'l1');

  vi.mocked(adoptContactEntityLink).mockResolvedValue({
    adopted: true,
    element_id: 'x',
    element_kind: 'organization',
    link: link(),
  });
  await act(async () => {
    await result.current.adopt(link());
  });
  expect(adoptContactEntityLink).toHaveBeenCalledWith(7, 'l1');

  vi.mocked(linkContactEntityFromExisting).mockResolvedValue({
    entity: {} as never,
    entity_created: true,
    link: link(),
  });
  await act(async () => {
    await result.current.linkFromExisting({
      role: 'employee',
      source: { kind: 'organization', id: 'o1' },
    });
  });
  expect(linkContactEntityFromExisting).toHaveBeenCalledTimes(1);
  expect(onRecordChanged).toHaveBeenCalledTimes(3);
});

test('a failed mutation rethrows and does not notify', async () => {
  const onRecordChanged = vi.fn();
  const { result } = renderHook(() => useContactEntityLinks(7, { onRecordChanged }));
  await waitFor(() => expect(result.current.links).toHaveLength(1));
  vi.mocked(deleteContactEntityLink).mockRejectedValue(new Error('nope'));
  await act(async () => {
    await expect(result.current.remove(result.current.links[0])).rejects.toThrow('nope');
  });
  expect(onRecordChanged).not.toHaveBeenCalled();
});

test('a slow load for the previous contact never replaces the new contact’s links', async () => {
  let resolveSlow: (v: ContactEntityLink[]) => void = () => {};
  listMock.mockImplementationOnce(
    () =>
      new Promise((r) => {
        resolveSlow = r;
      }),
  );
  listMock.mockResolvedValueOnce([link({ id: 'new' })]);
  const { result, rerender } = renderHook(({ id }) => useContactEntityLinks(id), {
    initialProps: { id: 7 },
  });
  rerender({ id: 8 });
  await waitFor(() => expect(result.current.links.map((l) => l.id)).toEqual(['new']));
  await act(async () => {
    resolveSlow([link({ id: 'stale' })]);
  });
  expect(result.current.links.map((l) => l.id)).toEqual(['new']);
});
