import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { useState } from 'react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { ContactAddress } from '../api/contacts';
import AddressFields from './AddressFields';

// This codebase's vitest setup has no auto-cleanup and no globals: true.
afterEach(cleanup);

const addr = (patch: Partial<ContactAddress> = {}): ContactAddress => ({
  type: 'home',
  street: '',
  city: '',
  region: '',
  postal: '',
  country: '',
  pobox: '',
  apartment: '',
  floor: '',
  ...patch,
});

// Controlled harness so edits flow back into the component the way a real
// form does; `onChange` records every emitted value.
function Harness({
  initial,
  onChange,
}: {
  initial: ContactAddress[];
  onChange: (next: ContactAddress[]) => void;
}) {
  const [value, setValue] = useState(initial);
  return (
    <AddressFields
      label="Addresses"
      value={value}
      onChange={(next) => {
        onChange(next);
        setValue(next);
      }}
    />
  );
}

function renderFields(initial: ContactAddress[] = []) {
  const onChange = vi.fn<(next: ContactAddress[]) => void>();
  render(<Harness initial={initial} onChange={onChange} />);
  const last = () => onChange.mock.calls.at(-1)?.[0];
  return { onChange, last };
}

test('renders the label and no address rows when empty', () => {
  renderFields();
  expect(screen.getByText('Addresses')).toBeInTheDocument();
  expect(screen.queryByLabelText('Street')).not.toBeInTheDocument();
});

test('Add appends an empty home address', () => {
  const { last } = renderFields();
  fireEvent.click(screen.getByRole('button', { name: 'Add' }));
  expect(last()).toEqual([addr()]);
  expect(screen.getByLabelText('Street')).toHaveValue('');
});

test('editing each field patches only that address', () => {
  const { last } = renderFields([addr({ street: 'Old St' }), addr({ city: 'Keep' })]);
  const [first] = screen.getAllByLabelText('Street');
  fireEvent.change(first, { target: { value: '1 Main St' } });
  fireEvent.change(screen.getAllByLabelText('City')[0], { target: { value: 'Springfield' } });
  fireEvent.change(screen.getAllByLabelText('State / Region')[0], { target: { value: 'IL' } });
  fireEvent.change(screen.getAllByLabelText('Postal Code')[0], { target: { value: '62701' } });
  fireEvent.change(screen.getAllByLabelText('Country')[0], { target: { value: 'US' } });
  fireEvent.change(screen.getAllByLabelText('Lived here from (year)')[0], {
    target: { value: '2019' },
  });
  fireEvent.change(screen.getAllByLabelText('Until (year)')[0], { target: { value: '2024' } });

  expect(last()).toEqual([
    addr({
      street: '1 Main St',
      city: 'Springfield',
      region: 'IL',
      postal: '62701',
      country: 'US',
      periodStartYear: '2019',
      periodEndYear: '2024',
    }),
    addr({ city: 'Keep' }),
  ]);
});

test('PO box / apartment / floor are hidden until revealed, then editable', () => {
  const { last } = renderFields([addr()]);
  expect(screen.queryByLabelText('PO Box')).not.toBeInTheDocument();

  fireEvent.click(screen.getByRole('button', { name: 'Additional fields' }));
  fireEvent.change(screen.getByLabelText('PO Box'), { target: { value: '12' } });
  fireEvent.change(screen.getByLabelText('Apartment / Suite'), { target: { value: '4B' } });
  fireEvent.change(screen.getByLabelText('Floor'), { target: { value: '3' } });

  expect(last()).toEqual([addr({ pobox: '12', apartment: '4B', floor: '3' })]);
  expect(screen.queryByRole('button', { name: 'Additional fields' })).not.toBeInTheDocument();
});

test('an imported address that already carries an additional part shows them without a toggle (T80)', () => {
  renderFields([addr({ apartment: 'Suite 9' }), addr()]);
  expect(screen.getByLabelText('Apartment / Suite')).toHaveValue('Suite 9');
  // Only the second (empty) address still offers the toggle.
  expect(screen.getAllByRole('button', { name: 'Additional fields' })).toHaveLength(1);
});

test('whitespace-only additional parts do not count as present', () => {
  renderFields([addr({ pobox: '   ' })]);
  expect(screen.getByRole('button', { name: 'Additional fields' })).toBeInTheDocument();
});

