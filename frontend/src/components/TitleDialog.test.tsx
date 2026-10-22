import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { CardTitle } from '../api/contacts';
import TitleDialog from './TitleDialog';

afterEach(cleanup);

function renderDialog(props: Partial<React.ComponentProps<typeof TitleDialog>> = {}) {
  const defaults: React.ComponentProps<typeof TitleDialog> = {
    open: true,
    onClose: vi.fn(),
    onSave: vi.fn().mockResolvedValue(undefined),
    ...props,
  };
  return render(<TitleDialog {...defaults} />);
}

const existing: CardTitle = { id: 'title-1', name: 'Engineer', kind: 'title' };

test('create mode defaults to a job title and no period', () => {
  renderDialog();
  expect(screen.getByRole('heading', { name: 'Add title or role' })).toBeInTheDocument();
  expect(screen.getByLabelText('Type')).toHaveTextContent('Job Title');
  expect(screen.getByLabelText('Title *')).toHaveValue('');
  expect(screen.getByLabelText('From (year)')).toHaveValue(null);
});

test('requires a title name', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(await screen.findByText('Enter a name.')).toBeInTheDocument();
  expect(onSave).not.toHaveBeenCalled();
});

test('saves a role with its period', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  fireEvent.mouseDown(screen.getByLabelText('Type'));
  fireEvent.click(await screen.findByRole('option', { name: 'Role' }));
  fireEvent.change(screen.getByLabelText('Title *'), { target: { value: 'Mentor' } });
  fireEvent.change(screen.getByLabelText('From (year)'), { target: { value: '2021' } });
  fireEvent.change(screen.getByLabelText('To (year)'), { target: { value: '2023' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave).toHaveBeenCalledWith({
    kind: 'role',
    name: 'Mentor',
    startYear: 2021,
    endYear: 2023,
  });
});

test('rejects a reversed period', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  fireEvent.change(screen.getByLabelText('Title *'), { target: { value: 'Engineer' } });
  fireEvent.change(screen.getByLabelText('From (year)'), { target: { value: '2024' } });
  fireEvent.change(screen.getByLabelText('To (year)'), { target: { value: '2020' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(await screen.findByText('Enter a valid start and end year.')).toBeInTheDocument();
  expect(onSave).not.toHaveBeenCalled();
});

test('edit mode pre-fills the title and its existing period', () => {
  renderDialog({
    title: existing,
    period: { start: { year: 2020 }, end: { year: 2024 } },
  });

  expect(screen.getByRole('heading', { name: 'Edit title or role' })).toBeInTheDocument();
  expect(screen.getByLabelText('Type')).toHaveTextContent('Job Title');
  expect(screen.getByLabelText('Title *')).toHaveValue('Engineer');
  expect(screen.getByLabelText('From (year)')).toHaveValue(2020);
  expect(screen.getByLabelText('To (year)')).toHaveValue(2024);
});

test('a failed save keeps the dialog open and shows an error', async () => {
  const onSave = vi.fn().mockRejectedValue(new Error('nope'));
  const onClose = vi.fn();
  renderDialog({ onSave, onClose, title: existing });

  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(await screen.findByText("Couldn't save this entry.")).toBeInTheDocument();
  expect(onClose).not.toHaveBeenCalled();
});
