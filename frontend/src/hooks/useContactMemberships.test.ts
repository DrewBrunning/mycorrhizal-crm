import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import {
  addCircleMember,
  type Circle,
  createCircle,
  listCircles,
  removeCircleMember,
} from '../api/circles';
import { addContactTag, createTag, listTags, removeContactTag, type Tag } from '../api/tags';
import { useContactMemberships } from './useContactMemberships';

vi.mock('../api/circles', () => ({
  listCircles: vi.fn(),
  createCircle: vi.fn(),
  updateCircle: vi.fn(),
  deleteCircle: vi.fn(),
  addCircleMember: vi.fn(),
  removeCircleMember: vi.fn(),
}));
vi.mock('../api/tags', () => ({
  listTags: vi.fn(),
  createTag: vi.fn(),
  updateTag: vi.fn(),
  deleteTag: vi.fn(),
  addContactTag: vi.fn(),
  removeContactTag: vi.fn(),
}));

const circle = (id: string, name: string): Circle => ({
  id,
  name,
  created_at: '',
  updated_at: '',
});
const tag = (id: string, name: string): Tag => ({ id, name, created_at: '', updated_at: '' });

const friends = circle('c-1', 'Friends');
const work = circle('c-2', 'Work');
const vip = tag('t-1', 'vip');
const golf = tag('t-2', 'golf');

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(listCircles).mockResolvedValue({
    circles: [friends, work],
    members: [
      { id: 1, circle_id: 'c-1', member_vcard_uid: 'alice-uid' },
      { id: 2, circle_id: 'c-2', member_vcard_uid: 'bob-uid' },
    ],
    total: 2,
    next_cursor: '',
    limit: 200,
  });
  vi.mocked(listTags).mockResolvedValue({
    tags: [vip, golf],
    contacts: [{ id: 1, tag_id: 't-2', contact_vcard_uid: 'alice-uid' }],
    total: 2,
    next_cursor: '',
    limit: 200,
  });
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

async function renderLoaded(...args: [] | [undefined]) {
  const uid = args.length === 0 ? 'alice-uid' : undefined;
  const notifier = { showError: vi.fn() };
  const hook = renderHook(() => useContactMemberships(uid, notifier));
  await waitFor(() => expect(hook.result.current.allCircles).toHaveLength(2));
  await waitFor(() => expect(hook.result.current.allTags).toHaveLength(2));
  return { ...hook, notifier };
}

test('derives this contact’s circles and tags from the user-wide lists', async () => {
  const { result } = await renderLoaded();
  expect(result.current.contactCircles).toEqual([friends]);
  expect(result.current.contactTags).toEqual([golf]);
});

test('a contact with no uid (still loading) has no memberships and every handler is a no-op', async () => {
  const { result } = await renderLoaded(undefined);
  expect(result.current.contactCircles).toEqual([]);
  expect(result.current.contactTags).toEqual([]);
  await act(async () => {
    await result.current.handleCircleAdd(work);
    await result.current.handleCircleRemove(friends);
    await result.current.handleTagAdd(vip);
    await result.current.handleTagRemove(golf);
  });
  expect(addCircleMember).not.toHaveBeenCalled();
  expect(removeCircleMember).not.toHaveBeenCalled();
  expect(addContactTag).not.toHaveBeenCalled();
  expect(removeContactTag).not.toHaveBeenCalled();
});

describe('circles', () => {
  test('adding an existing circle adds the membership and refreshes', async () => {
    const { result } = await renderLoaded();
    vi.mocked(listCircles).mockClear();
    await act(() => result.current.handleCircleAdd(work));
    expect(addCircleMember).toHaveBeenCalledWith('c-2', 'alice-uid');
    expect(listCircles).toHaveBeenCalledTimes(1);
  });

  test('adding an unsaved circle creates it first', async () => {
    vi.mocked(createCircle).mockResolvedValue({ message: '', circle: circle('c-9', 'New') });
    const { result } = await renderLoaded();
    await act(() => result.current.handleCircleAdd(circle('', 'New')));
    expect(createCircle).toHaveBeenCalledWith('New');
    expect(addCircleMember).toHaveBeenCalledWith('c-9', 'alice-uid');
  });

  test('a create that returns no id skips the membership call', async () => {
    vi.mocked(createCircle).mockResolvedValue({ message: '', circle: undefined as never });
    const { result } = await renderLoaded();
    await act(() => result.current.handleCircleAdd(circle('', 'New')));
    expect(addCircleMember).not.toHaveBeenCalled();
  });

  test('a failed create is reported by useCircles and still refreshes', async () => {
    vi.mocked(createCircle).mockRejectedValue(new Error('dup'));
    const { result, notifier } = await renderLoaded();
    vi.mocked(listCircles).mockClear();
    await act(() => result.current.handleCircleAdd(circle('', 'New')));
    expect(notifier.showError).toHaveBeenCalled();
    expect(listCircles).toHaveBeenCalledTimes(1);
  });

  test('removing refreshes on success and on failure', async () => {
    const { result } = await renderLoaded();
    vi.mocked(listCircles).mockClear();
    await act(() => result.current.handleCircleRemove(friends));
    expect(removeCircleMember).toHaveBeenCalledWith('c-1', 'alice-uid');
    vi.mocked(removeCircleMember).mockRejectedValue(new Error('x'));
    await act(() => result.current.handleCircleRemove(friends));
    expect(listCircles).toHaveBeenCalledTimes(2);
  });
});

describe('tags', () => {
  test('adding an existing tag adds it and refreshes', async () => {
    const { result } = await renderLoaded();
    vi.mocked(listTags).mockClear();
    await act(() => result.current.handleTagAdd(vip));
    expect(addContactTag).toHaveBeenCalledWith('t-1', 'alice-uid');
    expect(listTags).toHaveBeenCalledTimes(1);
  });

  test('adding an unsaved tag creates it first; a create with no id skips the add', async () => {
    vi.mocked(createTag).mockResolvedValueOnce({ message: '', tag: tag('t-9', 'new') });
    const { result } = await renderLoaded();
    await act(() => result.current.handleTagAdd(tag('', 'new')));
    expect(createTag).toHaveBeenCalledWith('new');
    expect(addContactTag).toHaveBeenCalledWith('t-9', 'alice-uid');

    vi.mocked(addContactTag).mockClear();
    vi.mocked(createTag).mockResolvedValueOnce({ message: '', tag: undefined as never });
    await act(() => result.current.handleTagAdd(tag('', 'other')));
    expect(addContactTag).not.toHaveBeenCalled();
  });

  test('a failed add still refreshes', async () => {
    vi.mocked(addContactTag).mockRejectedValue(new Error('x'));
    const { result } = await renderLoaded();
    vi.mocked(listTags).mockClear();
    await act(() => result.current.handleTagAdd(vip));
    expect(listTags).toHaveBeenCalledTimes(1);
  });

  test('removing refreshes on success and on failure', async () => {
    const { result } = await renderLoaded();
    vi.mocked(listTags).mockClear();
    await act(() => result.current.handleTagRemove(golf));
    expect(removeContactTag).toHaveBeenCalledWith('t-2', 'alice-uid');
    vi.mocked(removeContactTag).mockRejectedValue(new Error('x'));
    await act(() => result.current.handleTagRemove(golf));
    expect(listTags).toHaveBeenCalledTimes(2);
  });
});
