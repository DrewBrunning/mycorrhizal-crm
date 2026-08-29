import RefreshIcon from '@mui/icons-material/Refresh';
import { Box, Button, Chip, Paper, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type {
  NotificationChannelHealth,
  NotificationChannelStatus,
} from '../api/notificationHealth';
import { useNotificationChannelHealth } from '../hooks/useNotificationChannelHealth';

const STATUS_COLORS: Record<
  NotificationChannelStatus,
  'success' | 'error' | 'warning' | 'default'
> = {
  healthy: 'success',
  failing: 'error',
  no_devices: 'warning',
  unconfigured: 'default',
};

function formatDateTime(iso: string | null): string | null {
  if (!iso) return null;
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  return date.toLocaleString();
}

// NotificationHealthPanel — the per-channel notification delivery health
// surface (issue #422), next to the subsystem-health panel on the admin
// observability page. Tells an admin apart "notifications are broken"
// (failing), "no browser devices are registered" (no_devices), and "nothing
// is configured" (unconfigured) — three different remedies.
export default function NotificationHealthPanel() {
  const { t } = useTranslation();
  const { data, loading, error, refresh } = useNotificationChannelHealth();

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
            {t('notificationHealth.title')}
          </Typography>
          <Typography
            variant="body2"
            sx={{
              color: 'text.secondary',
            }}
          >
            {t('notificationHealth.description')}
          </Typography>
        </Box>
        <Button
          size="small"
          startIcon={<RefreshIcon />}
          onClick={refresh}
          disabled={loading}
          sx={{ flexShrink: 0 }}
        >
          {t('notificationHealth.refresh')}
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
          <ChannelCard key={h.channel} health={h} />
        ))}
      </Box>
    </Paper>
  );
}

function ChannelCard({ health }: { health: NotificationChannelHealth }) {
  const { t } = useTranslation();
  const failing = health.status === 'failing';

  return (
    <Box
      data-testid={`notification-health-${health.channel}`}
      sx={{
        textAlign: 'left',
        font: 'inherit',
        border: 1,
        borderColor: failing ? 'error.main' : 'divider',
        borderRadius: 1,
        bgcolor: 'background.paper',
        p: 1.25,
        display: 'flex',
        flexDirection: 'column',
        gap: 0.5,
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
          {t(`notificationHealth.channels.${health.channel}`)}
        </Typography>
        <Chip
          size="small"
          color={STATUS_COLORS[health.status]}
          label={t(`notificationHealth.status.${health.status}`)}
        />
      </Box>

      {failing && (
        <Typography variant="body2" color="error">
          {t('notificationHealth.consecutiveFailures', { n: health.consecutive_failures })}
        </Typography>
      )}

      {health.status === 'no_devices' && (
        <Typography
          variant="body2"
          sx={{
            color: 'text.secondary',
          }}
        >
          {t('notificationHealth.noDevicesHint')}
        </Typography>
      )}

      <Typography
        variant="caption"
        sx={{
          color: 'text.secondary',
        }}
      >
        {t('notificationHealth.enabledUsers')}: {health.enabled_user_count}
      </Typography>
      <Typography
        variant="caption"
        sx={{
          color: 'text.secondary',
        }}
      >
        {t('notificationHealth.deliveredCount', {
          delivered: health.delivered_count,
          attempted: health.attempted_count,
        })}
      </Typography>

      {health.channel === 'push' && (
        <>
          <Typography
            variant="caption"
            sx={{
              color: 'text.secondary',
            }}
          >
            {t('notificationHealth.deviceCount', { n: health.device_count })}
          </Typography>
          {health.fcm_configured && (
            <Typography
              variant="caption"
              sx={{
                color: 'text.secondary',
              }}
            >
              {t('notificationHealth.fcmConfigured')}
            </Typography>
          )}
        </>
      )}

      <Typography
        variant="caption"
        sx={{
          color: 'text.secondary',
        }}
      >
        {t('notificationHealth.lastSuccess')}:{' '}
        {formatDateTime(health.last_sent_at) ?? t('notificationHealth.never')}
      </Typography>
      <Typography
        variant="caption"
        sx={{
          color: 'text.secondary',
        }}
      >
        {t('notificationHealth.lastFailure')}:{' '}
        {formatDateTime(health.last_failed_at) ?? t('notificationHealth.never')}
      </Typography>

      {failing && health.last_error && (
        <Typography variant="caption" color="error" sx={{ wordBreak: 'break-word', mt: 0.25 }}>
          {health.last_error}
        </Typography>
      )}
    </Box>
  );
}
