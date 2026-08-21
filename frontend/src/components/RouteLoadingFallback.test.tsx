import { test, expect, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import '../i18n/config';
import RouteLoadingFallback from './RouteLoadingFallback';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(cleanup);

test('renders an accessible loading status with the translated label', () => {
  render(<RouteLoadingFallback />);

  const status = screen.getByRole('status');
  expect(status).toHaveAccessibleName('Loading…');
  expect(screen.getByRole('progressbar')).toBeInTheDocument();
});
