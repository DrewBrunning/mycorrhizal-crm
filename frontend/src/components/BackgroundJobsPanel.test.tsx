import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { getJobRunHealth, type JobRun, type JobRunHealth, listJobRuns } from '../api/jobRuns';
import BackgroundJobsPanel from './BackgroundJobsPanel';

// This codebase's vitest setup has no auto-cleanup and no globals: true.
afterEach(cleanup);

vi.mock('../api/jobRuns', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/jobRuns')>();
  return { ...actual, getJobRunHealth: vi.fn(), listJobRuns: vi.fn() };
});

function health(overrides: Partial<JobRunHealth> = {}): JobRunHealth {
  return {
    job_name: 'daily_reminders',
    status: 'unknown',
    last_run_at: null,
    last_result: '',
    last_trigger: '',
    last_duration_ms: null,
    last_items_processed: null,
    last_success_at: null,
    last_failure_at: null,
    last_error: '',
    incident_first_failure_at: null,
    consecutive_failures: 0,
    duration_sample_size: 0,
    avg_duration_ms: null,
    max_duration_ms: null,
    ...overrides,
  };
}

function run(overrides: Partial<JobRun> = {}): JobRun {
  return {
    id: 1,
    created_at: '2026-08-27T17:40:00Z',
    job_name: 'daily_reminders',
    trigger: 'scheduled',
    started_at: '2026-08-27T17:40:00Z',
    finished_at: '2026-08-27T17:40:01Z',
    duration_ms: 1000,
    result: 'success',
    error: '',
    items_processed: 3,
    detail: '',
    correlation_id: 'job:daily_reminders:abc',
    ...overrides,
  };
}

beforeEach(() => {
  vi.mocked(getJobRunHealth).mockReset();
  vi.mocked(listJobRuns).mockReset();
});

test('renders one row per job with its status', async () => {
  vi.mocked(getJobRunHealth).mockResolvedValue({
    jobs: [
      health({ job_name: 'daily_reminders', status: 'healthy' }),
      health({ job_name: 'calendar_sync', status: 'unknown' }),
    ],
  });

  render(<BackgroundJobsPanel />);

  await waitFor(() => expect(getJobRunHealth).toHaveBeenCalled());
  expect(screen.getByText('Daily reminders')).toBeInTheDocument();
  expect(screen.getByText('Calendar sync')).toBeInTheDocument();
  expect(screen.getByText('Healthy')).toBeInTheDocument();
  expect(screen.getByText('Unknown')).toBeInTheDocument();
});

test('shows the consecutive-failure count and last error when failing', async () => {
  vi.mocked(getJobRunHealth).mockResolvedValue({
    jobs: [
      health({
        job_name: 'daily_reminders',
        status: 'failing',
        consecutive_failures: 4,
        incident_first_failure_at: '2026-08-27T03:19:00Z',
        last_error: '2 notification send(s) failed',
      }),
    ],
  });

  render(<BackgroundJobsPanel />);

  await waitFor(() => expect(getJobRunHealth).toHaveBeenCalled());
  expect(screen.getByText(/Failed 4 times in a row/)).toBeInTheDocument();
  expect(screen.getByText('2 notification send(s) failed')).toBeInTheDocument();
});

test('expanding a row loads and shows its run history', async () => {
  vi.mocked(getJobRunHealth).mockResolvedValue({
    jobs: [health({ job_name: 'daily_reminders', status: 'healthy' })],
  });
  vi.mocked(listJobRuns).mockResolvedValue({
    job_runs: [
      run({ id: 10, result: 'failure', error: 'Resend rejected the request' }),
      run({ id: 11, result: 'skipped', trigger: 'initial' }),
    ],
    total: 2,
  });

  render(<BackgroundJobsPanel />);
  await waitFor(() => expect(getJobRunHealth).toHaveBeenCalled());

  fireEvent.click(screen.getByTestId('job-run-daily_reminders'));

  await waitFor(() =>
    expect(listJobRuns).toHaveBeenCalledWith({ jobName: 'daily_reminders', limit: 25 }),
  );
  expect(await screen.findByText('Resend rejected the request')).toBeInTheDocument();
  expect(screen.getByText('Skipped')).toBeInTheDocument();
});
