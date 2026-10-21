import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { CardOrganization } from '../api/contacts';
import OrganizationDialog from './OrganizationDialog';

afterEach(cleanup);

function renderDialog(props: Partial<React.ComponentProps<typeof OrganizationDialog>> = {}) {
  const defaults: React.ComponentProps<typeof OrganizationDialog> = {
    open: true,
    onClose: vi.fn(),
    onSave: vi.fn().mockResolvedValue(undefined),
    ...props,
  };
  return render(<OrganizationDialog {...defaults} />);
}

const existing: CardOrganization = {
  id: 'org-1',
  name: 'Acme',
  units: [{ name: 'Sales' }],
};

test('create mode starts empty', () => {
  renderDialog();
  expect(screen.getByRole('heading', { name: 'Add organization' })).toBeInTheDocument();
  expect(screen.getByLabelText('Organization *')).toHaveValue('');
  expect(screen.getByLabelText('Department')).toHaveValue('');
  expect(screen.getByLabelText('From (year)')).toHaveValue(null);
  expect(screen.getByLabelText('To (year)')).toHaveValue(null);
});

test('requires an organization name', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(await screen.findByText('Enter a name.')).toBeInTheDocument();
  expect(onSave).not.toHaveBeenCalled();
});

test('saves name, department and period years', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  const onClose = vi.fn();
  renderDialog({ onSave, onClose });

  fireEvent.change(screen.getByLabelText('Organization *'), {
    target: { value: '  NewCo ' },
  });
  fireEvent.change(screen.getByLabelText('Department'), { target: { value: 'Research' } });
  fireEvent.change(screen.getByLabelText('From (year)'), { target: { value: '2019' } });
  fireEvent.change(screen.getByLabelText('To (year)'), { target: { value: '2024' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave).toHaveBeenCalledWith({
    name: 'NewCo',
    department: 'Research',
    startYear: 2019,
    endYear: 2024,
  });
  expect(onClose).toHaveBeenCalled();
});

test('rejects a reversed period', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({ onSave });

  fireEvent.change(screen.getByLabelText('Organization *'), { target: { value: 'Acme' } });
  fireEvent.change(screen.getByLabelText('From (year)'), { target: { value: '2024' } });
  fireEvent.change(screen.getByLabelText('To (year)'), { target: { value: '2019' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(await screen.findByText('Enter a valid start and end year.')).toBeInTheDocument();
  expect(onSave).not.toHaveBeenCalled();
});

test('edit mode pre-fills the organization and its existing period', () => {
  renderDialog({
    organization: existing,
    period: { start: { year: 2019 }, end: { year: 2024 } },
  });

  expect(screen.getByRole('heading', { name: 'Edit organization' })).toBeInTheDocument();
  expect(screen.getByLabelText('Organization *')).toHaveValue('Acme');
  expect(screen.getByLabelText('Department')).toHaveValue('Sales');
  expect(screen.getByLabelText('From (year)')).toHaveValue(2019);
  expect(screen.getByLabelText('To (year)')).toHaveValue(2024);
});

test('clearing the period emits no years', async () => {
  const onSave = vi.fn().mockResolvedValue(undefined);
  renderDialog({
    onSave,
    organization: existing,
    period: { start: { year: 2019 }, end: { year: 2024 } },
  });

  fireEvent.change(screen.getByLabelText('From (year)'), { target: { value: '' } });
  fireEvent.change(screen.getByLabelText('To (year)'), { target: { value: '' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await vi.waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave).toHaveBeenCalledWith({
    name: 'Acme',
    department: 'Sales',
    startYear: undefined,
    endYear: undefined,
  });
});

test('a failed save keeps the dialog open and shows an error', async () => {
  const onSave = vi.fn().mockRejectedValue(new Error('nope'));
  const onClose = vi.fn();
  renderDialog({ onSave, onClose, organization: existing });

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
