import { test, expect, vi, afterEach, beforeEach } from 'vitest';
import { renderHook, cleanup, waitFor, act } from '@testing-library/react';
import { useCircles } from './useCircles';
import { listCircles, createCircle, updateCircle, deleteCircle, Circle, CircleMember } from '../api/circles';
import { ApiError } from '../api/client';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock('../api/circles', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/circles')>();
  return {
    ...actual,
    listCircles: vi.fn(),
    createCircle: vi.fn(),
    updateCircle: vi.fn(),
    deleteCircle: vi.fn(),
  };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(listCircles).mockReset();
  vi.mocked(createCircle).mockReset();
  vi.mocked(updateCircle).mockReset();
  vi.mocked(deleteCircle).mockReset();
});

function circle(id: string, name: string): Circle {
  return {
    id,
    name,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  };
}

function member(circleId: string, uid: string): CircleMember {
  return { id: 1, circle_id: circleId, member_vcard_uid: uid };
}

function emptyListResponse() {
  return { circles: [], total: 0, next_cursor: '', limit: 200 };
}

test('loads circles and members on mount', async () => {
  vi.mocked(listCircles).mockResolvedValue({
    circles: [circle('c-1', 'Family')],
    members: [member('c-1', 'uid-1')],
    total: 1,
    next_cursor: '',
    limit: 200,
  });

  const { result } = renderHook(() => useCircles());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(listCircles).toHaveBeenCalledWith({ limit: 200, include_members: true });
  expect(result.current.circles).toHaveLength(1);
  expect(result.current.members).toHaveLength(1);
  expect(result.current.error).toBeNull();
});

test('refresh replaces circles and members', async () => {
  vi.mocked(listCircles).mockResolvedValue(emptyListResponse());
  const { result } = renderHook(() => useCircles());
  await waitFor(() => expect(result.current.loading).toBe(false));

  vi.mocked(listCircles).mockResolvedValue({
    circles: [circle('c-2', 'Work')],
    members: [member('c-2', 'uid-9')],
    total: 1,
    next_cursor: '',
    limit: 200,
  });

  await act(async () => {
    await result.current.refresh();
  });

  expect(result.current.circles.map((c) => c.id)).toEqual(['c-2']);
  expect(result.current.members).toHaveLength(1);
});

test('sets error when the fetch fails', async () => {
  vi.mocked(listCircles).mockRejectedValue(new Error('boom'));

  const { result } = renderHook(() => useCircles());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.error).toBe('boom');
  expect(result.current.circles).toEqual([]);
});

test('circleNamesByUid maps a contact uid to its circle names', async () => {
  vi.mocked(listCircles).mockResolvedValue({
    circles: [circle('c-1', 'Family'), circle('c-2', 'Friends')],
    members: [member('c-1', 'uid-1'), member('c-2', 'uid-1'), member('c-1', 'uid-2')],
    total: 2,
    next_cursor: '',
    limit: 200,
  });

  const { result } = renderHook(() => useCircles());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.circleNamesByUid.get('uid-1')).toEqual(['Family', 'Friends']);
  expect(result.current.circleNamesByUid.get('uid-2')).toEqual(['Family']);
  expect(result.current.circleNamesByUid.get('uid-3')).toBeUndefined();
});

test('skips members whose circle row is missing', async () => {
  vi.mocked(listCircles).mockResolvedValue({
    circles: [circle('c-1', 'Family')],
    members: [member('c-1', 'uid-1'), member('c-ghost', 'uid-2')],
    total: 1,
    next_cursor: '',
    limit: 200,
  });

  const { result } = renderHook(() => useCircles());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.circleNamesByUid.get('uid-1')).toEqual(['Family']);
  expect(result.current.circleNamesByUid.get('uid-2')).toBeUndefined();
});

test('handleCreate creates a circle and returns it', async () => {
  vi.mocked(listCircles).mockResolvedValue(emptyListResponse());
  vi.mocked(createCircle).mockResolvedValue({ message: 'created', circle: circle('c-new', 'New') });

  const { result } = renderHook(() => useCircles());
  await waitFor(() => expect(result.current.loading).toBe(false));

  let created;
  await act(async () => {
    created = await result.current.handleCreate('New');
  });

  expect(createCircle).toHaveBeenCalledWith('New');
  expect(created).toEqual(circle('c-new', 'New'));
});

test('handleCreate surfaces a duplicate (409) error to the notifier', async () => {
  vi.mocked(listCircles).mockResolvedValue(emptyListResponse());
  const duplicate = new ApiError('Circle already exists', 'ALREADY_EXISTS', 409);
  vi.mocked(createCircle).mockRejectedValue(duplicate);
  const notifier = { showError: vi.fn() };

  const { result } = renderHook(() => useCircles(notifier));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await expect(
    act(async () => {
      await result.current.handleCreate('Duplicate');
    })
  ).rejects.toThrow('Circle already exists');

  expect(notifier.showError).toHaveBeenCalledWith('Circle already exists');
});

test('handleUpdate renames the circle and refreshes the list', async () => {
  vi.mocked(listCircles)
    .mockResolvedValueOnce({ circles: [circle('c-1', 'Old')], total: 1, next_cursor: '', limit: 200 })
    .mockResolvedValueOnce({ circles: [circle('c-1', 'Renamed')], total: 1, next_cursor: '', limit: 200 });
  vi.mocked(updateCircle).mockResolvedValue(circle('c-1', 'Renamed'));

  const { result } = renderHook(() => useCircles());
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.circles[0].name).toBe('Old');

  await act(async () => {
    await result.current.handleUpdate('c-1', 'Renamed');
  });

  expect(updateCircle).toHaveBeenCalledWith('c-1', 'Renamed');
  expect(listCircles).toHaveBeenCalledTimes(2);
  expect(result.current.circles[0].name).toBe('Renamed');
});

test('handleDelete removes the circle and refreshes the list', async () => {
  vi.mocked(listCircles)
    .mockResolvedValueOnce({ circles: [circle('c-1', 'Gone'), circle('c-2', 'Stays')], total: 2, next_cursor: '', limit: 200 })
    .mockResolvedValueOnce({ circles: [circle('c-2', 'Stays')], total: 1, next_cursor: '', limit: 200 });
  vi.mocked(deleteCircle).mockResolvedValue(undefined);

  const { result } = renderHook(() => useCircles());
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.circles).toHaveLength(2);

  await act(async () => {
    await result.current.handleDelete('c-1');
  });

  expect(deleteCircle).toHaveBeenCalledWith('c-1');
  expect(listCircles).toHaveBeenCalledTimes(2);
  expect(result.current.circles.map((c) => c.id)).toEqual(['c-2']);
});

test('handleDelete rethrows and notifies on failure', async () => {
  vi.mocked(listCircles).mockResolvedValue(emptyListResponse());
  vi.mocked(deleteCircle).mockRejectedValue(new Error('delete failed'));
  const notifier = { showError: vi.fn() };

  const { result } = renderHook(() => useCircles(notifier));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await expect(
    act(async () => {
      await result.current.handleDelete('c-1');
    })
  ).rejects.toThrow('delete failed');

  expect(notifier.showError).toHaveBeenCalledWith('delete failed');
});
