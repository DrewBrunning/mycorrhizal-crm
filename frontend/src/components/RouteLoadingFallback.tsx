import { Box, CircularProgress } from '@mui/material';
import { useTranslation } from 'react-i18next';

// #211: shared Suspense fallback for every route in App.tsx. `role="status"`
// + an accessible name means a route change announces itself to assistive
// tech instead of loading silently; extracted to one place rather than
// repeating the markup (and the role) at all 15 call sites.
export default function RouteLoadingFallback() {
  const { t } = useTranslation();
  return (
    <Box
      role="status"
      aria-label={t('common.loading')}
      sx={{
        display: 'flex',
        justifyContent: 'center',
        mt: 4,
      }}
    >
      <CircularProgress />
    </Box>
  );
}
