import { act, cleanup, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import { AnnouncerProvider, useAnnouncer } from './AnnouncerContext';

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

function Probe({ message }: { message: string }) {
  const { announce } = useAnnouncer();
  return (
    <button type="button" onClick={() => announce(message)}>
      go
    </button>
  );
}

test('announce() lands in the polite live region after its debounce', () => {
  vi.useFakeTimers();
  const { container } = render(
    <AnnouncerProvider>
      <Probe message="3 results" />
    </AnnouncerProvider>,
  );
  act(() => {
    screen.getByText('go').click();
  });
  act(() => {
    vi.advanceTimersByTime(60);
  });
  const region = screen.getByRole('status');
  expect(region).toHaveAttribute('aria-live', 'polite');
  expect(region).toHaveAttribute('aria-atomic', 'true');
  expect(region).toHaveTextContent('3 results');
  expect(container.ownerDocument.querySelectorAll('[aria-live="polite"]')).toHaveLength(1);
});

// #1918: Orca exposed this region in the AX tree but never spoke it. A stable
// role="status" region must exist from the first render -- creating the region
// together with the message is the classic live-region bug.
test('the status live region exists before anything is announced', () => {
  render(
    <AnnouncerProvider>
      <Probe message="x" />
    </AnnouncerProvider>,
  );

  const region = screen.getByRole('status');
  expect(region).toBeEmptyDOMElement();
  expect(region).toHaveAttribute('aria-live', 'polite');
});

// The hidden technique must keep the node in the accessibility tree: display:none
// or visibility:hidden would make the role="status" region unannounceable.
test('the status live region is clipped, not removed from the accessibility tree', () => {
  render(
    <AnnouncerProvider>
      <Probe message="x" />
    </AnnouncerProvider>,
  );

  const style = getComputedStyle(screen.getByRole('status'));
  expect(style.display).not.toBe('none');
  expect(style.visibility).not.toBe('hidden');
  expect(style.width).toBe('1px');
  expect(style.height).toBe('1px');
});

// MUI's sx reads a bare `1` as 100%, so `width: 1; height: 1` made this
// "visually hidden" region as large as its containing block. Absolutely
// positioned under the app, it stretched every page's scroll height by a
// viewport of blank space (and left the fixed sidebar looking cut off in
// full-page captures, #1788). It must be a literal 1px box.
test('the live region is a 1px box, not 100% of its containing block (#1788)', () => {
  render(
    <AnnouncerProvider>
      <Probe message="x" />
    </AnnouncerProvider>,
  );
  const region = screen.getByRole('status');
  const style = getComputedStyle(region);
  expect(style.width).toBe('1px');
  expect(style.height).toBe('1px');
});
