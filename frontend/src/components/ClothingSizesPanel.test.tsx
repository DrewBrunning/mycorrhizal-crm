import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent } from '@testing-library/react';
import '../i18n/config';
import ClothingSizesPanel from './ClothingSizesPanel';
import { Preference } from '../api/preferences';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(cleanup);

function size(overrides: Partial<Preference> = {}): Preference {
  return {
    id: 'size-1',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    entity_id: 'alice-uid',
    category: 'clothing_size',
    value: 'M',
    sensitivity: 'normal',
    ...overrides,
  };
}

function renderPanel(props: Partial<React.ComponentProps<typeof ClothingSizesPanel>> = {}) {
  const defaults: React.ComponentProps<typeof ClothingSizesPanel> = {
    sizes: [],
    onAdd: vi.fn().mockResolvedValue(undefined),
    onEdit: vi.fn().mockResolvedValue(undefined),
    onDelete: vi.fn().mockResolvedValue(undefined),
    ...props,
  };
  return render(<ClothingSizesPanel {...defaults} />);
}

test('shows the existing sizes', () => {
  renderPanel({ sizes: [size(), size({ id: 'size-2', value: 'S/M' })] });
  expect(screen.getByText('M')).toBeInTheDocument();
  expect(screen.getByText('S/M')).toBeInTheDocument();
});

test('adds a size and reports it upward', async () => {
  const onAdd = vi.fn().mockResolvedValue(undefined);
  renderPanel({ onAdd });

  const input = screen.getByPlaceholderText('Add a size, e.g. M, 42, S/M…');
  fireEvent.change(input, { target: { value: '42' } });
  fireEvent.click(screen.getByLabelText('Add'));

  await vi.waitFor(() => expect(onAdd).toHaveBeenCalledWith('42'));
  await vi.waitFor(() => expect(input).toHaveValue(''));
});

test('edits a size inline and reports it upward', async () => {
  const onEdit = vi.fn().mockResolvedValue(undefined);
  renderPanel({ sizes: [size()], onEdit });

  fireEvent.click(screen.getByLabelText('Edit'));
  const editInput = screen.getByDisplayValue('M');
  fireEvent.change(editInput, { target: { value: 'L' } });
  fireEvent.click(screen.getByLabelText('Save'));

  await vi.waitFor(() => expect(onEdit).toHaveBeenCalledWith(size(), 'L'));
});

test('deletes a size', () => {
  const onDelete = vi.fn().mockResolvedValue(undefined);
  renderPanel({ sizes: [size()], onDelete });

  fireEvent.click(screen.getByLabelText('Delete'));
  expect(onDelete).toHaveBeenCalledWith('size-1');
});
