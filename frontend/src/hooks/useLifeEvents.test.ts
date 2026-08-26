import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { type Contact, getContactsByUid } from '../api/contacts';
import {
  createLifeEvent,
  deleteLifeEvent,
  getLifeEvents,
  type LifeEvent,
  type LifeEventInputData,
  updateLifeEvent,
} from '../api/lifeEvents';
import { useLifeEvents } from './useLifeEvents';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock('../api/lifeEvents', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/lifeEvents')>();
  return {
    ...actual,
    getLifeEvents: vi.fn(),
    createLifeEvent: vi.fn(),
    updateLifeEvent: vi.fn(),
    deleteLifeEvent: vi.fn(),
  };
});

vi.mock('../api/contacts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/contacts')>();
  return { ...actual, getContactsByUid: vi.fn() };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(getLifeEvents).mockReset();
  vi.mocked(createLifeEvent).mockReset();
  vi.mocked(updateLifeEvent).mockReset();
  vi.mocked(deleteLifeEvent).mockReset();
  vi.mocked(getContactsByUid).mockReset();
});

const relatedContact: Contact = { ID: 2, firstname: 'Bob', lastname: 'Burns' };

function lifeEvent(id: string, overrides: Partial<LifeEvent> = {}): LifeEvent {
  return {
    id,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    entity_id: 'uid-1',
    type: 'moved',
    related_entity_ids: ['uid-2'],
    ...overrides,
  };
}

test('loads life events and the total on mount', async () => {
  vi.mocked(getLifeEvents).mockResolvedValue({
    life_events: [lifeEvent('le-1')],
    next_cursor: '',
    limit: 50,
  });
  vi.mocked(getContactsByUid).mockResolvedValue(new Map([['uid-2', relatedContact]]));

  const { result } = renderHook(() => useLifeEvents('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getLifeEvents).toHaveBeenCalledWith({ entity_id: 'uid-1', limit: 50 });
  expect(result.current.events).toHaveLength(1);
  expect(result.current.total).toBe(1);
  expect(result.current.error).toBeNull();
});

test('resolves related contacts, excluding the entity itself', async () => {
  vi.mocked(getLifeEvents).mockResolvedValue({
    life_events: [lifeEvent('le-1', { related_entity_ids: ['uid-1', 'uid-2', 'uid-2'] })],
    next_cursor: '',
    limit: 50,
  });
  vi.mocked(getContactsByUid).mockResolvedValue(new Map([['uid-2', relatedContact]]));

  const { result } = renderHook(() => useLifeEvents('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getContactsByUid).toHaveBeenCalledWith(['uid-2']);
  expect(result.current.contactsByUid.get('uid-2')).toEqual(relatedContact);
});

test('skips the contact lookup when nothing is related', async () => {
  vi.mocked(getLifeEvents).mockResolvedValue({
    life_events: [lifeEvent('le-1', { related_entity_ids: [] })],
    next_cursor: '',
    limit: 50,
  });

  const { result } = renderHook(() => useLifeEvents('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getContactsByUid).not.toHaveBeenCalled();
  expect(result.current.contactsByUid.size).toBe(0);
});

test('does not fetch without an entity id', async () => {
  const { result } = renderHook(() => useLifeEvents(undefined));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getLifeEvents).not.toHaveBeenCalled();
  expect(result.current.events).toEqual([]);
});

test('handleCreate creates the event and refreshes', async () => {
  vi.mocked(getLifeEvents).mockResolvedValue({ life_events: [], next_cursor: '', limit: 50 });
  vi.mocked(createLifeEvent).mockResolvedValue({
    message: 'created',
    life_event: lifeEvent('le-9'),
  });

  const { result } = renderHook(() => useLifeEvents('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  const data: LifeEventInputData = { entity_id: 'uid-1', type: 'married' };
  await act(async () => {
    await result.current.handleCreate(data);
  });

  expect(createLifeEvent).toHaveBeenCalledWith(data);
  expect(getLifeEvents).toHaveBeenCalledTimes(2);
});

test('handleUpdate updates the event and refreshes', async () => {
  vi.mocked(getLifeEvents).mockResolvedValue({
    life_events: [lifeEvent('le-1')],
    next_cursor: '',
    limit: 50,
  });

  const { result } = renderHook(() => useLifeEvents('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  const data: LifeEventInputData = {
    entity_id: 'uid-1',
    type: 'moved',
    description: 'to a new city',
  };
  await act(async () => {
    await result.current.handleUpdate('le-1', data);
  });

  expect(updateLifeEvent).toHaveBeenCalledWith('le-1', data);
  expect(getLifeEvents).toHaveBeenCalledTimes(2);
});

test('handleDelete deletes the event and refreshes', async () => {
  vi.mocked(getLifeEvents).mockResolvedValue({
    life_events: [lifeEvent('le-1')],
    next_cursor: '',
    limit: 50,
  });

  const { result } = renderHook(() => useLifeEvents('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.handleDelete('le-1');
  });

  expect(deleteLifeEvent).toHaveBeenCalledWith('le-1');
  expect(getLifeEvents).toHaveBeenCalledTimes(2);
});

test('sets error when the fetch fails', async () => {
  vi.mocked(getLifeEvents).mockRejectedValue(new Error('boom'));

  const { result } = renderHook(() => useLifeEvents('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.error).toBe('boom');
  expect(result.current.events).toEqual([]);
});
