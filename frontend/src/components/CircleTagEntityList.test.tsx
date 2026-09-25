import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import CircleTagEntityList, { type CircleTagEntityListItem } from './CircleTagEntityList';

// This codebase's vitest setup has no auto-cleanup and no globals: true.
afterEach(cleanup);

const items: CircleTagEntityListItem[] = [
  { id: '1', name: 'Close Friends', memberCount: 3 },
  { id: '2', name: 'Book Club', memberCount: 0 },
];

function renderList(props: Partial<React.ComponentProps<typeof CircleTagEntityList>> = {}) {
  const defaults: React.ComponentProps<typeof CircleTagEntityList> = {
    items,
    loading: false,
    newPlaceholder: 'New circle name',
    addLabel: 'Add Circle',
    emptyLabel: 'No circles yet.',
    memberCountLabel: (count) => `${count} member${count === 1 ? '' : 's'}`,
    deleteConfirmLabel: (name) => `Delete ${name}?`,
    onCreate: vi.fn().mockResolvedValue(undefined),
    onRename: vi.fn().mockResolvedValue(undefined),
    onDelete: vi.fn().mockResolvedValue(undefined),
    ...props,
  };
  return render(<CircleTagEntityList {...defaults} />);
}

afterEach(() => {
  vi.restoreAllMocks();
});

test('renders each item with its name and member count', () => {
  renderList();

  expect(screen.getByText('Close Friends')).toBeInTheDocument();
  expect(screen.getByText('3 members')).toBeInTheDocument();
  expect(screen.getByText('Book Club')).toBeInTheDocument();
  expect(screen.getByText('0 members')).toBeInTheDocument();
});

test('shows the empty label only when there are no items and loading has finished', () => {
  renderList({ items: [] });
  expect(screen.getByText('No circles yet.')).toBeInTheDocument();
});

test('while loading with no items yet, the empty label is not shown', () => {
  renderList({ items: [], loading: true });
  expect(screen.queryByText('No circles yet.')).not.toBeInTheDocument();
});

test('the add button is disabled until a non-blank name is entered', () => {
  renderList();

  const addButton = screen.getByRole('button', { name: 'Add Circle' });
  expect(addButton).toBeDisabled();

  fireEvent.change(screen.getByPlaceholderText('New circle name'), {
    target: { value: '   ' },
  });
  expect(addButton).toBeDisabled();

  fireEvent.change(screen.getByPlaceholderText('New circle name'), {
    target: { value: 'Neighbors' },
  });
  expect(addButton).not.toBeDisabled();
});

test('clicking Add creates the trimmed name and clears the input', async () => {
  const onCreate = vi.fn().mockResolvedValue(undefined);
  renderList({ onCreate });

  fireEvent.change(screen.getByPlaceholderText('New circle name'), {
    target: { value: '  Neighbors  ' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Add Circle' }));

  await waitFor(() => expect(onCreate).toHaveBeenCalledWith('Neighbors'));
  await waitFor(() =>
    expect((screen.getByPlaceholderText('New circle name') as HTMLInputElement).value).toBe(''),
  );
});

test('pressing Enter in the new-name field also creates the entity', async () => {
  const onCreate = vi.fn().mockResolvedValue(undefined);
  renderList({ onCreate });

  const input = screen.getByPlaceholderText('New circle name');
  fireEvent.change(input, { target: { value: 'Neighbors' } });
  fireEvent.keyDown(input, { key: 'Enter' });

  await waitFor(() => expect(onCreate).toHaveBeenCalledWith('Neighbors'));
});

test('clicking edit switches the row into an editable text field prefilled with the name', () => {
  renderList();

  fireEvent.click(screen.getAllByLabelText('Edit')[0]);

  const editInput = screen.getByDisplayValue('Close Friends');
  expect(editInput).toBeInTheDocument();
});

test('saving an edit renames via onRename with the trimmed value', async () => {
  const onRename = vi.fn().mockResolvedValue(undefined);
  renderList({ onRename });

  fireEvent.click(screen.getAllByLabelText('Edit')[0]);
  const editInput = screen.getByDisplayValue('Close Friends');
  fireEvent.change(editInput, { target: { value: '  Best Friends  ' } });
  fireEvent.click(screen.getByLabelText('Save'));

  await waitFor(() => expect(onRename).toHaveBeenCalledWith('1', 'Best Friends'));
  await waitFor(() => expect(screen.queryByDisplayValue('Best Friends')).not.toBeInTheDocument());
});

test('pressing Escape while editing cancels without calling onRename', () => {
  const onRename = vi.fn();
  renderList({ onRename });

  fireEvent.click(screen.getAllByLabelText('Edit')[0]);
  const editInput = screen.getByDisplayValue('Close Friends');
  fireEvent.change(editInput, { target: { value: 'Changed' } });
  fireEvent.keyDown(editInput, { key: 'Escape' });

  expect(onRename).not.toHaveBeenCalled();
  expect(screen.getByText('Close Friends')).toBeInTheDocument();
});

test('pressing Enter while editing saves via onRename', async () => {
  const onRename = vi.fn().mockResolvedValue(undefined);
  renderList({ onRename });

  fireEvent.click(screen.getAllByLabelText('Edit')[0]);
  const editInput = screen.getByDisplayValue('Close Friends');
  fireEvent.change(editInput, { target: { value: 'Best Friends' } });
  fireEvent.keyDown(editInput, { key: 'Enter' });

  await waitFor(() => expect(onRename).toHaveBeenCalledWith('1', 'Best Friends'));
});

test('clicking cancel while editing discards the change', () => {
  const onRename = vi.fn();
  renderList({ onRename });

  fireEvent.click(screen.getAllByLabelText('Edit')[0]);
  fireEvent.change(screen.getByDisplayValue('Close Friends'), { target: { value: 'Changed' } });
  fireEvent.click(screen.getByLabelText('Cancel'));

  expect(onRename).not.toHaveBeenCalled();
  expect(screen.getByText('Close Friends')).toBeInTheDocument();
});

test('deleting requires window.confirm and calls onDelete with the id when confirmed', async () => {
  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
  const onDelete = vi.fn().mockResolvedValue(undefined);
  renderList({ onDelete });

  fireEvent.click(screen.getAllByLabelText('Delete')[0]);

  expect(confirmSpy).toHaveBeenCalledWith('Delete Close Friends?');
  await waitFor(() => expect(onDelete).toHaveBeenCalledWith('1'));
});

test('declining the confirm dialog does not call onDelete', () => {
  vi.spyOn(window, 'confirm').mockReturnValue(false);
  const onDelete = vi.fn();
  renderList({ onDelete });

  fireEvent.click(screen.getAllByLabelText('Delete')[0]);

  expect(onDelete).not.toHaveBeenCalled();
});