test('the revealed state follows the row, not the index, when an earlier row is removed', () => {
  const { last } = renderFields([addr({ street: 'A' }), addr({ street: 'B' })]);
  // Reveal the second row's additional fields.
  fireEvent.click(screen.getAllByRole('button', { name: 'Additional fields' })[1]);
  expect(screen.getAllByLabelText('PO Box')).toHaveLength(1);

  // Remove the first row: the remaining row (B) keeps its revealed state.
  fireEvent.click(screen.getAllByRole('button', { name: 'Delete' })[0]);
  expect(last()).toEqual([addr({ street: 'B' })]);
  expect(screen.getByLabelText('Street')).toHaveValue('B');
  expect(screen.getByLabelText('PO Box')).toBeInTheDocument();
});

test('removing a row that was never revealed leaves the other row collapsed', () => {
  renderFields([addr({ street: 'A' }), addr({ street: 'B' })]);
  fireEvent.click(screen.getAllByRole('button', { name: 'Additional fields' })[0]);
  fireEvent.click(screen.getAllByRole('button', { name: 'Delete' })[0]);
  expect(screen.getByLabelText('Street')).toHaveValue('B');
  expect(screen.queryByLabelText('PO Box')).not.toBeInTheDocument();
});

test('typing a custom type label stores it verbatim; picking a standard one trims', () => {
  const { last } = renderFields([addr()]);
  const typeInput = screen.getByLabelText('Type');
  fireEvent.change(typeInput, { target: { value: 'Cabin' } });
  expect(last()?.[0].type).toBe('Cabin');

  fireEvent.change(typeInput, { target: { value: '' } });
  fireEvent.keyDown(typeInput, { key: 'ArrowDown' });
  const listbox = screen.getByRole('listbox');
  fireEvent.click(within(listbox).getAllByRole('option')[0]);
  const picked = last()?.[0].type;
  expect(picked).toBeTruthy();
  expect(picked).toBe(picked?.trim());
});

// --- ADR 0031 / issue #1286: coordinates + "find coordinates" ---------------

const geocodeMock = vi.hoisted(() => vi.fn());
beforeEach(() => geocodeMock.mockReset());
vi.mock('../api/map', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/map')>()),
  geocodeAddress: geocodeMock,
}));

function renderWithContact(
  initial: ContactAddress[],
  { contactId }: { contactId?: number } = { contactId: 7 },
) {
  const onChange = vi.fn<(next: ContactAddress[]) => void>();
  function H() {
    const [value, setValue] = useState(initial);
    return (
      <AddressFields
        label="Addresses"
        value={value}
        contactId={contactId}
        onChange={(next) => {
          onChange(next);
          setValue(next);
        }}
      />
    );
  }
  render(<H />);
  return { last: () => onChange.mock.calls.at(-1)?.[0] };
}

const findButton = () => screen.getByRole('button', { name: 'Find coordinates' });
const coordField = () => screen.getByLabelText('Coordinates (latitude, longitude)');

test('shows stored coordinates as a readable "lat, lng" pair', () => {
  renderWithContact([addr({ id: 'a', coordinates: 'geo:51.5007,-0.1246' })]);
  expect(coordField()).toHaveValue('51.5007, -0.1246');
});

test('shows an unparseable stored value verbatim rather than hiding it', () => {
  renderWithContact([addr({ id: 'a', coordinates: 'geo:nonsense' })]);
  expect(coordField()).toHaveValue('geo:nonsense');
  expect(screen.getByText(/Enter latitude/)).toBeInTheDocument();
});

test('a valid typed pair is committed as a geo: URI', () => {
  const { last } = renderWithContact([addr({ id: 'a' })]);
  fireEvent.change(coordField(), { target: { value: '12.5, -45.25' } });
  expect(last()?.[0].coordinates).toBe('geo:12.5,-45.25');
  expect(coordField()).toHaveValue('12.5, -45.25');
});

test('an invalid or out-of-range pair is flagged and not committed', () => {
  const { last } = renderWithContact([addr({ id: 'a', coordinates: 'geo:1,2' })]);
  fireEvent.change(coordField(), { target: { value: '95, 10' } });
  expect(screen.getByText(/Enter latitude/)).toBeInTheDocument();
  expect(coordField()).toHaveValue('95, 10');
  // nothing committed for the bad text: the previous value stays
  expect(last()).toBeUndefined();
});

