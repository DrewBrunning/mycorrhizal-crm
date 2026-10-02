import { Alert, Box, CircularProgress, Typography } from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { getMapConfig, getMapPoints, type MapPoint } from './api/map';
import ContactMap from './components/ContactMap';
import { useDocumentTitle } from './hooks/useDocumentTitle';
import { handleFetchError } from './utils/errorHandler';

// ADR 0031 / issue #1286. Rendered through React.lazy in App.tsx -- the first
// lazy route in this codebase -- so MapLibre's weight (the `map-vendor` chunk)
// is only fetched when someone opens the map.
export default function MapPage() {
  const { t } = useTranslation();
  useDocumentTitle(t('map.title'));
  const navigate = useNavigate();
  const [styleUrl, setStyleUrl] = useState<string | null>(null);
  const [points, setPoints] = useState<MapPoint[] | null>(null);
  const [truncated, setTruncated] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const [config, pts] = await Promise.all([getMapConfig(), getMapPoints()]);
        if (cancelled) return;
        setStyleUrl(config.tile_style_url);
        setPoints(pts.points);
        setTruncated(pts.truncated);
      } catch (err) {
        if (cancelled) return;
        setError(handleFetchError(err, 'loading the contact map'));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <Box>
      <Typography variant="h4" component="h1" gutterBottom>
        {t('map.title')}
      </Typography>
      {error && <Alert severity="error">{error}</Alert>}
      {!error && (styleUrl === null || points === null) && (
        <Box role="status" aria-label={t('common.loading')} sx={{ display: 'flex', mt: 4 }}>
          <CircularProgress />
        </Box>
      )}
      {!error && styleUrl !== null && points !== null && (
        <>
          {points.length === 0 ? (
            <Alert severity="info" sx={{ mb: 2 }}>
              {t('map.empty')}
            </Alert>
          ) : (
            <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
              {t('map.count', { count: points.length })}
            </Typography>
          )}
          {truncated && (
            <Alert severity="warning" sx={{ mb: 2 }}>
              {t('map.truncated', { count: points.length })}
            </Alert>
          )}
          <ContactMap
            styleUrl={styleUrl}
            points={points}
            onOpenContact={(id) => void navigate(`/contacts/${id}`)}
          />
        </>
      )}
    </Box>
  );
}
