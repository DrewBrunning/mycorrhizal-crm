import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import PreferenceDialog from './PreferenceDialog';

afterEach(cleanup);

function renderDialog(props: Partial<React.ComponentProps<typeof PreferenceDialog>> = {}) {
  const defaults: React.ComponentProps<typeof PreferenceDialog> = {
    open: true,
    onClose: vi.fn(),
    onSave: vi.fn().mockResolvedValue(undefined),
    ...props,
  };
  return render(<PreferenceDialog {...defaults} />);
}

test('create mode shows the category, value, key, notes, and sensitivity fields', () => {
  renderDialog();
  expect(screen.getByLabelText('Category *')).toBeInTheDocument();
  expect(screen.getByLabelText('Value *')).toBeInTheDocument();
  expect(screen.getByLabelText('Key')).toBeInTheDocument();
  expect(screen.getByLabelText('Notes (optional)')).toBeInTheDocument();
  expect(screen.getByLabelText('Sensitivity')).toBeInTheDocument();
});

test('requires a value before saving', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(await screen.findByText('Value is required')).toBeInTheDocument();
  expect(onSave).not.toHaveBeenCalled();
});

test('saves with the selected category, value, notes, and sensitivity', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  fireEvent.change(screen.getByLabelText('Value *'), { target: { value: 'Severance' } });
  fireEvent.mouseDown(screen.getByLabelText('Category *'));
  fireEvent.click(await screen.findByRole('option', { name: 'TV show' }));
  fireEvent.change(screen.getByLabelText('Notes (optional)'), {
    target: { value: 'Watched together' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave).toHaveBeenCalledWith({
    category: 'media_tv',
    key: undefined,
    value: 'Severance',
    notes: 'Watched together',
    sensitivity: 'normal',
  });
});

test('key suggestions follow the selected category', async () => {
  renderDialog();

  // The key field is a free-solo Autocomplete; its combobox input carries the
  // "Key" label. Default category is food: favorite/dislike/allergy.
  const keyInput = screen.getByRole('combobox', { name: 'Key' });
  fireEvent.mouseDown(keyInput);
  expect(await screen.findByRole('option', { name: 'Favorite' })).toBeInTheDocument();
  expect(screen.getByRole('option', { name: 'Allergy' })).toBeInTheDocument();

  // Close the key dropdown and switch to a media category (medium lives in
  // the category now; key is disposition — favorite/like/dislike).
  fireEvent.keyDown(keyInput, { key: 'Escape' });
  fireEvent.mouseDown(screen.getByLabelText('Category *'));
  fireEvent.click(await screen.findByRole('option', { name: 'Movie' }));
  fireEvent.mouseDown(keyInput);
  expect(await screen.findByRole('option', { name: 'Favorite' })).toBeInTheDocument();
  expect(screen.getByRole('option', { name: 'Like' })).toBeInTheDocument();
  expect(screen.getByRole('option', { name: 'Dislike' })).toBeInTheDocument();
});

test('category options are grouped by section', async () => {
  renderDialog();
  fireEvent.mouseDown(screen.getByLabelText('Category *'));
  expect(await screen.findByText('Food & Drink Preferences')).toBeInTheDocument();
  expect(screen.getByText('Media Preferences')).toBeInTheDocument();
  expect(screen.getByText('Jewelry & Style')).toBeInTheDocument();
  expect(screen.getByText('Gift Preferences')).toBeInTheDocument();
  expect(screen.getByText('Gift Avoid')).toBeInTheDocument();
  expect(screen.getByRole('option', { name: 'Jewelry — metal' })).toBeInTheDocument();
  expect(screen.getByRole('option', { name: 'Flowers' })).toBeInTheDocument();
});

test('edit mode pre-fills the existing preference', () => {
  renderDialog({
    preference: {
      id: 'p1',
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
      entity_id: 'alice-uid',
      category: 'food',
      value: 'Vegetarian',
      notes: 'No exceptions',
      sensitivity: 'normal',
    },
  });
  expect(screen.getByLabelText('Value *')).toHaveValue('Vegetarian');
  expect(screen.getByLabelText('Notes (optional)')).toHaveValue('No exceptions');
});

// Issue #246: the proficiency picker is gated to hobby/skill categories.
test('the proficiency picker appears only for the hobby category', async () => {
  renderDialog();

  // Default category is food — no level field.
  expect(screen.queryByRole('combobox', { name: 'Proficiency' })).not.toBeInTheDocument();

  fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Category' }));
  fireEvent.click(await screen.findByRole('option', { name: 'Activity/hobby' }));
  expect(await screen.findByRole('combobox', { name: 'Proficiency' })).toBeInTheDocument();

  // Switching to a non-hobby category hides it again.
  fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Category' }));
  fireEvent.click(await screen.findByRole('option', { name: 'Drink' }));
  await vi.waitFor(() =>
    expect(screen.queryByRole('combobox', { name: 'Proficiency' })).not.toBeInTheDocument(),
  );
});

test('saves the selected proficiency level for a hobby', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Category' }));
  fireEvent.click(await screen.findByRole('option', { name: 'Activity/hobby' }));
  fireEvent.change(screen.getByLabelText('Value *'), { target: { value: 'Piano' } });

  fireEvent.mouseDown(await screen.findByRole('combobox', { name: 'Proficiency' }));
  fireEvent.click(await screen.findByRole('option', { name: 'Advanced' }));
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave).toHaveBeenCalledWith({
    category: 'hobby',
    key: undefined,
    value: 'Piano',
    level: 'high',
    notes: undefined,
    sensitivity: 'normal',
  });
});

test('switching away from hobby drops a selected level', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Category' }));
  fireEvent.click(await screen.findByRole('option', { name: 'Activity/hobby' }));
  fireEvent.change(screen.getByLabelText('Value *'), { target: { value: 'Piano' } });
  fireEvent.mouseDown(await screen.findByRole('combobox', { name: 'Proficiency' }));
  fireEvent.click(await screen.findByRole('option', { name: 'Advanced' }));

  // Now a category with no level concept.
  fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Category' }));
  fireEvent.click(await screen.findByRole('option', { name: 'Food' }));
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave).toHaveBeenCalledWith(
    expect.objectContaining({ category: 'food', level: undefined }),
  );
});

test('edit mode pre-fills an existing hobby level', async () => {
  renderDialog({
    preference: {
      id: 'p1',
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
      entity_id: 'alice-uid',
      category: 'hobby',
      value: 'Piano',
      level: 'medium',
      sensitivity: 'normal',
    },
  });

  expect(await screen.findByRole('combobox', { name: 'Proficiency' })).toHaveTextContent(
    'Intermediate',
  );
});
