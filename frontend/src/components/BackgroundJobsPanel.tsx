import ExpandLessIcon from '@mui/icons-material/ExpandLess';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import RefreshIcon from '@mui/icons-material/Refresh';
import {
  Box,
  Button,
  Chip,
  CircularProgress,
  Collapse,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { JobRunHealth, JobRunResult, JobRunStatus } from '../api/jobRuns';
import { useJobRunHealth, useJobRunHistory } from '../hooks/useJobRuns';

const STATUS_COLORS: Record<JobRunStatus, 'success' | 'error' | 'default'> = {
  healthy: 'success',
  failing: 'error',
  unknown: 'default',
};

const RESULT_COLORS: Record<JobRunResult, 'success' | 'error' | 'warning'> = {
  success: 'success',
  failure: 'error',
  skipped: 'warning',
};

function formatDateTime(iso: string | null): string | null {
  if (!iso) return null;
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  return date.toLocaleString();
}

function formatDuration(ms: number | null | undefined): string {
  if (ms == null) return '—';
  if (ms < 1000) return `${ms} ms`;
  return `${(ms / 1000).toFixed(1)} s`;
}

// A compact "3 hours ago" hint, using the built-in Intl.RelativeTimeFormat (no
// dependency). Mirrors SubsystemHealthPanel's helper.
function relativeFromNow(iso: string | null, locale: string): string | null {
  if (!iso) return null;
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return null;
  const diffSec = Math.round((then - Date.now()) / 1000);
  const abs = Math.abs(diffSec);
  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' });
  if (abs < 60) return rtf.format(diffSec, 'second');
  if (abs < 3600) return rtf.format(Math.round(diffSec / 60), 'minute');
  if (abs < 86400) return rtf.format(Math.round(diffSec / 3600), 'hour');
  return rtf.format(Math.round(diffSec / 86400), 'day');
}

// BackgroundJobsPanel — the per-job run monitor (issue #391) on the System
// Events page. One row per scheduled job: status, last run, duration, and the
// consecutive-failure count; expand a row to load its recent run history.
export default function BackgroundJobsPanel() {
  const { t } = useTranslation();
  const { data, loading, error, refresh } = useJobRunHealth();

  return (
    <Paper sx={{ p: 1.5, mb: 2 }}>
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: 1,
          mb: 1,
        }}
      >
        <Box>
          <Typography variant="subtitle1" component="h2">
            {t('backgroundJobs.title')}
          </Typography>
          <Typography
            variant="body2"
            sx={{
              color: 'text.secondary',
            }}
          >
            {t('backgroundJobs.description')}
          </Typography>
        </Box>
        <Button
          size="small"
          startIcon={<RefreshIcon />}
          onClick={refresh}
          disabled={loading}
          sx={{ flexShrink: 0 }}
        >
          {t('backgroundJobs.refresh')}
        </Button>
      </Box>

      {error && (
        <Typography color="error" variant="body2" sx={{ mb: 1 }}>
          {error}
        </Typography>
      )}

      <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
        {data.map((job) => (
          <JobRow key={job.job_name} job={job} />
        ))}
      </Box>
    </Paper>
  );
}

