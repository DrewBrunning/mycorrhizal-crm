import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import { Box, Button, Chip, Paper, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { DiagnosticCheck, DiagnosticStatus } from '../api/diagnostics';
import { useDiagnostics } from '../hooks/useDiagnostics';

const STATUS_COLORS: Record<DiagnosticStatus, 'success' | 'warning' | 'error' | 'default'> = {
  ok: 'success',
  warning: 'warning',
  error: 'error',
};

// diagnosticsCheckNames is the hand-maintained mirror of the backend sweep's
// stable check identifiers (backend/services/diagnostics.go). No dynamic
// type-list endpoint exists — keep this in sync with the backend and with
// backend/openapi.yaml's DiagnosticCheck.name description (frontend trap #4).
const diagnosticsCheckNames = [
  'config',
  'database',
  'migrations',
  'filesystem',
  'backup',
  'notification_email',
  'notification_ntfy',
  'notification_gotify',
  'notification_push',
  'integration_carddav',
  'integration_caldav',
  'integration_immich',
  'integration_paperless',
  'integration_seafile',
  'integration_nextcloud',
  'disk_space',
  'background_jobs',
  'version',
] as const;

// DiagnosticsPanel — the admin "Run diagnostics" action (issue #423). The
// one-pass sweep is manual (unlike the continuously-derived panels next to
// it), so nothing loads on mount: the operator clicks "Run diagnostics", and
// the ok/warning/error checklist with its summary appears. Secret-free by
// construction — the backend never echoes config values.
export default function DiagnosticsPanel() {
  const { t } = useTranslation();
  const { data, loading, error, run } = useDiagnostics();

  return (
    <Paper sx={{ p: 1.5, mb: 2 }}>
      <Box display="flex" alignItems="center" justifyContent="space-between" gap={1} sx={{ mb: 1 }}>
        <Box>
          <Typography variant="subtitle1" component="h2">
            {t('diagnostics.title')}
          </Typography>
          <Typography variant="body2" color="text.secondary">
            {t('diagnostics.description')}
          </Typography>
        </Box>
        <Button
          size="small"
          variant="outlined"
          startIcon={<PlayArrowIcon />}
          onClick={run}
          disabled={loading}
          sx={{ flexShrink: 0 }}
        >
          {loading ? t('diagnostics.running') : t('diagnostics.run')}
        </Button>
      </Box>

      {error && (
        <Typography color="error" variant="body2" sx={{ mb: 1 }}>
          {error}
        </Typography>
      )}

      {data && (
        <Box data-testid="diagnostics-result">
          <Box display="flex" alignItems="center" gap={1} sx={{ mb: 1 }}>
            <Chip
              size="small"
              color={STATUS_COLORS[data.summary.status]}
              label={t(`diagnostics.status.${data.summary.status}`)}
            />
            <Typography variant="body2">
              {t('diagnostics.summary', {
                warnings: data.summary.warnings,
                errors: data.summary.errors,
              })}
            </Typography>
          </Box>

          <Box
            sx={{
              display: 'grid',
              gap: 0.5,
              gridTemplateColumns: 'repeat(auto-fill, minmax(260px, 1fr))',
            }}
          >
            {data.checks.map((check) => (
              <CheckRow key={check.name} check={check} />
            ))}
          </Box>
        </Box>
      )}
    </Paper>
  );
}

function CheckRow({ check }: { check: DiagnosticCheck }) {
  const { t } = useTranslation();
  const isKnown = (diagnosticsCheckNames as readonly string[]).includes(check.name);

  return (
    <Box
      data-testid={`diagnostics-check-${check.name}`}
      sx={{
        display: 'flex',
        alignItems: 'flex-start',
        gap: 1,
        p: 0.75,
        border: 1,
        borderColor: 'divider',
        borderRadius: 1,
      }}
    >
      <Chip
        size="small"
        color={STATUS_COLORS[check.status]}
        label={t(`diagnostics.status.${check.status}`)}
        sx={{ flexShrink: 0, mt: 0.25 }}
      />
      <Box>
        <Typography variant="body2" sx={{ fontWeight: 600 }}>
          {isKnown ? t(`diagnostics.checks.${check.name}`) : check.name}
        </Typography>
        <Typography variant="caption" color="text.secondary" sx={{ wordBreak: 'break-word' }}>
          {check.message}
        </Typography>
      </Box>
    </Box>
  );
}
