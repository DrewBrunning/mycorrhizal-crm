import { cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { CardEntryPeriod, CardOrganization, CardTitle } from '../api/contacts';
import WorkList from './WorkList';

afterEach(cleanup);

function renderList(props: Partial<React.ComponentProps<typeof WorkList>> = {}) {
  const callbacks = {
    onAddOrganization: vi.fn(),
    onEditOrganization: vi.fn(),
    onDeleteOrganization: vi.fn(),
    onAddTitle: vi.fn(),
    onEditTitle: vi.fn(),
    onDeleteTitle: vi.fn(),
    onEditWorkInformation: vi.fn(),
  };
  const defaults: React.ComponentProps<typeof WorkList> = {
    organizations: [],
    titles: [],
    showOrganizations: true,
    showTitles: true,
    showWorkInformation: true,
    ...callbacks,
    ...props,
  };
  render(<WorkList {...defaults} />);
  return callbacks;
}

const organizations: CardOrganization[] = [
  { id: 'org-1', name: 'Acme', units: [{ name: 'Sales' }] },
];
const titles: CardTitle[] = [
  { id: 'title-1', name: 'Engineer', kind: 'title' },
  { id: 'title-2', name: 'Mentor', kind: 'role' },
];
const periods: CardEntryPeriod[] = [
  {
    kind: 'organization',
    entry_id: 'org-1',
    range: { start: { year: 2019 }, end: { year: 2024 } },
  },
  { kind: 'title', entry_id: 'title-1', range: { start: { year: 2020 } } },
];

test('shows an empty hint and add affordance for every enabled subsection', () => {
  const callbacks = renderList();

  const org = within(screen.getByRole('region', { name: 'Organizations' }));
  const title = within(screen.getByRole('region', { name: 'Titles & roles' }));
  const info = within(screen.getByRole('region', { name: 'Work Information' }));

  expect(org.getByText('No organizations yet.')).toBeInTheDocument();
  expect(title.getByText('No titles or roles yet.')).toBeInTheDocument();
  expect(info.getByText('No work information yet.')).toBeInTheDocument();

  fireEvent.click(org.getByRole('button', { name: 'Add organization' }));
  fireEvent.click(title.getByRole('button', { name: 'Add title or role' }));
  fireEvent.click(info.getByRole('button', { name: 'Edit work information' }));

  expect(callbacks.onAddOrganization).toHaveBeenCalled();
  expect(callbacks.onAddTitle).toHaveBeenCalled();
  expect(callbacks.onEditWorkInformation).toHaveBeenCalled();
});

test('renders organizations with department and period, and titles by kind with period', () => {
  renderList({ organizations, titles, periods });

  const org = within(screen.getByRole('region', { name: 'Organizations' }));
  expect(org.getByText('Acme')).toBeInTheDocument();
  expect(org.getByText('Sales')).toBeInTheDocument();
  expect(org.getByText('2019 – 2024')).toBeInTheDocument();

  const title = within(screen.getByRole('region', { name: 'Titles & roles' }));
  expect(title.getByText('Engineer')).toBeInTheDocument();
  expect(title.getByText('Mentor')).toBeInTheDocument();
  expect(title.getByText('Job Title')).toBeInTheDocument();
  expect(title.getByText('Role')).toBeInTheDocument();
  expect(title.getByText('Since 2020')).toBeInTheDocument();
});

test('renders work information as preserved text', () => {
  renderList({ workInformation: 'Remote, PST hours\nOn call on Tuesdays' });
  expect(screen.getByText(/Remote, PST hours/)).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Edit work information' })).toBeInTheDocument();
});

test('each subsection follows its own visibility switch', () => {
  renderList({
    organizations,
    titles,
    workInformation: 'Remote',
    showOrganizations: false,
    showTitles: false,
    showWorkInformation: true,
  });

  expect(screen.queryByRole('region', { name: 'Organizations' })).not.toBeInTheDocument();
  expect(screen.queryByRole('region', { name: 'Titles & roles' })).not.toBeInTheDocument();
  expect(screen.getByRole('region', { name: 'Work Information' })).toBeInTheDocument();
});

test('edit and delete call back with the exact entry', () => {
  const callbacks = renderList({ organizations, titles });
  const org = within(screen.getByRole('region', { name: 'Organizations' }));
  const title = within(screen.getByRole('region', { name: 'Titles & roles' }));

  const confirm = vi.fn(() => true);
  vi.stubGlobal('confirm', confirm);

  fireEvent.click(org.getByLabelText('Edit'));
  expect(callbacks.onEditOrganization).toHaveBeenCalledWith(organizations[0]);

  fireEvent.click(org.getByLabelText('Delete'));
  expect(callbacks.onDeleteOrganization).toHaveBeenCalledWith(organizations[0]);

  fireEvent.click(title.getAllByLabelText('Edit')[1]);
  expect(callbacks.onEditTitle).toHaveBeenCalledWith(titles[1]);

  fireEvent.click(title.getAllByLabelText('Delete')[0]);
  expect(callbacks.onDeleteTitle).toHaveBeenCalledWith(titles[0]);

  vi.unstubAllGlobals();
});

test('declining the delete confirmation makes no callback', () => {
  const callbacks = renderList({ organizations });
  vi.stubGlobal(
    'confirm',
    vi.fn(() => false),
  );

  fireEvent.click(screen.getByLabelText('Delete'));

  expect(callbacks.onDeleteOrganization).not.toHaveBeenCalled();
  vi.unstubAllGlobals();
});
