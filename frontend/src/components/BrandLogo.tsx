import { Box } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { useThemePreference } from '../AppThemeProvider';

interface BrandLogoProps {
  height?: number | string;
  width?: number | string;
}

export default function BrandLogo({ height = 'auto', width = 'auto' }: BrandLogoProps) {
  const { t } = useTranslation();
  const { mode } = useThemePreference();
  const src =
    mode === 'dark' ? '/mycorrhizal-logo-dark_512.png' : '/mycorrhizal-logo-light_512.png';

  // #2536: the accessible name is UI copy like any other — it was the one
  // hardcoded string the pseudo-locale pass found in the app shell. `app.title`
  // is the app's own name, so the value is the same in every locale.
  return (
    <Box component="img" src={src} alt={t('app.title')} sx={{ height, width, flexShrink: 0 }} />
  );
}
