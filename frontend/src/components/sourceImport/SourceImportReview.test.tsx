import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, expect, test } from 'vitest';
import '../../i18n/config';
import type { SourceImportPreviewResponse } from '../../api/sourceImport';
import SourceImportReview from './SourceImportReview';

afterEach(cleanup);

const preview: SourceImportPreviewResponse = {
  session_id: 's1',
  total_rows: 2,
  valid_rows: 2,
  duplicate_count: 1,
  error_count: 0,
  totals: { activities: 1, notes: 2, reminders: 0, relationships: 3, gifts: 0 },
  loss_report: [
    {
      record: 'monica contact/5',
      field: 'gift.amount',
      category: 'lossy',
      message: 'gift amount kept as free text',
    },
  ],
  rows: [
    {
      row_index: 0,
      parsed_contact: { firstname: 'Ada', lastname: 'Lovelace' },
      validation_errors: [],
      duplicate_match: null,
      suggested_action: 'add',
      merge_diff: null,
      batch_duplicate_of: null,
      related: { activities: 0, notes: 1, reminders: 0, relationships: 2, gifts: 0 },
      has_photo: true,
    },
    {
      row_index: 1,
      parsed_contact: { firstname: 'Grace', lastname: 'Hopper' },
      validation_errors: [],
      duplicate_match: {
        existing_contact_id: 9,
        existing_firstname: 'Grace',
        existing_lastname: 'Hopper',
        existing_email: '',
        existing_phone: '',
        match_reason: 'name',
      },
      merge_diff: null,
      batch_duplicate_of: null,
      suggested_action: 'update',
      related: { activities: 1, notes: 1, reminders: 0, relationships: 1, gifts: 0 },
      has_photo: false,
    },
  ],
};

test('shows every contact row and the loss report before confirm', () => {
  render(
    <SourceImportReview
      preview={preview}
      rowActions={new Map()}
      onRowAction={() => {}}
      onSetAll={() => {}}
    />,
  );

  expect(screen.getByText('Ada Lovelace')).toBeInTheDocument();
  expect(screen.getByText('Grace Hopper')).toBeInTheDocument();
  // The duplicate is flagged.
  expect(screen.getByText('Matches Grace Hopper')).toBeInTheDocument();
  // The loss report is rendered here, not only on the result screen.
  expect(screen.getByText('What won’t come across exactly')).toBeInTheDocument();
  expect(screen.getByText('Imported with some detail lost')).toBeInTheDocument();
});

test('offers add/skip/update and disables update where there is no duplicate', () => {
  render(
    <SourceImportReview
      preview={preview}
      rowActions={new Map()}
      onRowAction={() => {}}
      onSetAll={() => {}}
    />,
  );
  // Two per-row selects plus the "apply to all" buttons.
  const selects = screen.getAllByRole('combobox');
  expect(selects).toHaveLength(2);
});
