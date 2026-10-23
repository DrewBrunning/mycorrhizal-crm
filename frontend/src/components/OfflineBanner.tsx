import { Alert } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { useOnlineStatus } from '../hooks/useOnlineStatus';

/**
 * The app-shell "you are offline" state (issue #1915). The service worker may
 * still serve the cached shell while every `/api/v1/*` request fails, so
 * without this an offline reload looked like a server error. role="status"
 * makes the change polite rather than an interrupting alert, and the banner
 * unmounts itself as soon as the browser reports the connection is back.
 */
export default function OfflineBanner() {
  const { t } = useTranslation();
  const online = useOnlineStatus();

  if (online) return null;

  return (
    <Alert role="status" severity="info" sx={{ mb: 2 }}>
      {t('app.offline.banner')}
    </Alert>
  );
}
