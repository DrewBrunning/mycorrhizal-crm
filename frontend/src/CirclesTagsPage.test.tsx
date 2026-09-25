import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import './i18n/config';
import {
  type Circle,
  type CircleListResponse,
  createCircle,
  deleteCircle,
  listCircles,
  updateCircle,
} from './api/circles';
import {
  createTag,
  deleteTag,
  listTags,
  type Tag,
  type TagListResponse,
  updateTag,
} from './api/tags';
import CirclesTagsPage from './CirclesTagsPage';
import { SnackbarProvider } from './context/SnackbarContext';

// This codebase's vitest setup has no auto-cleanup and no globals: true.
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

vi.mock('./api/circles', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/circles')>();
  return {
    ...actual,
    listCircles: vi.fn(),
    createCircle: vi.fn(),
    updateCircle: vi.fn(),
    deleteCircle: vi.fn(),
  };
});

vi.mock('./api/tags', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/tags')>();
  return {
    ...actual,
    listTags: vi.fn(),
    createTag: vi.fn(),
    updateTag: vi.fn(),
    deleteTag: vi.fn(),
  };
});

const listCirclesMock = vi.mocked(listCircles);
const createCircleMock = vi.mocked(createCircle);
const updateCircleMock = vi.mocked(updateCircle);
const deleteCircleMock = vi.mocked(deleteCircle);
const listTagsMock = vi.mocked(listTags);
const createTagMock = vi.mocked(createTag);
const updateTagMock = vi.mocked(updateTag);
const deleteTagMock = vi.mocked(deleteTag);

function circle(overrides: Partial<Circle> = {}): Circle {
  return {
    id: 'c1',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    name: 'Book Club',
    ...overrides,
  };
}

function tag(overrides: Partial<Tag> = {}): Tag {
  return {
    id: 't1',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    name: 'VIP',
    ...overrides,
  };
}

function circlesResponse(overrides: Partial<CircleListResponse> = {}): CircleListResponse {
  return {
    circles: [circle()],
    total: 1,
    next_cursor: '',
    limit: 200,
    members: [{ id: 1, circle_id: 'c1', member_vcard_uid: 'alice-uid' }],
    ...overrides,
  };
}

function tagsResponse(overrides: Partial<TagListResponse> = {}): TagListResponse {
  return {
    tags: [tag()],
    total: 1,
    next_cursor: '',
    limit: 200,
    contacts: [{ id: 1, tag_id: 't1', contact_vcard_uid: 'alice-uid' }],
    ...overrides,
  };
}

beforeEach(() => {
  listTagsMock.mockResolvedValue(tagsResponse({ tags: [], contacts: [] }));
});

function renderPage(initialEntries: string[] = ['/circles-tags']) {
  return render(
    <MemoryRouter initialEntries={initialEntries}>
      <SnackbarProvider>
        <CirclesTagsPage />
      </SnackbarProvider>
    </MemoryRouter>,
  );
}

test('loads and renders circles with their member counts on the default tab', async () => {
  listCirclesMock.mockResolvedValue(circlesResponse());

  renderPage();

  expect(await screen.findByText('Book Club')).toBeInTheDocument();
  expect(screen.getByText('1 contacts')).toBeInTheDocument();
  expect(listCirclesMock).toHaveBeenCalledWith({ limit: 200, include_members: true });
});

test('renders the empty state when there are no circles', async () => {
  listCirclesMock.mockResolvedValue(circlesResponse({ circles: [], members: [] }));

  renderPage();

  expect(
    await screen.findByText("No circles yet. Add one below, or from a contact's page."),
  ).toBeInTheDocument();
});

test('a circles fetch failure surfaces an error alert', async () => {
  listCirclesMock.mockRejectedValue(new Error('circles down'));

  renderPage();

  expect(await screen.findByText('circles down')).toBeInTheDocument();
});

test('the tags tab loads and renders tags with member counts, driven by the ?tab= query param', async () => {
  listCirclesMock.mockResolvedValue(circlesResponse({ circles: [], members: [] }));
  listTagsMock.mockResolvedValue(tagsResponse());

  renderPage(['/circles-tags?tab=tags']);

  expect(await screen.findByText('VIP')).toBeInTheDocument();
  expect(screen.getByText('1 contacts')).toBeInTheDocument();
  expect(listTagsMock).toHaveBeenCalledWith({ limit: 200, include_contacts: true });
});

