import { Box, List, ListItem, Link as MuiLink, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Link as RouterLink } from 'react-router';
import type { MapPoint } from '../api/map';

interface MapPointListProps {
  points: MapPoint[];
}

// The non-visual twin of ContactMap (issue #1286): every plotted address as a
// keyboard-navigable link to its contact. The MapLibre canvas is not reachable
// by keyboard or screen reader, and the list must still work when WebGL is
// missing, so MapPage renders this independently of the map.
export default function MapPointList({ points }: MapPointListProps) {
  const { t } = useTranslation();
  if (points.length === 0) return null;
  return (
    <Box component="section" aria-labelledby="map-point-list-heading" sx={{ mt: 3 }}>
      <Typography variant="h5" component="h2" id="map-point-list-heading" gutterBottom>
        {t('map.listHeading')}
      </Typography>
      <List dense disablePadding data-testid="map-point-list">
        {points.map((p) => (
          <ListItem key={`${p.contactId}:${p.addressId}`} disableGutters>
            <MuiLink component={RouterLink} to={`/contacts/${p.contactId}`}>
              {p.contactName || t('map.unnamed')}
              {p.label ? ` — ${p.label}` : ''}
            </MuiLink>
          </ListItem>
        ))}
      </List>
    </Box>
  );
}
