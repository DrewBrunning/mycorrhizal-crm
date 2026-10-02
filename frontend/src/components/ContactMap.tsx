import { Alert, Box } from '@mui/material';
import { LngLatBounds, Map as MapLibreMap, Marker, NavigationControl, Popup } from 'maplibre-gl';
import 'maplibre-gl/dist/maplibre-gl.css';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { MapPoint } from '../api/map';

interface ContactMapProps {
  styleUrl: string;
  points: MapPoint[];
  onOpenContact: (contactId: number) => void;
}

// The MapLibre canvas. Markers are plain DOM-built popups (textContent only,
// never innerHTML) because contact names and addresses are user data.
export default function ContactMap({ styleUrl, points, onOpenContact }: ContactMapProps) {
  const { t } = useTranslation();
  const containerRef = useRef<HTMLDivElement>(null);
  const openRef = useRef(onOpenContact);
  openRef.current = onOpenContact;
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    let map: MapLibreMap;
    try {
      map = new MapLibreMap({ container, style: styleUrl, center: [0, 20], zoom: 1 });
    } catch {
      // No WebGL (or a broken style) throws synchronously at construction.
      setFailed(true);
      return;
    }
    setFailed(false);
    map.addControl(new NavigationControl(), 'top-right');

    for (const p of points) {
      const popupContent = document.createElement('div');
      const name = document.createElement('strong');
      name.textContent = p.contactName;
      popupContent.appendChild(name);
      if (p.label) {
        const label = document.createElement('div');
        label.textContent = p.label;
        popupContent.appendChild(label);
      }
      const open = document.createElement('button');
      open.type = 'button';
      open.textContent = t('map.openContact');
      open.addEventListener('click', () => openRef.current(p.contactId));
      popupContent.appendChild(open);

      new Marker()
        .setLngLat([p.lng, p.lat])
        .setPopup(new Popup({ offset: 24 }).setDOMContent(popupContent))
        .addTo(map);
    }

    if (points.length === 1) {
      map.setCenter([points[0].lng, points[0].lat]);
      map.setZoom(10);
    } else if (points.length > 1) {
      const bounds = new LngLatBounds();
      for (const p of points) bounds.extend([p.lng, p.lat]);
      map.fitBounds(bounds, { padding: 48, maxZoom: 12, animate: false });
    }

    return () => map.remove();
  }, [styleUrl, points, t]);

  if (failed) return <Alert severity="error">{t('map.unavailable')}</Alert>;
  return (
    <Box
      ref={containerRef}
      role="region"
      aria-label={t('map.title')}
      data-testid="contact-map"
      sx={{
        width: '100%',
        height: { xs: '60vh', md: '70vh' },
        borderRadius: 1,
        overflow: 'hidden',
      }}
    />
  );
}
