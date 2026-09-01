import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import '../../i18n/config';
import type { SourceImportWizard as Wizard } from '../../hooks/useSourceImportWizard';
import SourceImportWizard from './SourceImportWizard';

afterEach(cleanup);

function makeWizard(overrides: Partial<Wizard>): Wizard {
  return {
    step: 'connect',
    status: null,
    preview: null,
    result: null,
    rowActions: new Map(),
    error: null,
    busy: false,
    sessionId: null,
    beginFetch: vi.fn(),
    setRowAction: vi.fn(),
    setAllActions: vi.fn(),
    confirm: vi.fn(),
    cancelImport: vi.fn(),
    cancel: vi.fn(),
    reset: vi.fn(),
    ...overrides,
  };
}

function renderWizard(wizard: Wizard) {
  return render(
    <SourceImportWizard
      open
      onClose={() => {}}
      titleKey="settings.monicaImport.title"
      sourceLabel="Monica"
      wizard={wizard}
      connectStep={<div>connect-step-content</div>}
      onComplete={() => {}}
    />,
  );
}

test('connect step renders the source-specific content and the 5-step stepper', () => {
  renderWizard(makeWizard({ step: 'connect' }));
  expect(screen.getByText('connect-step-content')).toBeInTheDocument();
  for (const label of ['Connect', 'Fetch', 'Review', 'Import', 'Done']) {
    // Step labels live in the stepper; there may be a same-named button too.
    expect(screen.getAllByText(label).length).toBeGreaterThan(0);
  }
});

test('importing step shows progress and a working Cancel import button', () => {
  const cancelImport = vi.fn();
  renderWizard(
    makeWizard({
      step: 'importing',
      status: { session_id: 's1', phase: 'importing', phase_done: 2, phase_total: 12 },
      cancelImport,
    }),
  );
  expect(screen.getByRole('progressbar')).toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'Cancel import' }));
  expect(cancelImport).toHaveBeenCalled();
});

test('review step confirms; result step completes', () => {
  const confirm = vi.fn();
  const onComplete = vi.fn();
  const wizard = makeWizard({
    step: 'review',
    preview: {
      session_id: 's1',
      rows: [],
      total_rows: 0,
      valid_rows: 0,
      duplicate_count: 0,
      error_count: 0,
      totals: { activities: 0, notes: 0, reminders: 0, relationships: 0, gifts: 0 },
      loss_report: [],
    },
    confirm,
  });
  const { rerender } = renderWizard(wizard);
  fireEvent.click(screen.getByRole('button', { name: 'Import' }));
  expect(confirm).toHaveBeenCalled();

  rerender(
    <SourceImportWizard
      open
      onClose={() => {}}
      titleKey="settings.monicaImport.title"
      sourceLabel="Monica"
      wizard={makeWizard({
        step: 'result',
        result: {
          total_processed: 1,
          created: 1,
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
        },
      })}
      connectStep={<div />}
      onComplete={onComplete}
    />,
  );
  fireEvent.click(screen.getByRole('button', { name: 'Done' }));
  expect(onComplete).toHaveBeenCalled();
});

test('an error is shown as an alert', () => {
  renderWizard(makeWizard({ step: 'connect', error: 'something broke' }));
  expect(screen.getByText('something broke')).toBeInTheDocument();
});
