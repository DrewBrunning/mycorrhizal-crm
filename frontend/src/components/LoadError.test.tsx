import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import LoadError from './LoadError';

afterEach(cleanup);

test('renders an announced error alert with the message and title', () => {
  render(<LoadError message="Server exploded (ref: abc123)" />);

  const alert = screen.getByRole('alert');
  expect(alert).toHaveTextContent("Couldn't load this data");
  expect(alert).toHaveTextContent('Server exploded (ref: abc123)');
});

test('shows a Retry button that calls onRetry once', () => {
  const onRetry = vi.fn();
  render(<LoadError message="boom" onRetry={onRetry} />);

  fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
  expect(onRetry).toHaveBeenCalledTimes(1);
});

test('omits the Retry button when no onRetry is given', () => {
  render(<LoadError message="boom" />);

  expect(screen.queryByRole('button', { name: 'Retry' })).not.toBeInTheDocument();
});
