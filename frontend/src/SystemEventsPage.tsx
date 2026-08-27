import ClearIcon from '@mui/icons-material/Clear';
import HubIcon from '@mui/icons-material/Hub';
import {
  Box,
  Button,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  InputLabel,
  MenuItem,
  Paper,
  Select,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from '@mui/material';
import type { SelectChangeEvent } from '@mui/material/Select';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  SYSTEM_EVENT_COMPONENTS,
  SYSTEM_EVENT_SEVERITIES,
  SYSTEM_EVENT_TYPES,
  type SystemEvent,
  type SystemEventSeverity,
  type SystemEventType,
} from './api/systemEvents';
import { ListSkeleton } from './components/LoadingSkeletons';
import { useDebouncedValue } from './hooks/useDebounce';
import { useDocumentTitle } from './hooks/useDocumentTitle';
import { useSystemEvents } from './hooks/useSystemEvents';

const SEVERITY_COLORS: Record<SystemEventSeverity, 'default' | 'warning' | 'error'> = {
  info: 'default',
  warn: 'warning',
  error: 'error',
};

const RESULT_COLORS: Record<string, 'success' | 'error' | 'default'> = {
  success: 'success',
  failure: 'error',
  skipped: 'default',
};

function formatDateTime(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  return date.toLocaleString();
}

function formatDuration(ms?: number): string {
  if (ms == null) return '—';
  if (ms < 1000) return `${ms} ms`;
  return `${(ms / 1000).toFixed(1)} s`;
}

