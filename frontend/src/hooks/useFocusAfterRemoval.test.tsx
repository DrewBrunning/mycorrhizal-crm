import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { useEffect, useRef, useState } from 'react';
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

// #2920 flake: an action taken after the rows were committed but before that
// commit's passive effect flushed was mistaken for the removal itself.
test('a request made before the load effect flushes waits for the real removal', async () => {
  function Loaded() {
    const [rows, setRows] = useState<string[]>([]);
    const heading = useRef<HTMLHeadingElement>(null);
    const focus = useFocusAfterRemoval(heading, rows);
    useEffect(() => {
      void Promise.resolve().then(() => setRows(['a', 'b']));
    }, []);
    return (
      <div>
        <h2 ref={heading} tabIndex={-1}>
          Rows
        </h2>
        {rows.map((id) => (
          <button
            type="button"
            key={id}
            ref={focus.register(id)}
            // The removal is still in flight: request, but no list change yet.
            onClick={() => focus.request(rows, id)}
          >
            remove {id}
          </button>
        ))}
        <button type="button" onClick={() => setRows((r) => r.filter((x) => x !== 'a'))}>
          finish
        </button>
      </div>
    );
  }
  let clicked: HTMLElement | null = null;
  // Runs in the microtask after the commit, before the scheduler flushes the
  // commit's passive effects.
  const observer = new MutationObserver(() => {
    const a = screen.queryByText('remove a');
    if (clicked || !a) return;
    clicked = a;
    a.focus();
    fireEvent.click(a);
  });
  observer.observe(document.body, { childList: true, subtree: true });
  try {
    render(<Loaded />);
    await waitFor(() => expect(clicked).not.toBeNull());
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });
  } finally {
    observer.disconnect();
  }
  expect(document.activeElement).toBe(clicked);
  fireEvent.click(screen.getByText('finish'));
  expect(document.activeElement).toBe(screen.getByText('remove b'));
});
