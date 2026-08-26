import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import ErrorBoundary from './ErrorBoundary';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// A child that throws on render, controlled by a prop, so a test can
// exercise capture and then recovery without re-rendering the boundary.
function Bomb({ shouldThrow }: { shouldThrow: boolean }) {
  if (shouldThrow) {
    throw new Error('boom');
  }
  return <div>healthy content</div>;
}

function stubLocation() {
  Object.defineProperty(window, 'location', {
    writable: true,
    configurable: true,
    value: { href: 'http://localhost/', reload: vi.fn(), assign: vi.fn() },
  });
  return window.location as Location & { reload: ReturnType<typeof vi.fn> };
}

beforeEach(() => {
  // React logs every caught render error; the boundary also logs in
  // componentDidCatch. Neither should leak into the test output.
  vi.spyOn(console, 'error').mockImplementation(() => {});
});

test('renders its children when nothing throws', () => {
  render(
    <ErrorBoundary name="ContactsList">
      <Bomb shouldThrow={false} />
    </ErrorBoundary>,
  );

  expect(screen.getByText('healthy content')).toBeInTheDocument();
  expect(screen.queryByText(/Oops! Something went wrong/)).not.toBeInTheDocument();
});

test('captures a render error, renders the fallback UI, and reports it via onError', () => {
  const onError = vi.fn();

  render(
    <ErrorBoundary name="PrepView" onError={onError}>
      <Bomb shouldThrow />
    </ErrorBoundary>,
  );

  expect(screen.getByText('Oops! Something went wrong')).toBeInTheDocument();
  expect(screen.getByText(/We encountered an unexpected error in PrepView/)).toBeInTheDocument();
  expect(onError).toHaveBeenCalledTimes(1);
  expect(onError.mock.calls[0][0]).toBeInstanceOf(Error);
  expect(onError.mock.calls[0][0].message).toBe('boom');
  expect(screen.queryByText('healthy content')).not.toBeInTheDocument();
});

test('renders a custom fallback when one is provided', () => {
  render(
    <ErrorBoundary fallback={<div>custom fallback</div>}>
      <Bomb shouldThrow />
    </ErrorBoundary>,
  );

  expect(screen.getByText('custom fallback')).toBeInTheDocument();
  expect(screen.queryByText('Oops! Something went wrong')).not.toBeInTheDocument();
});

test('shows the error details when showDetails is enabled', () => {
  render(
    <ErrorBoundary showDetails name="PrepView" errorMessage="Failed to load the prep view">
      <Bomb shouldThrow />
    </ErrorBoundary>,
  );

  expect(screen.getByText('Error Details (Development Only)')).toBeInTheDocument();
  expect(screen.getByText(/Error: boom/)).toBeInTheDocument();
  expect(screen.getByText('Failed to load the prep view')).toBeInTheDocument();
});

test('Try Again resets the boundary and re-renders children that stopped throwing', () => {
  const { rerender } = render(
    <ErrorBoundary name="PrepView">
      <Bomb shouldThrow />
    </ErrorBoundary>,
  );
  expect(screen.getByText('Oops! Something went wrong')).toBeInTheDocument();

  // A plain re-render with fixed children does NOT clear the boundary state.
  rerender(
    <ErrorBoundary name="PrepView">
      <Bomb shouldThrow={false} />
    </ErrorBoundary>,
  );
  expect(screen.getByText('Oops! Something went wrong')).toBeInTheDocument();

  fireEvent.click(screen.getByRole('button', { name: 'Try Again' }));
  expect(screen.getByText('healthy content')).toBeInTheDocument();
  expect(screen.queryByText('Oops! Something went wrong')).not.toBeInTheDocument();
});

test('remounting the boundary with a fresh key restores normal rendering without a manual reset', () => {
  const { rerender } = render(
    <ErrorBoundary name="PrepView">
      <Bomb shouldThrow />
    </ErrorBoundary>,
  );
  expect(screen.getByText('Oops! Something went wrong')).toBeInTheDocument();

  rerender(
    <ErrorBoundary key="fresh-boundary" name="PrepView">
      <Bomb shouldThrow={false} />
    </ErrorBoundary>,
  );

  expect(screen.getByText('healthy content')).toBeInTheDocument();
  expect(screen.queryByText('Oops! Something went wrong')).not.toBeInTheDocument();
});

test('Reload Page reloads the page and Go Home navigates to the root', () => {
  const location = stubLocation();

  render(
    <ErrorBoundary>
      <Bomb shouldThrow />
    </ErrorBoundary>,
  );

  fireEvent.click(screen.getByRole('button', { name: 'Reload Page' }));
  expect(location.reload).toHaveBeenCalledTimes(1);

  fireEvent.click(screen.getByRole('button', { name: 'Go Home' }));
  expect(location.href).toBe('/');
});