function JobRow({ job }: { job: JobRunHealth }) {
  const { t, i18n } = useTranslation();
  const [open, setOpen] = useState(false);
  const history = useJobRunHistory();

  const failing = job.status === 'failing';
  const lastRunAbs = formatDateTime(job.last_run_at);
  const lastRunRel = relativeFromNow(job.last_run_at, i18n.language);

  const toggle = () => {
    const next = !open;
    setOpen(next);
    if (next && history.data.length === 0 && !history.loading) {
      history.load({ jobName: job.job_name, limit: 25 });
    }
  };

  return (
    <Box
      sx={{
        border: 1,
        borderColor: failing ? 'error.main' : 'divider',
        borderRadius: 1,
        bgcolor: 'background.paper',
      }}
    >
      <Box
        component="button"
        type="button"
        onClick={toggle}
        data-testid={`job-run-${job.job_name}`}
        aria-expanded={open}
        sx={{
          width: '100%',
          textAlign: 'left',
          font: 'inherit',
          cursor: 'pointer',
          border: 0,
          bgcolor: 'transparent',
          p: 1.25,
          display: 'flex',
          alignItems: 'center',
          gap: 1,
          flexWrap: 'wrap',
          '&:hover': { bgcolor: 'action.hover' },
        }}
      >
        <Typography variant="body2" sx={{ fontWeight: 600, flexGrow: 1 }}>
          {t(`backgroundJobs.jobs.${job.job_name}`, job.job_name)}
        </Typography>

        <Chip
          size="small"
          color={STATUS_COLORS[job.status]}
          label={t(`backgroundJobs.status.${job.status}`)}
        />

        {failing && job.consecutive_failures > 0 && (
          <Typography variant="caption" color="error">
            {t('backgroundJobs.consecutiveFailures', { n: job.consecutive_failures })}
            {job.incident_first_failure_at &&
              ` ${t('backgroundJobs.incidentSince', {
                time:
                  relativeFromNow(job.incident_first_failure_at, i18n.language) ??
                  formatDateTime(job.incident_first_failure_at),
              })}`}
          </Typography>
        )}

        <Typography
          variant="caption"
          sx={{
            color: 'text.secondary',
          }}
        >
          {t('backgroundJobs.lastRun')}:{' '}
          {lastRunAbs
            ? lastRunRel
              ? `${lastRunAbs} (${lastRunRel})`
              : lastRunAbs
            : t('backgroundJobs.never')}
        </Typography>

        <Typography
          variant="caption"
          sx={{
            color: 'text.secondary',
          }}
        >
          {t('backgroundJobs.duration')}: {formatDuration(job.last_duration_ms)}
          {job.duration_sample_size > 1 &&
            job.avg_duration_ms != null &&
            job.max_duration_ms != null &&
            ` · ${t('backgroundJobs.durationTrend', {
              avg: formatDuration(job.avg_duration_ms),
              max: formatDuration(job.max_duration_ms),
              n: job.duration_sample_size,
            })}`}
        </Typography>

        {open ? <ExpandLessIcon fontSize="small" /> : <ExpandMoreIcon fontSize="small" />}
      </Box>

      {failing && job.last_error && (
        <Typography
          variant="caption"
          color="error"
          sx={{ display: 'block', px: 1.25, pb: 1, wordBreak: 'break-word' }}
        >
          {job.last_error}
        </Typography>
      )}

      <Collapse in={open} unmountOnExit>
        <Box sx={{ px: 1.25, pb: 1.25 }}>
          <Typography
            variant="caption"
            sx={{
              color: 'text.secondary',
              fontWeight: 600,
            }}
          >
            {t('backgroundJobs.history')}
          </Typography>
          {history.loading && (
            <Box sx={{ py: 1 }}>
              <CircularProgress size={18} />
            </Box>
          )}
          {history.error && (
            <Typography color="error" variant="caption" sx={{ display: 'block' }}>
              {history.error}
            </Typography>
          )}
          {!history.loading && !history.error && history.data.length === 0 && (
            <Typography
              variant="caption"
              sx={{
                color: 'text.secondary',
                display: 'block',
              }}
            >
              {t('backgroundJobs.noHistory')}
            </Typography>
          )}
          {history.data.length > 0 && (
            <Box sx={{ overflowX: 'auto' }}>
              <Table size="small" sx={{ mt: 0.5 }}>
                <TableHead>
                  <TableRow>
                    <TableCell>{t('backgroundJobs.columns.result')}</TableCell>
                    <TableCell>{t('backgroundJobs.columns.started')}</TableCell>
                    <TableCell align="right">{t('backgroundJobs.columns.duration')}</TableCell>
                    <TableCell align="right">{t('backgroundJobs.columns.items')}</TableCell>
                    <TableCell>{t('backgroundJobs.columns.error')}</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {history.data.map((run) => (
                    <TableRow key={run.id}>
                      <TableCell>
                        <Chip
                          size="small"
                          color={RESULT_COLORS[run.result]}
                          label={t(`backgroundJobs.result.${run.result}`)}
                        />{' '}
                        <Typography
                          variant="caption"
                          sx={{
                            color: 'text.secondary',
                          }}
                        >
                          {t(`backgroundJobs.trigger.${run.trigger}`, run.trigger)}
                        </Typography>
                      </TableCell>
                      <TableCell>{formatDateTime(run.started_at)}</TableCell>
                      <TableCell align="right">{formatDuration(run.duration_ms)}</TableCell>
                      <TableCell align="right">
                        {run.items_processed != null ? run.items_processed : '—'}
                      </TableCell>
                      <TableCell sx={{ wordBreak: 'break-word', maxWidth: 320 }}>
                        {run.error || ''}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Box>
          )}
        </Box>
      </Collapse>
    </Box>
  );
}
