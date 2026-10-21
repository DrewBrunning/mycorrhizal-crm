import { cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { CardPersonalInfo, PersonalInfoMeta } from '../api/contacts';
import PersonalInfoList from './PersonalInfoList';

// vitest here has no auto-cleanup and no globals — see CLAUDE.md.
afterEach(cleanup);

function entry(overrides: Partial<CardPersonalInfo> = {}): CardPersonalInfo {
  return { id: 'pi-1', kind: 'expertise', value: 'Go', ...overrides };
}

function renderList(entries: CardPersonalInfo[], metaById?: Record<string, PersonalInfoMeta>) {
  const onEdit = vi.fn();
  const onDelete = vi.fn();
  render(
    <PersonalInfoList entries={entries} metaById={metaById} onEdit={onEdit} onDelete={onDelete} />,
  );
  return { onEdit, onDelete };
}

test('shows the empty state when there are no skills or interests', () => {
  renderList([]);
  expect(screen.getByText('No skills or interests yet.')).toBeInTheDocument();
});

test('groups entries by kind and keeps the expertise/hobby/interest order', () => {
  renderList([
    entry({ id: 'pi-1', kind: 'interest', value: 'Architecture' }),
    entry({ id: 'pi-2', kind: 'hobby', value: 'Gardening' }),
    entry({ id: 'pi-3', kind: 'expertise', value: 'Go' }),
    entry({ id: 'pi-4', kind: 'hobby', value: 'Piano' }),
  ]);

  const expertise = within(screen.getByRole('region', { name: 'Expertise' }));
  const hobby = within(screen.getByRole('region', { name: 'Hobby' }));
  const interest = within(screen.getByRole('region', { name: 'Interest' }));

  expect(expertise.getByText('Go')).toBeInTheDocument();
  expect(hobby.getByText('Gardening')).toBeInTheDocument();
  expect(hobby.getByText('Piano')).toBeInTheDocument();
  expect(interest.getByText('Architecture')).toBeInTheDocument();

  // DOM order matches PERSONAL_INFO_KIND_OPTIONS, not the input order.
  const headings = screen.getAllByText(/^(Expertise|Hobby|Interest)$/);
  expect(headings.map((h) => h.textContent)).toEqual(['Expertise', 'Hobby', 'Interest']);
});

test('renders level, label and CRM metadata (favorite, lock badge, notes)', () => {
  renderList(
    [
      entry({
        id: 'pi-1',
        kind: 'hobby',
        value: 'Piano',
        level: 'high',
        label: 'classical',
      }),
      entry({ id: 'pi-2', kind: 'expertise', value: 'Go', level: 'medium' }),
    ],
    {
      'pi-1': {
        favorite: true,
        notes: 'Wants to learn a Chopin nocturne',
        sensitivity: 'private',
      },
    },
  );

  expect(screen.getByText('High')).toBeInTheDocument();
  expect(screen.getByText('classical')).toBeInTheDocument();
  expect(screen.getByLabelText('Favorite')).toBeInTheDocument();
  expect(screen.getByLabelText('Private')).toBeInTheDocument();
  expect(screen.getByText('Wants to learn a Chopin nocturne')).toBeInTheDocument();

  // The unmarked entry has neither badge.
  const expertise = within(screen.getByRole('region', { name: 'Expertise' }));
  expect(expertise.queryByLabelText('Favorite')).not.toBeInTheDocument();
  expect(expertise.queryByLabelText('Private')).not.toBeInTheDocument();
});

test('shows no lock badge for a normal-sensitivity entry', () => {
  renderList([entry()], { 'pi-1': { sensitivity: 'normal' } });
  expect(screen.queryByLabelText('Private')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('Secret')).not.toBeInTheDocument();
});

test('a kind outside the fixed vocabulary is still shown, under Other', () => {
  renderList([entry({ kind: 'future_kind', value: 'Something new' })]);
  const other = within(screen.getByRole('region', { name: 'Other' }));
  expect(other.getByText('Something new')).toBeInTheDocument();
});

test('edit calls back with the exact entry', () => {
  const target = entry({ id: 'pi-9', kind: 'interest', value: 'Typography' });
  const { onEdit } = renderList([target]);

  fireEvent.click(screen.getByLabelText('Edit'));
  expect(onEdit).toHaveBeenCalledWith(target);
});

test('delete asks for confirmation before calling back', () => {
  const confirm = vi.fn(() => false);
  vi.stubGlobal('confirm', confirm);
  const target = entry();
  const { onDelete } = renderList([target]);

  fireEvent.click(screen.getByLabelText('Delete'));
  expect(confirm).toHaveBeenCalled();
  expect(onDelete).not.toHaveBeenCalled();

  confirm.mockReturnValue(true);
  fireEvent.click(screen.getByLabelText('Delete'));
  expect(onDelete).toHaveBeenCalledWith(target);
  vi.unstubAllGlobals();
});
