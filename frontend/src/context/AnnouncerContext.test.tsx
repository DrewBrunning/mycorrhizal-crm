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
  expect(container.ownerDocument.querySelector('[aria-live="polite"]')).toHaveTextContent(
    '3 results',
  );
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
  const region = document.querySelector('[aria-live="polite"]') as HTMLElement;
  const style = getComputedStyle(region);
  expect(style.width).toBe('1px');
  expect(style.height).toBe('1px');
});
