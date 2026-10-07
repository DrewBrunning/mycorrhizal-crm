import { cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { useState } from 'react';
import { afterEach, describe, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { CardAnniversary } from '../api/contacts';
import AnniversariesEditor from './AnniversariesEditor';

afterEach(cleanup);

// Controlled wrapper so edits round-trip through value -> render -> onChange,
// and the last payload the parent would save is observable.
function Harness({
  initial,
  onChange,
}: {
  initial: CardAnniversary[];
  onChange: (next: CardAnniversary[]) => void;
}) {
  const [value, setValue] = useState(initial);
  return (
    <AnniversariesEditor
      label="Anniversaries"
      value={value}
      onChange={(next) => {
        setValue(next);
        onChange(next);
      }}
    />
  );
}

const lastCall = (fn: ReturnType<typeof vi.fn>) => fn.mock.calls[fn.mock.calls.length - 1][0];

describe('AnniversariesEditor', () => {
  test('renders the label and an Add button when empty', () => {
    render(<AnniversariesEditor label="Anniversaries" value={[]} onChange={vi.fn()} />);
    expect(screen.getByText('Anniversaries')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Add' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Delete' })).not.toBeInTheDocument();
  });

  test('Add appends an empty wedding row (a fresh object each time)', () => {
    const onChange = vi.fn();
    render(<Harness initial={[]} onChange={onChange} />);
    fireEvent.click(screen.getByRole('button', { name: 'Add' }));
    fireEvent.click(screen.getByRole('button', { name: 'Add' }));
    const next = lastCall(onChange) as CardAnniversary[];
    expect(next).toEqual([
      { kind: 'wedding', date: {} },
      { kind: 'wedding', date: {} },
    ]);
    expect(next[0]).not.toBe(next[1]);
    expect(screen.getAllByRole('button', { name: 'Delete' })).toHaveLength(2);
  });

  test('typing a full date saves a partial {year, month, day}', () => {
    const onChange = vi.fn();
    render(<Harness initial={[{ kind: 'wedding', date: {} }]} onChange={onChange} />);
    fireEvent.change(screen.getByPlaceholderText('--MM-DD'), { target: { value: '2010-06-05' } });
    expect(lastCall(onChange)).toEqual([
      { kind: 'wedding', date: { partial: { year: 2010, month: 6, day: 5 } } },
    ]);
    expect(screen.getByDisplayValue('2010-06-05')).toBeInTheDocument();
  });

  test('typing a year-less date saves {month, day} with no year', () => {
    const onChange = vi.fn();
    render(<Harness initial={[{ kind: 'birth', date: {} }]} onChange={onChange} />);
    fireEvent.change(screen.getByPlaceholderText('--MM-DD'), { target: { value: '--03-09' } });
    expect(lastCall(onChange)).toEqual([
      { kind: 'birth', date: { partial: { month: 3, day: 9 } } },
    ]);
    expect(screen.getByDisplayValue('--03-09')).toBeInTheDocument();
  });

  test('unparseable input is kept as a raw timestamp rather than dropped', () => {
    const onChange = vi.fn();
    render(<Harness initial={[{ kind: 'death', date: {} }]} onChange={onChange} />);
    fireEvent.change(screen.getByPlaceholderText('--MM-DD'), { target: { value: '2010-06' } });
    expect(lastCall(onChange)).toEqual([{ kind: 'death', date: { timestamp: '2010-06' } }]);
  });

  test('a partial missing day (or month) displays as empty so the user cannot see half a date', () => {
    render(
      <AnniversariesEditor
        label="A"
        value={[
          { kind: 'birth', date: { partial: { year: 1990, month: 4 } } },
          { kind: 'birth', date: { partial: { year: 1990, day: 4 } } },
        ]}
        onChange={vi.fn()}
      />,
    );
    const inputs = screen.getAllByPlaceholderText('--MM-DD') as HTMLInputElement[];
    expect(inputs.map((i) => i.value)).toEqual(['', '']);
  });

  test('displays zero-padded full and year-less partials and a trimmed timestamp', () => {
    render(
      <AnniversariesEditor
        label="A"
        value={[
          { kind: 'wedding', date: { partial: { year: 2001, month: 2, day: 3 } } },
          { kind: 'wedding', date: { partial: { month: 12, day: 25 } } },
          { kind: 'wedding', date: { timestamp: '2005-07-08T10:00:00Z' } },
        ]}
        onChange={vi.fn()}
      />,
    );
    const inputs = screen.getAllByPlaceholderText('--MM-DD') as HTMLInputElement[];
    expect(inputs.map((i) => i.value)).toEqual(['2001-02-03', '--12-25', '2005-07-08']);
  });

  test('changing the kind saves the new kind and keeps the date', async () => {
    const onChange = vi.fn();
    render(
      <Harness
        initial={[{ kind: 'wedding', date: { partial: { month: 1, day: 2 } } }]}
        onChange={onChange}
      />,
    );
    fireEvent.mouseDown(screen.getByRole('combobox'));
    fireEvent.click(await screen.findByRole('option', { name: /death/i }));
    expect(lastCall(onChange)).toEqual([
      { kind: 'death', date: { partial: { month: 1, day: 2 } } },
    ]);
  });

  test('every kind option is offered', async () => {
    render(
      <AnniversariesEditor label="A" value={[{ kind: 'birth', date: {} }]} onChange={vi.fn()} />,
    );
    fireEvent.mouseDown(screen.getByRole('combobox'));
    const list = await screen.findByRole('listbox');
    expect(within(list).getAllByRole('option')).toHaveLength(3);
  });

  test('Delete removes only that row and keeps the others', () => {
    const onChange = vi.fn();
    const rows: CardAnniversary[] = [
      { kind: 'birth', date: { partial: { month: 1, day: 1 } } },
      { kind: 'wedding', date: { partial: { month: 2, day: 2 } } },
      { kind: 'death', date: { partial: { month: 3, day: 3 } } },
    ];
    render(<Harness initial={rows} onChange={onChange} />);
    fireEvent.click(screen.getAllByRole('button', { name: 'Delete' })[1]);
    expect(lastCall(onChange)).toEqual([rows[0], rows[2]]);
    expect(screen.getAllByPlaceholderText('--MM-DD')).toHaveLength(2);
    expect(screen.queryByDisplayValue('--02-02')).not.toBeInTheDocument();
    expect(screen.getByDisplayValue('--03-03')).toBeInTheDocument();
  });

  test('editing one row leaves its siblings untouched', () => {
    const onChange = vi.fn();
    const rows: CardAnniversary[] = [
      { kind: 'birth', date: { partial: { month: 1, day: 1 } } },
      { kind: 'wedding', date: { partial: { month: 2, day: 2 } } },
    ];
    render(<Harness initial={rows} onChange={onChange} />);
    fireEvent.change(screen.getAllByPlaceholderText('--MM-DD')[1], {
      target: { value: '--09-09' },
    });
    expect(lastCall(onChange)).toEqual([
      rows[0],
      { kind: 'wedding', date: { partial: { month: 9, day: 9 } } },
    ]);
  });

  test('shows a read-only place line: full text, else joined components, else nothing', () => {
    render(
      <AnniversariesEditor
        label="A"
        value={[
          { kind: 'wedding', date: {}, place: { full: 'Paris, France' } },
          {
            kind: 'wedding',
            date: {},
            place: {
              components: [
                { kind: 'locality', value: 'Rome' },
                { kind: 'country', value: '' },
                { kind: 'country', value: 'Italy' },
              ],
            },
          },
          { kind: 'wedding', date: {}, place: {} },
        ]}
        onChange={vi.fn()}
      />,
    );
    expect(screen.getByText('Paris, France')).toBeInTheDocument();
    expect(screen.getByText('Rome, Italy')).toBeInTheDocument();
  });
});
