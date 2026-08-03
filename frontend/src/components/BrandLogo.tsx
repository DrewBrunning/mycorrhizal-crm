import { Box } from '@mui/material';
import { useThemePreference } from '../AppThemeProvider';

interface BrandLogoProps {
  height?: number | string;
  width?: number | string;
}

export default function BrandLogo({ height = 'auto', width = '100%' }: BrandLogoProps) {
  const { mode } = useThemePreference();
  const src = mode === 'dark' ? '/mycorrhizal-logo-dark_512.png' : '/mycorrhizal-logo-light_512.png';

  return (
    <Box
      component="img"
      src={src}
      alt="Mycorrhizal CRM"
      sx={{ height, width, flexShrink: 0 }}
    />
  );
}
