import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { CardPersonalInfo, PersonalInfoMeta } from '../api/contacts';
import PersonalInfoDialog from './PersonalInfoDialog';

afterEach(cleanup);

function renderDialog(props: Partial<React.ComponentProps<typeof PersonalInfoDialog>> = {}) {
  const defaults: React.ComponentProps<typeof PersonalInfoDialog> = {
    open: true,
    onClose: vi.fn(),
    onSave: vi.fn().mockResolvedValue(undefined),
    ...props,
  };
  return render(<PersonalInfoDialog {...defaults} />);
}

test('create mode starts with the first kind, no level and normal sensitivity', () => {
  renderDialog();
  expect(screen.getByRole('heading', { name: 'Add skill or interest' })).toBeInTheDocument();
  expect(screen.getByLabelText('Type *')).toHaveTextContent('Expertise');
  expect(screen.getByLabelText('Value *')).toHaveValue('');
  expect(screen.getByLabelText('Level')).toBeInTheDocument();
  expect(screen.getByLabelText('Sensitivity')).toHaveTextContent('Normal');
  expect(screen.getByRole('switch', { name: 'Favorite' })).not.toBeChecked();
});

test('requires a value before saving', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(await screen.findByText('Enter a skill, hobby or interest.')).toBeInTheDocument();
  expect(onSave).not.toHaveBeenCalled();
});

test('saves kind, value, level, label, notes, sensitivity and favorite', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  const onClose = vi.fn();
  renderDialog({ onSave, onClose });

  fireEvent.mouseDown(screen.getByLabelText('Type *'));
  fireEvent.click(await screen.findByRole('option', { name: 'Interest' }));
  fireEvent.change(screen.getByLabelText('Value *'), { target: { value: 'Typography' } });
  fireEvent.mouseDown(screen.getByLabelText('Level'));
  fireEvent.click(await screen.findByRole('option', { name: 'Medium' }));
  fireEvent.change(screen.getByLabelText('Label'), { target: { value: 'systems' } });
  fireEvent.change(screen.getByLabelText('Notes (optional)'), {
    target: { value: 'Likes Swiss grotesques' },
  });
  fireEvent.mouseDown(screen.getByLabelText('Sensitivity'));
  fireEvent.click(await screen.findByRole('option', { name: 'Secret' }));
  fireEvent.click(screen.getByRole('switch', { name: 'Favorite' }));
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave).toHaveBeenCalledWith({
    kind: 'interest',
    value: 'Typography',
    level: 'medium',
    label: 'systems',
    notes: 'Likes Swiss grotesques',
    sensitivity: 'secret',
    favorite: true,
  });
  expect(onClose).toHaveBeenCalled();
});

test('trims text values and omits blank optional fields', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  fireEvent.change(screen.getByLabelText('Value *'), { target: { value: '  Go  ' } });
  fireEvent.change(screen.getByLabelText('Label'), { target: { value: '   ' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave).toHaveBeenCalledWith({
    kind: 'expertise',
    value: 'Go',
    level: undefined,
    label: undefined,
    notes: undefined,
    sensitivity: 'normal',
    favorite: false,
  });
});

test('edit mode pre-fills the card entry and its CRM metadata', () => {
  const entry: CardPersonalInfo = {
    id: 'pi-1',
    kind: 'hobby',
    value: 'Piano',
    level: 'high',
    label: 'classical',
  };
  const meta: PersonalInfoMeta = {
    notes: 'Learning Chopin',
    sensitivity: 'private',
    favorite: true,
  };
  renderDialog({ entry, meta });

  expect(screen.getByRole('heading', { name: 'Edit skill or interest' })).toBeInTheDocument();
  expect(screen.getByLabelText('Type *')).toHaveTextContent('Hobby');
  expect(screen.getByLabelText('Value *')).toHaveValue('Piano');
  expect(screen.getByLabelText('Level')).toHaveTextContent('High');
  expect(screen.getByLabelText('Label')).toHaveValue('classical');
  expect(screen.getByLabelText('Notes (optional)')).toHaveValue('Learning Chopin');
  expect(screen.getByLabelText('Sensitivity')).toHaveTextContent('Private');
  expect(screen.getByRole('switch', { name: 'Favorite' })).toBeChecked();
});

test('a failed save keeps the dialog open and shows an error', async () => {
  const onSave = vi.fn().mockRejectedValue(new Error('nope'));
  const onClose = vi.fn();
  renderDialog({
    onSave,
    onClose,
    entry: { id: 'pi-1', kind: 'hobby', value: 'Piano' },
  });

  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(await screen.findByText("Couldn't save this entry.")).toBeInTheDocument();
  expect(onClose).not.toHaveBeenCalled();
});

test('cancel closes without saving', () => {
  const onSave = vi.fn();
  const onClose = vi.fn();
  renderDialog({ onSave, onClose });

  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

  expect(onClose).toHaveBeenCalled();
  expect(onSave).not.toHaveBeenCalled();
});
