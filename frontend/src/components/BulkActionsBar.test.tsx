import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent } from '@testing-library/react';
import '../i18n/config';
import BulkActionsBar from './BulkActionsBar';
import { Circle } from '../api/circles';
import { Tag } from '../api/tags';

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
  expect(screen.queryByText('Archive')).not.toBeInTheDocument();

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
