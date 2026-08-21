import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import ErrorFallback, { SectionErrorFallback } from './ErrorFallback';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(cleanup);

test('renders the default title and message', () => {
  render(<ErrorFallback />);

  expect(screen.getByText('Something went wrong')).toBeInTheDocument();
  expect(screen.getByText('An unexpected error occurred. Please try again.')).toBeInTheDocument();
});

test('renders custom title and message when provided', () => {
  render(<ErrorFallback title="Failed to load contacts" message="The contacts list is unavailable." />);

  expect(screen.getByText('Failed to load contacts')).toBeInTheDocument();
  expect(screen.getByText('The contacts list is unavailable.')).toBeInTheDocument();
});

test('renders a Try Again button only when onReset is provided and calls it', () => {
  const onReset = vi.fn();

  const { unmount } = render(<ErrorFallback onReset={onReset} />);
  fireEvent.click(screen.getByRole('button', { name: 'Try Again' }));
  expect(onReset).toHaveBeenCalledTimes(1);

  unmount();
  render(<ErrorFallback />);
  expect(screen.queryByRole('button', { name: 'Try Again' })).not.toBeInTheDocument();
});

test('shows technical details when showDetails and an error are provided', () => {
  const error = new Error('detail boom');
  error.stack = 'at componentStack line 1';

  render(<ErrorFallback error={error} showDetails />);

  expect(screen.getByText('Technical Details')).toBeInTheDocument();
  expect(screen.getByText(/detail boom/)).toBeInTheDocument();
  expect(screen.getByText(/at componentStack line 1/)).toBeInTheDocument();
});

test('omits technical details when there is no error', () => {
  render(<ErrorFallback showDetails />);

  expect(screen.queryByText('Technical Details')).not.toBeInTheDocument();
});

test('SectionErrorFallback renders its title and message with a Retry action', () => {
  const onReset = vi.fn();

  render(<SectionErrorFallback title="Error loading section" error={new Error('fetch failed')} onReset={onReset} />);

  expect(screen.getByText('Error loading section')).toBeInTheDocument();
  expect(screen.getByText('fetch failed')).toBeInTheDocument();

  fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
  expect(onReset).toHaveBeenCalledTimes(1);
});

test('SectionErrorFallback shows a custom message and defaults to the generic error copy', () => {
  render(<SectionErrorFallback message="Custom failure" />);
  expect(screen.getByText('Custom failure')).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Retry' })).not.toBeInTheDocument();

  cleanup();
  render(<SectionErrorFallback />);
  expect(screen.getByText('An unexpected error occurred.')).toBeInTheDocument();
});
