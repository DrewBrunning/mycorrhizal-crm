import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, expect, test } from 'vitest';
import '../../i18n/config';
import type { SourceImportStatus } from '../../api/sourceImport';
import SourceImportProgress from './SourceImportProgress';

afterEach(cleanup);

function status(overrides: Partial<SourceImportStatus>): SourceImportStatus {
  return {
    session_id: 's1',
    phase: 'building_preview',
    phase_done: 0,
    phase_total: 0,
    ...overrides,
  };
}

test('shows a determinate bar with a count when a total is known', () => {
  render(
    <SourceImportProgress
      status={status({ phase: 'importing', phase_done: 3, phase_total: 12 })}
      sourceLabel="Monica"
    />,
  );
  const bar = screen.getByRole('progressbar');
  expect(bar).toHaveAttribute('aria-valuenow', '25');
  expect(screen.getByText('3 of 12')).toBeInTheDocument();
  // The "you can leave" reassurance is always shown.
  expect(screen.getByText(/keeps running on the server/i)).toBeInTheDocument();
});

test('falls back to an indeterminate bar and interpolates the source name', () => {
  render(
    <SourceImportProgress
      status={status({ phase: 'connecting', phase_total: 0 })}
      sourceLabel="Monica"
    />,
  );
  expect(screen.getByText('Connecting to Monica…')).toBeInTheDocument();
  expect(screen.queryByText(/ of /)).not.toBeInTheDocument();
});
