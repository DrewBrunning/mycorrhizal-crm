import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import WorkInformationDialog from './WorkInformationDialog';

afterEach(cleanup);

function renderDialog(props: Partial<React.ComponentProps<typeof WorkInformationDialog>> = {}) {
  const defaults: React.ComponentProps<typeof WorkInformationDialog> = {
    open: true,
    onClose: vi.fn(),
    onSave: vi.fn().mockResolvedValue(undefined),
    ...props,
  };
  return render(<WorkInformationDialog {...defaults} />);
}

test('create mode starts empty and names itself add', () => {
  renderDialog();
  expect(screen.getByRole('heading', { name: 'Add work information' })).toBeInTheDocument();
  expect(screen.getByLabelText('Work Information')).toHaveValue('');
});

test('edit mode pre-fills the existing value', () => {
  renderDialog({ value: 'Remote, PST hours' });
  expect(screen.getByRole('heading', { name: 'Edit work information' })).toBeInTheDocument();
  expect(screen.getByLabelText('Work Information')).toHaveValue('Remote, PST hours');
});

test('saves the trimmed text and closes', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  const onClose = vi.fn();
  renderDialog({ onSave, onClose });

  fireEvent.change(screen.getByLabelText('Work Information'), {
    target: { value: '  Remote, PST hours  ' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(onSave).toHaveBeenCalledWith('Remote, PST hours'));
  expect(onClose).toHaveBeenCalled();
});

test('clearing the text saves an empty string so the backend clears the field', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave, value: 'Remote' });

  fireEvent.change(screen.getByLabelText('Work Information'), { target: { value: '' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(onSave).toHaveBeenCalledWith(''));
});

test('a failed save keeps the dialog open and shows an error', async () => {
  const onSave = vi.fn().mockRejectedValue(new Error('nope'));
  const onClose = vi.fn();
  renderDialog({ onSave, onClose, value: 'Remote' });

  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(await screen.findByText("Couldn't save this entry.")).toBeInTheDocument();
  expect(onClose).not.toHaveBeenCalled();
});
