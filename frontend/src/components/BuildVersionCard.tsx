import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
import {
  Box,
  Card,
  CardContent,
  Chip,
  Divider,
  IconButton,
  Tooltip,
  Typography,
} from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { formatBuildVersion, getHealth, type HealthResponse } from '../api/health';
import { getSystemStatus } from '../api/systemStatus';
import { isAdmin } from '../auth';
import { useSnackbar } from '../context/SnackbarContext';

/**
 * Shows which build is running, so a user filing a bug report can say which
 * one they are on.
 *
 * /health used to report a hardcoded "0.1.0" for every build ever made, and
 * nothing surfaced it in the UI regardless. With several alpha candidates in
 * circulation that made reports untraceable to a binary.
 *
 * Self-contained (own fetch, own state) rather than more state on the already
 * 570-line SettingsPage — nothing else on that page needs this data.
 *
 * When the operator opted into the update-availability check
 * (UPDATE_CHECK_ENABLED, issue #650) and the running build is behind the
 * latest release, a small "Update available" chip is shown next to the
 * version. The check is admin-only on the backend, so non-admins never fetch
 * it and nobody sees the chip when the flag is off or the lookup failed.
 */
export default function BuildVersionCard() {
  const { t } = useTranslation();
  const { showSuccess } = useSnackbar();
  const [health, setHealth] = useState<HealthResponse | null>(null);
  const [failed, setFailed] = useState(false);
  const [updateAvailable, setUpdateAvailable] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    getHealth()
      .then((data) => {
        if (!cancelled) setHealth(data);
      })
      .catch(() => {
        // A failed version lookup must never take over the settings page —
        // it is informational. Render a dash instead.
        if (!cancelled) setFailed(true);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    // The system-status endpoint is admin-only; skip the (guaranteed 403)
    // call for everyone else. Both the data and the chip are informational —
    // any failure renders nothing, never an error.
    if (!isAdmin()) return;
    let cancelled = false;
    getSystemStatus()
      .then((data) => {
        if (!cancelled && data.update?.update_available && data.update.latest) {
          setUpdateAvailable(data.update.latest);
        }
      })
      .catch(() => {
        // informational — render nothing
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const versionText = health ? formatBuildVersion(health) : failed ? '—' : '…';

  const handleCopy = async () => {
    if (!health) return;
    const details = [
      `version: ${health.version}`,
      health.commit ? `commit: ${health.commit}` : null,
      health.build_date ? `built: ${health.build_date}` : null,
    ]
      .filter(Boolean)
      .join('\n');
    await navigator.clipboard.writeText(details);
    showSuccess(t('settings.about.copied'));
  };

  return (
    <Card sx={{ mb: 2 }}>
      <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
        <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
          <InfoOutlinedIcon sx={{ mr: 1, color: 'text.secondary', fontSize: 20 }} />
          <Typography variant="subtitle1" component="h2" sx={{ fontWeight: 500 }}>
            {t('settings.about.title')}
          </Typography>
        </Box>
        <Divider sx={{ mb: 1.5 }} />

        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
          <Typography variant="body2" color="text.secondary">
            {t('settings.about.version')}
          </Typography>
          <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
            {versionText}
          </Typography>
          {updateAvailable && (
            <Chip
              size="small"
              color="primary"
              label={t('settings.about.updateAvailable', { version: updateAvailable })}
            />
          )}
          {health && (
            <Tooltip title={t('settings.about.copy')}>
              <IconButton size="small" onClick={handleCopy} aria-label={t('settings.about.copy')}>
                <ContentCopyIcon fontSize="inherit" />
              </IconButton>
            </Tooltip>
          )}
        </Box>

        {health?.build_date && (
          <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5 }}>
            {t('settings.about.built', { date: health.build_date })}
          </Typography>
        )}

        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5 }}>
          {t('settings.about.reportHint')}
        </Typography>
      </CardContent>
    </Card>
  );
}
