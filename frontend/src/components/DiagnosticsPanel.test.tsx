import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { runDiagnostics } from '../api/diagnostics';
import DiagnosticsPanel from './DiagnosticsPanel';

// This codebase's vitest setup has no auto-cleanup and no globals: true.
afterEach(cleanup);

vi.mock('../api/diagnostics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/diagnostics')>();
  return { ...actual, runDiagnostics: vi.fn() };
});

beforeEach(() => {
  vi.mocked(runDiagnostics).mockReset();
});

test('starts empty and only runs the sweep when the operator clicks the button', async () => {
  render(<DiagnosticsPanel />);

  expect(screen.getByText('Run diagnostics')).toBeInTheDocument();
  expect(runDiagnostics).not.toHaveBeenCalled();

  vi.mocked(runDiagnostics).mockResolvedValue({
    timestamp: '2026-08-27T22:00:00Z',
    summary: { status: 'ok', ok: 18, warnings: 0, errors: 0 },
    checks: [
      { name: 'config', status: 'ok', message: 'configuration is valid' },
      { name: 'version', status: 'ok', message: 'version dev' },
    ],
  });

  fireEvent.click(screen.getByRole('button', { name: 'Run diagnostics' }));

  await waitFor(() => expect(runDiagnostics).toHaveBeenCalledTimes(1));
  await waitFor(() => expect(screen.getByTestId('diagnostics-result')).toBeInTheDocument());
  expect(screen.getByText('0 warning(s), 0 error(s)')).toBeInTheDocument();
  expect(screen.getByTestId('diagnostics-check-config')).toBeInTheDocument();
  expect(screen.getByText('Configuration')).toBeInTheDocument();
  expect(screen.getByText('configuration is valid')).toBeInTheDocument();
});

test('renders the summary and per-check statuses when problems are found', async () => {
  vi.mocked(runDiagnostics).mockResolvedValue({
    timestamp: '2026-08-27T22:00:00Z',
    summary: { status: 'error', ok: 15, warnings: 2, errors: 1 },
    checks: [
      { name: 'config', status: 'ok', message: 'configuration is valid' },
      { name: 'filesystem', status: 'error', message: 'attachments directory is missing' },
      { name: 'disk_space', status: 'warning', message: 'filesystem is 95% full' },
    ],
  });

  render(<DiagnosticsPanel />);
  fireEvent.click(screen.getByRole('button', { name: 'Run diagnostics' }));

  await waitFor(() => expect(screen.getByTestId('diagnostics-result')).toBeInTheDocument());
  expect(screen.getByText('2 warning(s), 1 error(s)')).toBeInTheDocument();
  expect(screen.getByText('attachments directory is missing')).toBeInTheDocument();
  expect(screen.getByText('filesystem is 95% full')).toBeInTheDocument();
  // The status chips render inside each row: the summary chip is "Error"
  // (error dominates), plus one error and one warning check.
  expect(screen.getAllByText('Error')).toHaveLength(2);
  expect(screen.getAllByText('Warning')).toHaveLength(1);
});

test('surfaces a load error without dropping the run button', async () => {
  vi.mocked(runDiagnostics).mockRejectedValue(new Error('backend unreachable'));

  render(<DiagnosticsPanel />);
  fireEvent.click(screen.getByRole('button', { name: 'Run diagnostics' }));

  await waitFor(() => expect(screen.getByText('backend unreachable')).toBeInTheDocument());
  expect(screen.getByRole('button', { name: 'Run diagnostics' })).toBeInTheDocument();
});

test('keeps the previous result visible while a refresh is running', async () => {
  vi.mocked(runDiagnostics).mockResolvedValueOnce({
    timestamp: '2026-08-27T22:00:00Z',
    summary: { status: 'ok', ok: 18, warnings: 0, errors: 0 },
    checks: [{ name: 'config', status: 'ok', message: 'configuration is valid' }],
  });
  render(<DiagnosticsPanel />);
  fireEvent.click(screen.getByRole('button', { name: 'Run diagnostics' }));
  await waitFor(() => expect(screen.getByTestId('diagnostics-result')).toBeInTheDocument());

  const pending = new Promise<never>(() => {});
  vi.mocked(runDiagnostics).mockReturnValueOnce(pending as never);
  fireEvent.click(screen.getByRole('button', { name: 'Run diagnostics' }));

  // While the second run is in flight the previous result stays on screen.
  await waitFor(() => expect(screen.getByRole('button', { name: 'Running…' })).toBeDisabled());
  expect(screen.getByTestId('diagnostics-result')).toBeInTheDocument();
});
