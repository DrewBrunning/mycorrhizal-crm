import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { FieldDefinition } from '../api/fieldDefinitions';
import { SnackbarProvider } from '../context/SnackbarContext';
import { DateFormatProvider } from '../DateFormatProvider';
import CustomFieldGroups from './CustomFieldGroups';

afterEach(cleanup);

function def(id: string, label: string, group: string): FieldDefinition {
  return {
    id,
    label,
    key: id,
    target: 'contact',
    type: 'string',
    projection: 'internal-only',
    sensitivity: 'normal',
    position: 0,
    group,
    created_at: '',
    updated_at: '',
  };
}

function renderGroups(
  definitions: FieldDefinition[],
  values: Record<string, unknown> = {},
  onSave = vi.fn().mockResolvedValue(undefined),
) {
  render(
    <SnackbarProvider>
      <DateFormatProvider>
        <CustomFieldGroups
          definitions={definitions}
          valuesByDefinition={new Map(Object.entries(values))}
          onSave={onSave}
        />
      </DateFormatProvider>
    </SnackbarProvider>,
  );
  return onSave;
}

test('with no grouped definitions the rows render flat, with no accordion', () => {
  renderGroups([def('a', 'Alpha', ''), def('b', 'Beta', '')]);
  expect(screen.getByText('Alpha')).toBeInTheDocument();
  expect(screen.getByText('Beta')).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Custom fields' })).not.toBeInTheDocument();
});

test('renders one accordion per group, alphabetical, ungrouped "Custom fields" last', () => {
  renderGroups([def('a', 'Loose', ''), def('w', 'Desk', 'Work'), def('t', 'Hotel', 'Travel')]);
  const headers = screen
    .getAllByRole('button')
    .filter((b) => b.hasAttribute('aria-expanded'))
    .map((b) => b.textContent);
  expect(headers).toEqual(['Travel', 'Work', 'Custom fields']);
});

test('a group without filled values is collapsed; a filled one is open', () => {
  renderGroups([def('w', 'Desk', 'Work'), def('t', 'Hotel', 'Travel')], { t: 'Hilton' });
  expect(screen.getByRole('button', { name: 'Travel' })).toHaveAttribute('aria-expanded', 'true');
  expect(screen.getByRole('button', { name: 'Work' })).toHaveAttribute('aria-expanded', 'false');
  fireEvent.click(screen.getByRole('button', { name: 'Work' }));
  expect(screen.getByRole('button', { name: 'Work' })).toHaveAttribute('aria-expanded', 'true');
});

test('case-different labels are separate groups', () => {
  renderGroups([def('a', 'One', 'Travel'), def('b', 'Two', 'travel')]);
  expect(screen.getByRole('button', { name: 'Travel' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'travel' })).toBeInTheDocument();
});

test('editing a value in a group saves through onSave', async () => {
  const onSave = renderGroups([def('t', 'Hotel', 'Travel')], { t: 'Hilton' });
  fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
  fireEvent.change(screen.getByDisplayValue('Hilton'), { target: { value: 'Ritz' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  await vi.waitFor(() => expect(onSave).toHaveBeenCalledWith('t', 'Ritz'));
});
