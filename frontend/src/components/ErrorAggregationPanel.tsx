import RefreshIcon from '@mui/icons-material/Refresh';
import {
  Box,
  Button,
  Chip,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { ErrorBucket } from '../api/errorAggregation';
import { useErrorAggregation } from '../hooks/useErrorAggregation';

const WINDOW_HOURS = 24;

function formatDateTime(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  return date.toLocaleString();
}

// A compact "3 hours ago" hint, using the built-in Intl.RelativeTimeFormat (no
// dependency), mirroring SubsystemHealthPanel. Best-effort — null if it can't
// be computed.
function relativeFromNow(iso: string, locale: string): string | null {
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

// ErrorAggregationPanel — operational failures over the last 24h bucketed by
// cause (issue #426), above the operational-event timeline. "17 CardDAV
// authentication failures" instead of 17 rows to correlate by hand. Clicking
// "View N events" opens exactly those system_events rows in the timeline below.
export default function ErrorAggregationPanel({
  onViewEvents,
}: {
  onViewEvents: (ids: number[]) => void;
}) {
  const { t, i18n } = useTranslation();
  const { buckets, loading, error, refresh } = useErrorAggregation(WINDOW_HOURS);

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
            {t('errorAggregation.title')}
          </Typography>
          <Typography
            variant="body2"
            sx={{
              color: 'text.secondary',
            }}
          >
            {t('errorAggregation.description', { hours: WINDOW_HOURS })}
          </Typography>
        </Box>
        <Button
          size="small"
          startIcon={<RefreshIcon />}
          onClick={refresh}
          disabled={loading}
          sx={{ flexShrink: 0 }}
        >
          {t('errorAggregation.refresh')}
        </Button>
      </Box>

      {error && (
        <Typography color="error" variant="body2" sx={{ mb: 1 }}>
          {error}
        </Typography>
      )}

      {buckets.length === 0 ? (
        <Typography
          variant="body2"
          sx={{
            color: 'text.secondary',
          }}
        >
          {t('errorAggregation.empty', { hours: WINDOW_HOURS })}
        </Typography>
      ) : (
        <TableContainer sx={{ overflowX: 'auto' }}>
          <Table size="small" sx={{ minWidth: 640 }}>
            <TableHead>
              <TableRow>
                <TableCell align="right">{t('errorAggregation.columns.count')}</TableCell>
                <TableCell>{t('errorAggregation.columns.component')}</TableCell>
                <TableCell>{t('errorAggregation.columns.error')}</TableCell>
                <TableCell>{t('errorAggregation.columns.lastSeen')}</TableCell>
                <TableCell align="right">{t('errorAggregation.columns.actions')}</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {buckets.map((b) => (
                <ErrorBucketRow
                  key={`${b.component}${b.cause}`}
                  bucket={b}
                  locale={i18n.language}
                  componentLabel={t(`subsystemHealth.subsystems.${b.component}`, {
                    defaultValue: b.component || '—',
                  })}
                  onViewEvents={() => onViewEvents(b.event_ids)}
                />
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}
    </Paper>
  );
}

function ErrorBucketRow({
  bucket,
  locale,
  componentLabel,
  onViewEvents,
}: {
  bucket: ErrorBucket;
  locale: string;
  componentLabel: string;
  onViewEvents: () => void;
}) {
  const { t } = useTranslation();
  const rel = relativeFromNow(bucket.last_seen, locale);
  return (
    <TableRow hover>
      <TableCell align="right">
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'flex-end',
            gap: 0.75,
          }}
        >
          <Typography
            variant="body2"
            component="span"
            sx={{ fontWeight: 700 }}
            color={bucket.recurring ? 'error' : 'text.primary'}
          >
            {bucket.count}
          </Typography>
          {bucket.recurring && (
            <Chip
              size="small"
              color="error"
              variant="outlined"
              label={t('errorAggregation.recurring')}
            />
          )}
        </Box>
      </TableCell>
      <TableCell sx={{ overflowWrap: 'anywhere' }}>{componentLabel}</TableCell>
      <TableCell sx={{ overflowWrap: 'anywhere' }}>
        <Typography variant="body2" component="span" color="error" sx={{ wordBreak: 'break-word' }}>
          {bucket.sample_error || bucket.cause}
        </Typography>
      </TableCell>
      <TableCell sx={{ whiteSpace: 'nowrap' }}>
        <Typography
          variant="caption"
          sx={{
            color: 'text.secondary',
          }}
        >
          {rel ? `${formatDateTime(bucket.last_seen)} (${rel})` : formatDateTime(bucket.last_seen)}
        </Typography>
      </TableCell>
      <TableCell align="right">
        <Button
          size="small"
          color="inherit"
          onClick={onViewEvents}
          disabled={bucket.event_ids.length === 0}
        >
          {t('errorAggregation.viewEvents', { n: bucket.event_ids.length })}
        </Button>
      </TableCell>
    </TableRow>
  );
}
