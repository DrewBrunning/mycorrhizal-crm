import { Alert, AlertTitle, Button } from '@mui/material';
import type { SxProps, Theme } from '@mui/material/styles';
import { useTranslation } from 'react-i18next';

interface LoadErrorProps {
  /** The error message from the failed load (already carries the request ref for 5xx). */
  message: string;
  /** Re-runs the failed load in place, no page reload. Omit to hide the Retry action. */
  onRetry?: () => void;
  sx?: SxProps<Theme>;
}

/**
 * The one shared "this data failed to load" state (issue #1779).
 *
 * Render it in place of the list AND its empty-state copy: an empty state is only
 * true once a load has succeeded, so a page shows either this or "No ... yet",
 * never both. `Alert` carries role="alert", so screen readers announce it as it
 * appears.
 */
export default function LoadError({ message, onRetry, sx }: LoadErrorProps) {
  const { t } = useTranslation();
  return (
    <Alert
      severity="error"
      sx={sx}
      action={
        onRetry ? (
          <Button color="inherit" size="small" onClick={onRetry}>
            {t('common.retry')}
          </Button>
        ) : undefined
      }
    >
      <AlertTitle>{t('common.loadFailed')}</AlertTitle>
      {message}
    </Alert>
  );
}
