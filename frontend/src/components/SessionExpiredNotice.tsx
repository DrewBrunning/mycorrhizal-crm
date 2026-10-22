import { Alert, Button } from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { onSessionExpired, onSessionRestored, requestSessionSignIn } from '../api/sessionExpiry';

// Issue #1793: the only working "Sign in" used to be the small toast, while
// the routed page showed an inline "Try Again" that could never succeed. This
// in-flow notice sits at the top of the app's main content and opens the same
// re-authentication dialog, so the action is where the user is looking.
export default function SessionExpiredNotice() {
  const { t } = useTranslation();
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    const offExpired = onSessionExpired(() => setVisible(true));
    const offRestored = onSessionRestored(() => setVisible(false));
    return () => {
      offExpired();
      offRestored();
    };
  }, []);

  if (!visible) return null;

  return (
    <Alert
      severity="warning"
      sx={{ mb: 2 }}
      action={
        <Button color="inherit" size="small" onClick={requestSessionSignIn}>
          {t('sessionExpired.signIn')}
        </Button>
      }
    >
      {t('sessionExpired.bannerMessage')} {t('sessionExpired.description')}
    </Alert>
  );
}
