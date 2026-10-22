import {
  act,
  cleanup,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
} from '@testing-library/react';
import { useState } from 'react';
import { createMemoryRouter, Link, RouterProvider } from 'react-router';
import { afterEach, describe, expect, test, vi } from 'vitest';
import '../i18n/config';
import { useDirtyDialog } from './useDirtyDialog';

afterEach(cleanup);

describe('useDirtyDialog', () => {
  test('markDirty arms the guard and clearDirty disarms it', () => {
    const { result } = renderHook(() => useDirtyDialog(true, vi.fn()));

    expect(result.current.isDirty).toBe(false);

    act(() => result.current.markDirty());
    expect(result.current.isDirty).toBe(true);

    act(() => result.current.clearDirty());
    expect(result.current.isDirty).toBe(false);
  });

  test('reopening the dialog starts clean again', () => {
    const { result, rerender } = renderHook(({ open }) => useDirtyDialog(open, vi.fn()), {
      initialProps: { open: true },
    });

    act(() => result.current.markDirty());
    expect(result.current.isDirty).toBe(true);

    rerender({ open: false });
    rerender({ open: true });

    expect(result.current.isDirty).toBe(false);
  });

  test('requestClose closes immediately when clean and runs the real close handler', () => {
    const onClose = vi.fn();
    const { result } = renderHook(() => useDirtyDialog(true, onClose));

    act(() => result.current.requestClose());

    expect(onClose).toHaveBeenCalledTimes(1);
    expect(result.current.isDirty).toBe(false);
  });

  test('requestClose does not close while dirty', () => {
    const onClose = vi.fn();
    const { result } = renderHook(() => useDirtyDialog(true, onClose));

    act(() => result.current.markDirty());
    act(() => result.current.requestClose());

    expect(onClose).not.toHaveBeenCalled();
    expect(result.current.isDirty).toBe(true);
  });
});

// A custom onNavigationDiscard lets a dialog do extra cleanup for a route
// change while a plain Cancel/Escape still runs its own close handler.
function NavigationHarness({
  onClose,
  onNavigationDiscard,
}: {
  onClose: () => void;
  onNavigationDiscard: () => void;
}) {
  const [open, setOpen] = useState(true);
  const { markDirty, requestClose, guardElement } = useDirtyDialog(
    open,
    () => {
      setOpen(false);
      onClose();
    },
    { onNavigationDiscard },
  );

  return (
    <div>
      {guardElement}
      <Link to="/away">leave</Link>
      <button type="button" onClick={markDirty}>
        make dirty
      </button>
      <button type="button" onClick={requestClose}>
        close
      </button>
      {open ? <div>dialog body</div> : null}
    </div>
  );
}

describe('useDirtyDialog navigation discard option', () => {
  test('a custom onNavigationDiscard runs instead of the close handler', async () => {
    const onClose = vi.fn();
    const onNavigationDiscard = vi.fn();
    const router = createMemoryRouter(
      [
        {
          path: '/home',
          element: (
            <NavigationHarness onClose={onClose} onNavigationDiscard={onNavigationDiscard} />
          ),
        },
        { path: '/away', element: <div>away page</div> },
      ],
      { initialEntries: ['/home'] },
    );
    render(<RouterProvider router={router} />);

    fireEvent.click(screen.getByText('make dirty'));
    await act(async () => {
      await router.navigate('/away');
    });
    expect(screen.getByText('Discard unsaved changes?')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Discard' }));

    await waitFor(() => expect(router.state.location.pathname).toBe('/away'));
    expect(onNavigationDiscard).toHaveBeenCalledTimes(1);
    expect(onClose).not.toHaveBeenCalled();
  });

  test('without the option, a navigation discard falls back to the close handler', async () => {
    const onClose = vi.fn();
    const router = createMemoryRouter(
      [
        { path: '/home', element: <FallbackHarness onClose={onClose} /> },
        { path: '/away', element: <div>away page</div> },
      ],
      { initialEntries: ['/home'] },
    );
    render(<RouterProvider router={router} />);

    fireEvent.click(screen.getByText('make dirty'));
    await act(async () => {
      await router.navigate('/away');
    });
    expect(screen.getByText('Discard unsaved changes?')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Discard' }));

    await waitFor(() => expect(router.state.location.pathname).toBe('/away'));
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});

function FallbackHarness({ onClose }: { onClose: () => void }) {
  const { markDirty, guardElement } = useDirtyDialog(true, onClose);
  return (
    <div>
      {guardElement}
      <button type="button" onClick={markDirty}>
        make dirty
      </button>
    </div>
  );
}
