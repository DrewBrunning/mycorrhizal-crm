import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, expect, test } from 'vitest';
import '../../i18n/config';
import type { SourceImportResult as Result } from '../../api/sourceImport';
import SourceImportResult from './SourceImportResult';

afterEach(cleanup);

function result(overrides: Partial<Result>): Result {
  return {
    total_processed: 0,
    created: 0,
    updated: 0,
    skipped: 0,
    errors: [],
    relationships_created: 0,
    notes_created: 0,
    activities_created: 0,
    reminders_created: 0,
    gifts_created: 0,
    custom_fields_created: 0,
    photos_queued: 0,
    photos_saved: 0,
    photos_failed: 0,
    ...overrides,
  };
}

test('summarises counts and lists named partial failures', () => {
  render(
    <SourceImportResult
      result={result({
        created: 4,
        updated: 1,
        skipped: 2,
        relationships_created: 3,
        errors: ['monica contact/7 (name): contact has no usable name'],
      })}
    />,
  );
  expect(screen.getByText('4 added, 1 merged, 2 skipped')).toBeInTheDocument();
  expect(screen.getByText('3 relationships')).toBeInTheDocument();
  expect(screen.getByText(/1 records could not be imported/)).toBeInTheDocument();
  expect(screen.getByText(/contact has no usable name/)).toBeInTheDocument();
});

test('shows photo progress while pending and the tally when done', () => {
  const { rerender } = render(
    <SourceImportResult result={result({ photos_queued: 5 })} photosPending />,
  );
  expect(screen.getByText(/Downloading 5 profile photos/)).toBeInTheDocument();

  rerender(
    <SourceImportResult result={result({ photos_queued: 5, photos_saved: 4, photos_failed: 1 })} />,
  );
  expect(screen.getByText('4 photos saved, 1 failed')).toBeInTheDocument();
});