test('clearing the field removes the coordinates', () => {
  const { last } = renderWithContact([addr({ id: 'a', coordinates: 'geo:1,2' })]);
  fireEvent.change(coordField(), { target: { value: '' } });
  expect(last()?.[0].coordinates).toBeUndefined();
  expect(screen.queryByText(/Enter latitude/)).not.toBeInTheDocument();
});

test('find coordinates geocodes the saved address and fills the field', async () => {
  geocodeMock.mockResolvedValueOnce({
    address_id: 'a',
    coordinates: 'geo:48.85,2.35',
    cached: false,
  });
  const { last } = renderWithContact([addr({ id: 'a', street: '1 Rue' })]);
  fireEvent.click(findButton());
  await waitFor(() => expect(last()?.[0].coordinates).toBe('geo:48.85,2.35'));
  expect(geocodeMock).toHaveBeenCalledWith(7, 'a');
  expect(coordField()).toHaveValue('48.85, 2.35');
});

test('find coordinates overrides a half-typed draft with the result', async () => {
  geocodeMock.mockResolvedValueOnce({ address_id: 'a', coordinates: 'geo:1,2', cached: true });
  renderWithContact([addr({ id: 'a' })]);
  fireEvent.change(coordField(), { target: { value: '9,' } });
  fireEvent.click(findButton());
  await waitFor(() => expect(coordField()).toHaveValue('1, 2'));
});

test('is disabled while a lookup is in flight', async () => {
  let resolve!: (v: unknown) => void;
  geocodeMock.mockReturnValueOnce(new Promise((r) => (resolve = r)));
  renderWithContact([addr({ id: 'a' })]);
  fireEvent.click(findButton());
  await waitFor(() => expect(findButton()).toBeDisabled());
  resolve({ address_id: 'a', coordinates: 'geo:1,2', cached: false });
  await waitFor(() => expect(findButton()).toBeEnabled());
});

test('a failed lookup shows the error and leaves coordinates alone, then clears on retry', async () => {
  geocodeMock.mockRejectedValueOnce(new Error('the provider could not be reached'));
  const { last } = renderWithContact([addr({ id: 'a' })]);
  fireEvent.click(findButton());
  expect(await screen.findByRole('alert')).toHaveTextContent('the provider could not be reached');
  expect(last()).toBeUndefined();
  expect(findButton()).toBeEnabled();

  geocodeMock.mockResolvedValueOnce({ address_id: 'a', coordinates: 'geo:1,2', cached: false });
  fireEvent.click(findButton());
  await waitFor(() => expect(screen.queryByRole('alert')).not.toBeInTheDocument());
});

test('a failure with no message falls back to a generic one', async () => {
  geocodeMock.mockRejectedValueOnce(new Error(''));
  renderWithContact([addr({ id: 'a' })]);
  fireEvent.click(findButton());
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Could not find coordinates for this address.',
  );
});

test.each(['private', 'secret'] as const)(
  'blocks find coordinates for a %s address with a visible reason',
  (sensitivity) => {
    renderWithContact([addr({ id: 'a', sensitivity })]);
    expect(findButton()).toBeDisabled();
    expect(screen.getByText(/never sent to the geocoder/)).toBeInTheDocument();
    fireEvent.click(findButton());
    expect(geocodeMock).not.toHaveBeenCalled();
    // manual entry still works for a sensitive address
    fireEvent.change(coordField(), { target: { value: '1, 2' } });
    expect(coordField()).toHaveValue('1, 2');
  },
);

test('a normal-sensitivity address is not blocked', () => {
  renderWithContact([addr({ id: 'a', sensitivity: 'normal' })]);
  expect(findButton()).toBeEnabled();
  expect(screen.queryByText(/never sent to the geocoder/)).not.toBeInTheDocument();
});

test('an unsaved address (no id) asks to save first', () => {
  renderWithContact([addr()]);
  expect(findButton()).toBeDisabled();
  expect(screen.getByText(/Save this address first/)).toBeInTheDocument();
});

test('without a contact id (creating) the action is disabled with the save-first reason', () => {
  renderWithContact([addr({ id: 'a' })], {});
  expect(findButton()).toBeDisabled();
  expect(screen.getByText(/Save this address first/)).toBeInTheDocument();
});

test('sensitivity reason takes precedence over save-first', () => {
  renderWithContact([addr({ sensitivity: 'private' })]);
  expect(screen.getByText(/never sent to the geocoder/)).toBeInTheDocument();
  expect(screen.queryByText(/Save this address first/)).not.toBeInTheDocument();
});