// SystemEventsPage — the operational-event timeline (issue #424). Admin-only,
// read-only. The first place an admin looks when something is wrong: filter by
// component / severity / event type, then drill from any event into every
// other event sharing its correlation ID ("view related").
export default function SystemEventsPage() {
  const { t } = useTranslation();
  useDocumentTitle(t('nav.systemEvents'));

  const {
    events,
    loading,
    error,
    filters,
    hasFilters,
    patchFilters,
    clearFilters,
    showRelated,
    canLoadMore,
    loadMore,
  } = useSystemEvents();

  const [correlationInput, setCorrelationInput] = useState('');
  const debouncedCorrelation = useDebouncedValue(correlationInput, 350);
  useEffect(() => {
    patchFilters({ correlationId: debouncedCorrelation });
  }, [debouncedCorrelation, patchFilters]);

  // Keep the text field in sync when a "view related" action sets the filter
  // programmatically.
  useEffect(() => {
    setCorrelationInput((prev) => (prev === filters.correlationId ? prev : filters.correlationId));
  }, [filters.correlationId]);

  const [selected, setSelected] = useState<SystemEvent | null>(null);

  const handleClear = () => {
    setCorrelationInput('');
    clearFilters();
  };

  const handleViewRelated = (correlationId: string) => {
    setSelected(null);
    setCorrelationInput(correlationId);
    showRelated(correlationId);
  };

  return (
    <Box sx={{ maxWidth: 1200, mx: 'auto', mt: 2, p: 2 }}>
      <Typography variant="h5" component="h1" gutterBottom sx={{ mb: 1.5 }}>
        {t('systemEvents.title')}
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        {t('systemEvents.description')}
      </Typography>

      <Paper sx={{ p: 1.5, mb: 2 }}>
        <Box display="flex" gap={2} flexWrap="wrap" alignItems="center">
          <FormControl size="small" sx={{ minWidth: 170 }}>
            <InputLabel id="se-component-label">{t('systemEvents.filters.component')}</InputLabel>
            <Select
              labelId="se-component-label"
              label={t('systemEvents.filters.component')}
              value={filters.component}
              onChange={(e: SelectChangeEvent<string>) =>
                patchFilters({ component: e.target.value })
              }
            >
              <MenuItem value="">{t('systemEvents.filters.all')}</MenuItem>
              {SYSTEM_EVENT_COMPONENTS.map((c) => (
                <MenuItem key={c} value={c}>
                  {c}
                </MenuItem>
              ))}
            </Select>
          </FormControl>

          <FormControl size="small" sx={{ minWidth: 150 }}>
            <InputLabel id="se-severity-label">{t('systemEvents.filters.severity')}</InputLabel>
            <Select
              labelId="se-severity-label"
              label={t('systemEvents.filters.severity')}
              value={filters.severity}
              onChange={(e: SelectChangeEvent<string>) =>
                patchFilters({ severity: e.target.value as SystemEventSeverity | '' })
              }
            >
              <MenuItem value="">{t('systemEvents.filters.all')}</MenuItem>
              {SYSTEM_EVENT_SEVERITIES.map((s) => (
                <MenuItem key={s} value={s}>
                  {t(`systemEvents.severities.${s}`)}
                </MenuItem>
              ))}
            </Select>
          </FormControl>

          <FormControl size="small" sx={{ minWidth: 190 }}>
            <InputLabel id="se-type-label">{t('systemEvents.filters.eventType')}</InputLabel>
            <Select
              labelId="se-type-label"
              label={t('systemEvents.filters.eventType')}
              value={filters.eventType}
              onChange={(e: SelectChangeEvent<string>) =>
                patchFilters({ eventType: e.target.value as SystemEventType | '' })
              }
            >
              <MenuItem value="">{t('systemEvents.filters.all')}</MenuItem>
              {SYSTEM_EVENT_TYPES.map((et) => (
                <MenuItem key={et} value={et}>
                  {t(`systemEvents.eventTypes.${et}`)}
                </MenuItem>
              ))}
            </Select>
          </FormControl>

          <TextField
            size="small"
            label={t('systemEvents.filters.correlationId')}
            value={correlationInput}
            onChange={(e) => setCorrelationInput(e.target.value)}
            variant="outlined"
            sx={{ flex: 1, minWidth: 220 }}
          />

          <Button
            variant="outlined"
            size="medium"
            startIcon={<ClearIcon />}
            onClick={handleClear}
            disabled={!hasFilters}
          >
            {t('systemEvents.filters.clear')}
          </Button>
        </Box>
      </Paper>

      {filters.correlationId.trim() !== '' && (
        <Paper sx={{ p: 1.5, mb: 2, display: 'flex', alignItems: 'center', gap: 1 }}>
          <HubIcon fontSize="small" color="action" />
          <Typography variant="body2">
            {t('systemEvents.relatedBanner', { id: filters.correlationId })}
          </Typography>
        </Paper>
      )}

      {error && (
        <Paper sx={{ p: 2, mb: 2 }}>
          <Typography color="error">{error}</Typography>
        </Paper>
      )}

      {loading && events.length === 0 ? (
        <ListSkeleton count={8} />
      ) : events.length === 0 ? (
        <Paper sx={{ p: 4, textAlign: 'center' }}>
          <Typography variant="body1" color="text.secondary">
            {hasFilters ? t('systemEvents.empty') : t('systemEvents.emptyNoFilters')}
          </Typography>
        </Paper>
      ) : (
        <TableContainer component={Paper} sx={{ overflowX: 'auto' }}>
          <Table size="small" sx={{ minWidth: 760 }}>
            <TableHead>
              <TableRow>
                <TableCell>{t('systemEvents.columns.time')}</TableCell>
                <TableCell>{t('systemEvents.columns.severity')}</TableCell>
                <TableCell>{t('systemEvents.columns.component')}</TableCell>
                <TableCell>{t('systemEvents.columns.event')}</TableCell>
                <TableCell>{t('systemEvents.columns.operation')}</TableCell>
                <TableCell>{t('systemEvents.columns.result')}</TableCell>
                <TableCell align="right">{t('systemEvents.columns.actions')}</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {events.map((event) => (
                <TableRow key={event.id} hover>
                  <TableCell>
                    <Typography variant="body2">{formatDateTime(event.occurred_at)}</Typography>
                  </TableCell>
                  <TableCell>
                    <Chip
                      size="small"
                      color={SEVERITY_COLORS[event.severity]}
                      label={t(`systemEvents.severities.${event.severity}`)}
                    />
                  </TableCell>
                  <TableCell sx={{ overflowWrap: 'anywhere' }}>{event.component || '—'}</TableCell>
                  <TableCell sx={{ overflowWrap: 'anywhere' }}>
                    {t(`systemEvents.eventTypes.${event.event_type}`)}
                  </TableCell>
                  <TableCell sx={{ overflowWrap: 'anywhere' }}>{event.operation || '—'}</TableCell>
                  <TableCell>
                    {event.result ? (
                      <Chip
                        size="small"
                        variant="outlined"
                        color={RESULT_COLORS[event.result] ?? 'default'}
                        label={t(`systemEvents.results.${event.result}`)}
                      />
                    ) : (
                      '—'
                    )}
                  </TableCell>
                  <TableCell align="right">
                    <Button size="small" color="inherit" onClick={() => setSelected(event)}>
                      {t('systemEvents.details.button')}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}

      {canLoadMore && (
        <Box display="flex" justifyContent="center" mt={3}>
          <Button variant="outlined" onClick={loadMore} disabled={loading}>
            {t('common.loadMore')}
          </Button>
        </Box>
      )}

      <Dialog open={!!selected} onClose={() => setSelected(null)} maxWidth="sm" fullWidth>
        <DialogTitle>{t('systemEvents.details.title')}</DialogTitle>
        <DialogContent dividers>
          {selected && (
            <Box
              component="dl"
              sx={{ display: 'grid', gridTemplateColumns: 'max-content 1fr', gap: 1 }}
            >
              <DetailRow label={t('systemEvents.columns.event')}>
                {t(`systemEvents.eventTypes.${selected.event_type}`)}
              </DetailRow>
              <DetailRow label={t('systemEvents.columns.time')}>
                {formatDateTime(selected.occurred_at)}
              </DetailRow>
              <DetailRow label={t('systemEvents.columns.severity')}>
                {t(`systemEvents.severities.${selected.severity}`)}
              </DetailRow>
              <DetailRow label={t('systemEvents.columns.component')}>
                {selected.component || '—'}
              </DetailRow>
              <DetailRow label={t('systemEvents.columns.operation')}>
                {selected.operation || '—'}
              </DetailRow>
              <DetailRow label={t('systemEvents.columns.result')}>
                {selected.result ? t(`systemEvents.results.${selected.result}`) : '—'}
              </DetailRow>
              <DetailRow label={t('systemEvents.columns.duration')}>
                {formatDuration(selected.duration_ms)}
              </DetailRow>
              <DetailRow label={t('systemEvents.details.correlationId')}>
                <Typography variant="body2" component="span" sx={{ wordBreak: 'break-all' }}>
                  {selected.correlation_id || '—'}
                </Typography>
              </DetailRow>
              {selected.detail && (
                <DetailRow label={t('systemEvents.details.detail')}>
                  <Typography variant="body2" component="span" sx={{ wordBreak: 'break-word' }}>
                    {selected.detail}
                  </Typography>
                </DetailRow>
              )}
              {selected.error && (
                <DetailRow label={t('systemEvents.details.error')}>
                  <Typography
                    variant="body2"
                    component="span"
                    color="error"
                    sx={{ wordBreak: 'break-word' }}
                  >
                    {selected.error}
                  </Typography>
                </DetailRow>
              )}
            </Box>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setSelected(null)}>{t('common.close')}</Button>
          <Button
            variant="contained"
            startIcon={<HubIcon />}
            disabled={!selected?.correlation_id}
            onClick={() => selected?.correlation_id && handleViewRelated(selected.correlation_id)}
          >
            {t('systemEvents.details.viewRelated')}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}

function DetailRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <>
      <Typography component="dt" variant="body2" color="text.secondary" sx={{ fontWeight: 600 }}>
        {label}
      </Typography>
      <Typography component="dd" variant="body2" sx={{ m: 0 }}>
        {children}
      </Typography>
    </>
  );
}