test('a tags fetch failure surfaces an error alert on the tags tab', async () => {
  listCirclesMock.mockResolvedValue(circlesResponse({ circles: [], members: [] }));
  listTagsMock.mockRejectedValue(new Error('tags down'));

  renderPage(['/circles-tags?tab=tags']);

  expect(await screen.findByText('tags down')).toBeInTheDocument();
});

test('clicking the Tags tab switches the view without reloading circles data again', async () => {
  listCirclesMock.mockResolvedValue(circlesResponse());
  listTagsMock.mockResolvedValue(tagsResponse());

  renderPage();
  await screen.findByText('Book Club');
  expect(listCirclesMock).toHaveBeenCalledTimes(1);

  fireEvent.click(screen.getByRole('tab', { name: 'Tags' }));

  expect(await screen.findByText('VIP')).toBeInTheDocument();
  expect(screen.queryByText('Book Club')).toBeNull();
  // Switching tabs is a pure client-side render toggle -- the circles data
  // that's already in memory should not be re-fetched.
  expect(listCirclesMock).toHaveBeenCalledTimes(1);

  fireEvent.click(screen.getByRole('tab', { name: 'Circles' }));
  expect(await screen.findByText('Book Club')).toBeInTheDocument();
});

test('creating a circle calls createCircle with the trimmed name, refreshes, and shows success', async () => {
  listCirclesMock.mockResolvedValueOnce(circlesResponse({ circles: [], members: [] }));
  createCircleMock.mockResolvedValue({
    message: 'ok',
    circle: circle({ id: 'c2', name: 'New Circle' }),
  });
  listCirclesMock.mockResolvedValueOnce(
    circlesResponse({ circles: [circle({ id: 'c2', name: 'New Circle' })] }),
  );

  renderPage();
  await screen.findByText("No circles yet. Add one below, or from a contact's page.");

  fireEvent.change(screen.getByPlaceholderText('New circle name…'), {
    target: { value: '  New Circle  ' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Add' }));

  await waitFor(() => expect(createCircleMock).toHaveBeenCalledWith('New Circle'));
  await waitFor(() => expect(listCirclesMock).toHaveBeenCalledTimes(2));
  expect(await screen.findByText('Circle created')).toBeInTheDocument();
});

test('renaming a circle calls updateCircle with the id and new name', async () => {
  listCirclesMock.mockResolvedValue(circlesResponse());
  updateCircleMock.mockResolvedValue(circle({ name: 'Renamed Club' }));

  renderPage();
  await screen.findByText('Book Club');

  fireEvent.click(screen.getByLabelText('Edit'));
  const editInput = screen.getByDisplayValue('Book Club');
  fireEvent.change(editInput, { target: { value: 'Renamed Club' } });
  fireEvent.click(screen.getByLabelText('Save'));

  await waitFor(() => expect(updateCircleMock).toHaveBeenCalledWith('c1', 'Renamed Club'));
  expect(await screen.findByText('Circle renamed')).toBeInTheDocument();
});

test('canceling a rename discards the edit without calling updateCircle', async () => {
  listCirclesMock.mockResolvedValue(circlesResponse());

  renderPage();
  await screen.findByText('Book Club');

  fireEvent.click(screen.getByLabelText('Edit'));
  fireEvent.change(screen.getByDisplayValue('Book Club'), { target: { value: 'Discarded' } });
  fireEvent.click(screen.getByLabelText('Cancel'));

  expect(updateCircleMock).not.toHaveBeenCalled();
  expect(await screen.findByText('Book Club')).toBeInTheDocument();
});

test('deleting a circle prompts for confirmation naming the circle and calls deleteCircle', async () => {
  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
  listCirclesMock.mockResolvedValue(circlesResponse());
  deleteCircleMock.mockResolvedValue(undefined);

  renderPage();
  await screen.findByText('Book Club');

  fireEvent.click(screen.getByLabelText('Delete'));

  expect(confirmSpy).toHaveBeenCalledWith(
    'Delete the circle "Book Club"? It will be removed from every contact that has it.',
  );
  await waitFor(() => expect(deleteCircleMock).toHaveBeenCalledWith('c1'));
  expect(await screen.findByText('Circle deleted')).toBeInTheDocument();

  confirmSpy.mockRestore();
});

test('declining the delete confirmation never calls deleteCircle', async () => {
  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false);
  listCirclesMock.mockResolvedValue(circlesResponse());

  renderPage();
  await screen.findByText('Book Club');

  fireEvent.click(screen.getByLabelText('Delete'));

  expect(deleteCircleMock).not.toHaveBeenCalled();
  confirmSpy.mockRestore();
});

test('creating a tag calls createTag with the trimmed name, refreshes, and shows success', async () => {
  listCirclesMock.mockResolvedValue(circlesResponse({ circles: [], members: [] }));
  listTagsMock.mockResolvedValueOnce(tagsResponse({ tags: [], contacts: [] }));
  createTagMock.mockResolvedValue({ message: 'ok', tag: tag({ id: 't2', name: 'New Tag' }) });
  listTagsMock.mockResolvedValueOnce(tagsResponse({ tags: [tag({ id: 't2', name: 'New Tag' })] }));

  renderPage(['/circles-tags?tab=tags']);
  await screen.findByText("No tags yet. Add one below, or from a contact's page.");

  fireEvent.change(screen.getByPlaceholderText('New tag name…'), { target: { value: 'New Tag' } });
  fireEvent.click(screen.getByRole('button', { name: 'Add' }));

  await waitFor(() => expect(createTagMock).toHaveBeenCalledWith('New Tag'));
  expect(await screen.findByText('Tag created')).toBeInTheDocument();
});

test('the tag Add button is disabled for a blank/whitespace-only name', async () => {
  listCirclesMock.mockResolvedValue(circlesResponse({ circles: [], members: [] }));
  listTagsMock.mockResolvedValue(tagsResponse({ tags: [], contacts: [] }));

  renderPage(['/circles-tags?tab=tags']);
  await screen.findByText("No tags yet. Add one below, or from a contact's page.");

  const addButton = screen.getByRole('button', { name: 'Add' });
  expect(addButton).toBeDisabled();

  fireEvent.change(screen.getByPlaceholderText('New tag name…'), { target: { value: '   ' } });
  expect(addButton).toBeDisabled();
  expect(createTagMock).not.toHaveBeenCalled();
});

test('renaming a tag calls updateTag with the id and new name', async () => {
  listCirclesMock.mockResolvedValue(circlesResponse({ circles: [], members: [] }));
  listTagsMock.mockResolvedValue(tagsResponse());
  updateTagMock.mockResolvedValue(tag({ name: 'Renamed Tag' }));

  renderPage(['/circles-tags?tab=tags']);
  await screen.findByText('VIP');

  fireEvent.click(screen.getByLabelText('Edit'));
  fireEvent.change(screen.getByDisplayValue('VIP'), { target: { value: 'Renamed Tag' } });
  fireEvent.click(screen.getByLabelText('Save'));

  await waitFor(() => expect(updateTagMock).toHaveBeenCalledWith('t1', 'Renamed Tag'));
  expect(await screen.findByText('Tag renamed')).toBeInTheDocument();
});

test('deleting a tag prompts for confirmation naming the tag and calls deleteTag', async () => {
  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
  listCirclesMock.mockResolvedValue(circlesResponse({ circles: [], members: [] }));
  listTagsMock.mockResolvedValue(tagsResponse());
  deleteTagMock.mockResolvedValue(undefined);

  renderPage(['/circles-tags?tab=tags']);
  await screen.findByText('VIP');

  fireEvent.click(screen.getByLabelText('Delete'));

  expect(confirmSpy).toHaveBeenCalledWith(
    'Delete the tag "VIP"? It will be removed from every contact that has it.',
  );
  await waitFor(() => expect(deleteTagMock).toHaveBeenCalledWith('t1'));
  expect(await screen.findByText('Tag deleted')).toBeInTheDocument();

  confirmSpy.mockRestore();
});
