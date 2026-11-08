import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { Circle } from '../api/circles';
import type { Tag } from '../api/tags';
import BulkActionsBar from './BulkActionsBar';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(cleanup);

const circles: Circle[] = [{ id: 'c1', created_at: '', updated_at: '', name: 'Friends' }];
const tags: Tag[] = [{ id: 't1', created_at: '', updated_at: '', name: 'vip' }];

function renderBar(props: Partial<React.ComponentProps<typeof BulkActionsBar>> = {}) {
  const defaults: React.ComponentProps<typeof BulkActionsBar> = {
    selectedCount: 0,
    loadedCount: 2,
    allSelected: false,
    circles,
    tags,
    busy: false,
    onSelectAll: vi.fn(),
    onClear: vi.fn(),
    onAddCircle: vi.fn(),
    onRemoveCircle: vi.fn(),
    onAddTag: vi.fn(),
    onRemoveTag: vi.fn(),
    onArchive: vi.fn(),
    onUnarchive: vi.fn(),
    onEditFields: vi.fn(),
    onMerge: vi.fn(),
    onDelete: vi.fn(),
    ...props,
  };
  return render(<BulkActionsBar {...defaults} />);
}

test('renders nothing when there are no loaded contacts', () => {
  renderBar({ loadedCount: 0 });
  expect(screen.queryByText('Select all')).not.toBeInTheDocument();
});

test('select-all is always visible with contacts; actions appear once selected', () => {
  renderBar({ selectedCount: 0 });
  expect(screen.getByText('Select all')).toBeInTheDocument();
  expect(screen.queryByText('2 selected')).not.toBeInTheDocument();
  // #2365: the actions stay mounted (reserving their height) but are hidden
  // from sight and from assistive tech until something is selected.
  expect(screen.queryByRole('button', { name: 'Archive' })).not.toBeInTheDocument();

  cleanup();
  renderBar({ selectedCount: 2 });
  expect(screen.getByText('2 selected')).toBeInTheDocument();
  expect(screen.getByText('Archive')).toBeInTheDocument();
  expect(screen.getByText('Unarchive')).toBeInTheDocument();
  expect(screen.getByText('Delete')).toBeInTheDocument();
});

test('select-all and clear call their handlers', () => {
  const onSelectAll = vi.fn();
  const onClear = vi.fn();
  renderBar({ selectedCount: 2, onSelectAll, onClear });

  fireEvent.click(screen.getByLabelText('Select all'));
  expect(onSelectAll).toHaveBeenCalled();

  fireEvent.click(screen.getByText('Clear'));
  expect(onClear).toHaveBeenCalled();
});

test('add to circle reports the selected circle id', async () => {
  const onAddCircle = vi.fn();
  renderBar({ selectedCount: 2, onAddCircle });

  fireEvent.mouseDown(screen.getByLabelText('Circle…'));
  fireEvent.click(await screen.findByRole('option', { name: 'Friends' }));
  fireEvent.click(screen.getByText('Add to circle'));

  expect(onAddCircle).toHaveBeenCalledWith('c1');
});

test('remove from circle reports the selected circle id', async () => {
  const onRemoveCircle = vi.fn();
  renderBar({ selectedCount: 2, onRemoveCircle });

  fireEvent.mouseDown(screen.getByLabelText('Circle…'));
  fireEvent.click(await screen.findByRole('option', { name: 'Friends' }));
  fireEvent.click(screen.getByText('Remove from circle'));

  expect(onRemoveCircle).toHaveBeenCalledWith('c1');
});

test('add tag reports the selected tag id', async () => {
  const onAddTag = vi.fn();
  renderBar({ selectedCount: 2, onAddTag });

  fireEvent.mouseDown(screen.getByLabelText('Tag…'));
  fireEvent.click(await screen.findByRole('option', { name: 'vip' }));
  fireEvent.click(screen.getByText('Add tag'));

  expect(onAddTag).toHaveBeenCalledWith('t1');
});

test('archive, unarchive, and delete call their handlers', () => {
  const onArchive = vi.fn();
  const onUnarchive = vi.fn();
  const onDelete = vi.fn();
  renderBar({ selectedCount: 2, onArchive, onUnarchive, onDelete });

  fireEvent.click(screen.getByText('Archive'));
  expect(onArchive).toHaveBeenCalled();

  fireEvent.click(screen.getByText('Unarchive'));
  expect(onUnarchive).toHaveBeenCalled();

  fireEvent.click(screen.getByText('Delete'));
  expect(onDelete).toHaveBeenCalled();
});

// --- T92 / #2197 bulk merge --------------------------------------------------
// Merge works on two or more selected contacts (the contacts page folds
// them into one primary, pair by pair).

test('merge is disabled for a single selection and explains the constraint', async () => {
  renderBar({ selectedCount: 1 });
  const mergeButton = screen.getByRole('button', { name: 'Merge' });
  expect(mergeButton).toBeDisabled();

  // A disabled button emits no pointer events, so hover the tooltip's wrapper.
  fireEvent.mouseOver(mergeButton.parentElement as HTMLElement);
  expect(await screen.findByText('Select at least two contacts to merge.')).toBeInTheDocument();
});

test.each([2, 3, 7])('merge is enabled for %i selections and reports them', (count) => {
  const onMerge = vi.fn();
  renderBar({ selectedCount: count, onMerge });

  const mergeButton = screen.getByRole('button', { name: 'Merge' });
  expect(mergeButton).not.toBeDisabled();
  fireEvent.click(mergeButton);
  expect(onMerge).toHaveBeenCalledTimes(1);
});

test('the action row is mounted but hidden with no selection, so the bar height never changes (#2365)', () => {
  renderBar({ selectedCount: 0 });
  const archive = screen.getByText('Archive');
  expect(archive).toBeInTheDocument();
  expect(archive.closest('[data-testid="bulk-actions"]')).toHaveStyle({
    visibility: 'hidden',
  });
});

test('the action row is visible once something is selected', () => {
  renderBar({ selectedCount: 1 });
  expect(screen.getByTestId('bulk-actions')).toHaveStyle({
    visibility: 'visible',
  });
});

test('merge stays disabled while a bulk action is busy', () => {
  renderBar({ selectedCount: 2, busy: true });
  expect(screen.getByRole('button', { name: 'Merge' })).toBeDisabled();
});

test('Edit fields button opens the bulk field editor', () => {
  const onEditFields = vi.fn();
  renderBar({ selectedCount: 2, onEditFields });
  fireEvent.click(screen.getByRole('button', { name: 'Edit fields' }));
  expect(onEditFields).toHaveBeenCalledTimes(1);
});
