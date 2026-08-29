import RefreshIcon from '@mui/icons-material/Refresh';
import { Box, Button, Chip, Paper, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { SubsystemHealth, SubsystemStatus } from '../api/subsystemHealth';
import { useSubsystemHealth } from '../hooks/useSubsystemHealth';

const STATUS_COLORS: Record<SubsystemStatus, 'success' | 'error' | 'default'> = {
  healthy: 'success',
  failing: 'error',
  unknown: 'default',
};

function formatDateTime(iso: string | null): string | null {
  if (!iso) return null;
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  return date.toLocaleString();
}

// A compact "3 hours ago" hint next to the absolute incident-start time, using
// the built-in Intl.RelativeTimeFormat (no dependency). Best-effort — returns
// null if it can't be computed.
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

// SubsystemHealthPanel — the per-subsystem last-known-good state (issue #427)
// above the operational-event timeline. "The last success was 17:04, it has
// failed 9 times in a row, and this incident started at 17:19." Clicking a
// card filters the timeline below to that subsystem's component.
export default function SubsystemHealthPanel({
  onSelectComponent,
}: {
  onSelectComponent: (component: string) => void;
}) {
  const { t, i18n } = useTranslation();
  const { data, loading, error, refresh } = useSubsystemHealth();

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
            {t('subsystemHealth.title')}
          </Typography>
          <Typography
            variant="body2"
            sx={{
              color: 'text.secondary',
            }}
          >
            {t('subsystemHealth.description')}
          </Typography>
        </Box>
        <Button
          size="small"
          startIcon={<RefreshIcon />}
          onClick={refresh}
          disabled={loading}
          sx={{ flexShrink: 0 }}
        >
          {t('subsystemHealth.refresh')}
        </Button>
      </Box>

      {error && (
        <Typography color="error" variant="body2" sx={{ mb: 1 }}>
          {error}
        </Typography>
      )}

      <Box
        sx={{
          display: 'grid',
          gap: 1.5,
          gridTemplateColumns: 'repeat(auto-fill, minmax(240px, 1fr))',
        }}
      >
        {data.map((h) => (
          <SubsystemCard
            key={h.subsystem}
            health={h}
            locale={i18n.language}
            onClick={() => onSelectComponent(h.subsystem)}
          />
        ))}
      </Box>
    </Paper>
  );
}

function SubsystemCard({
  health,
  locale,
  onClick,
}: {
  health: SubsystemHealth;
  locale: string;
  onClick: () => void;
}) {
  const { t } = useTranslation();
  const failing = health.status === 'failing';
  const incidentAbs = formatDateTime(health.incident_first_failure_at);
  const incidentRel = relativeFromNow(health.incident_first_failure_at, locale);

  return (
    <Box
      component="button"
      type="button"
      onClick={onClick}
      data-testid={`subsystem-health-${health.subsystem}`}
      sx={{
        textAlign: 'left',
        font: 'inherit',
        cursor: 'pointer',
        border: 1,
        borderColor: failing ? 'error.main' : 'divider',
        borderRadius: 1,
        bgcolor: 'background.paper',
        p: 1.25,
        display: 'flex',
        flexDirection: 'column',
        gap: 0.5,
        '&:hover': { bgcolor: 'action.hover' },
      }}
    >
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: 1,
        }}
      >
        <Typography variant="body2" sx={{ fontWeight: 600 }}>
          {t(`subsystemHealth.subsystems.${health.subsystem}`)}
        </Typography>
        <Chip
          size="small"
          color={STATUS_COLORS[health.status]}
          label={t(`subsystemHealth.status.${health.status}`)}
        />
      </Box>

      {failing && (
        <Typography variant="body2" color="error">
          {t('subsystemHealth.consecutiveFailures', { n: health.consecutive_failures })}
        </Typography>
      )}

      {failing && incidentAbs && (
        <Typography
          variant="caption"
          sx={{
            color: 'text.secondary',
          }}
        >
          {t('subsystemHealth.incidentSince', {
            time: incidentRel ? `${incidentAbs} (${incidentRel})` : incidentAbs,
          })}
        </Typography>
      )}

      <Typography
        variant="caption"
        sx={{
          color: 'text.secondary',
        }}
      >
        {t('subsystemHealth.lastSuccess')}:{' '}
        {formatDateTime(health.last_success_at) ?? t('subsystemHealth.never')}
      </Typography>
      <Typography
        variant="caption"
        sx={{
          color: 'text.secondary',
        }}
      >
        {t('subsystemHealth.lastFailure')}:{' '}
        {formatDateTime(health.last_failure_at) ?? t('subsystemHealth.never')}
      </Typography>

      {failing && health.last_error && (
        <Typography variant="caption" color="error" sx={{ wordBreak: 'break-word', mt: 0.25 }}>
          {health.last_error}
        </Typography>
      )}
    </Box>
  );
}
