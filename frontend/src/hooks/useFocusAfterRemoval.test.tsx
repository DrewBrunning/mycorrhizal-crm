import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { useRef, useState } from 'react';
import { afterEach, expect, test } from 'vitest';
import { useFocusAfterRemoval } from './useFocusAfterRemoval';

afterEach(cleanup);

function List({ failRemoval = false }: { failRemoval?: boolean }) {
  const [rows, setRows] = useState(['a', 'b', 'c']);
  const heading = useRef<HTMLHeadingElement>(null);
  const focus = useFocusAfterRemoval(heading, rows);
  const remove = (id: string) => {
    focus.request(rows, id);
    if (failRemoval) {
      focus.cancel();
      return;
    }
    setRows((r) => r.filter((x) => x !== id));
  };
  return (
    <div>
      <h2 ref={heading} tabIndex={-1}>
        Rows
      </h2>
      <button type="button" onClick={() => setRows((r) => [...r])}>
        refresh
      </button>
      {rows.map((id) => (
        <button type="button" key={id} ref={focus.register(id)} onClick={() => remove(id)}>
          remove {id}
        </button>
      ))}
    </div>
  );
}

test('focus moves to the next row, then the previous, then the fallback', () => {
  render(<List />);
  fireEvent.click(screen.getByText('remove a'));
  expect(document.activeElement).toBe(screen.getByText('remove b'));
  fireEvent.click(screen.getByText('remove c'));
  expect(document.activeElement).toBe(screen.getByText('remove b'));
  fireEvent.click(screen.getByText('remove b'));
  expect(document.activeElement).toBe(screen.getByRole('heading', { name: 'Rows' }));
});

test('a cancelled request does not steal focus on a later change', () => {
  render(<List failRemoval />);
  fireEvent.click(screen.getByText('remove a'));
  screen.getByText('refresh').focus();
  fireEvent.click(screen.getByText('refresh'));
  expect(document.activeElement).toBe(screen.getByText('refresh'));
});

test('a request waits until the list is ready again', () => {
  function Reloading() {
    const [rows, setRows] = useState(['a', 'b']);
    const [loading, setLoading] = useState(false);
    const heading = useRef<HTMLHeadingElement>(null);
    const focus = useFocusAfterRemoval(heading, rows, !loading);
    return (
      <div>
        <h2 ref={heading} tabIndex={-1}>
          Rows
        </h2>
        <button
          type="button"
          onClick={() => {
            focus.request(rows, 'a');
            setLoading(true);
            setRows([]);
          }}
        >
          start
        </button>
        <button
          type="button"
          onClick={() => {
            setRows(['b']);
            setLoading(false);
          }}
        >
          finish
        </button>
        {rows.map((id) => (
          <button type="button" key={id} ref={focus.register(id)}>
            row {id}
          </button>
        ))}
      </div>
    );
  }
  render(<Reloading />);
  fireEvent.click(screen.getByText('start'));
  expect(document.activeElement).not.toBe(screen.getByRole('heading', { name: 'Rows' }));
  fireEvent.click(screen.getByText('finish'));
  expect(document.activeElement).toBe(screen.getByText('row b'));
});
