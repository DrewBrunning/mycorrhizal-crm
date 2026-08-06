import { useEffect, useState } from 'react';
import { Snackbar, Alert, Button } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { onUpdateAvailable, applyUpdate } from '../serviceWorkerUpdates';

// ServiceWorkerUpdatePrompt tells the user a new build has been precached and
// offers to switch to it.
//
// It exists because registering the service worker (rather than CRA's default
// unregister()) means the app is served cache-first: without a prompt, a user
// with the app open stays on the old bundle until every tab is closed, which
// for a CRM someone leaves open all day can be days. The shared SnackbarContext
// is not reused here -- it auto-hides and has no action slot, and this notice
// must persist until acted on.
export default function ServiceWorkerUpdatePrompt() {
  const { t } = useTranslation();
  const [registration, setRegistration] = useState<ServiceWorkerRegistration | null>(null);

  useEffect(() => onUpdateAvailable(setRegistration), []);

  if (!registration) {
    return null;
  }

  return (
    <Snackbar
      open
      anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
      // No autoHideDuration: an update the user never acted on should keep
      // asking, not disappear after six seconds.
    >
      <Alert
        severity="info"
        variant="filled"
        sx={{ width: '100%' }}
        action={
          <Button color="inherit" size="small" onClick={() => applyUpdate(registration)}>
            {t('app.update.reload')}
          </Button>
        }
      >
        {t('app.update.available')}
      </Alert>
    </Snackbar>
  );
}
