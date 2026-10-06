import { Alert, Box } from '@mui/material';
import {
  type GeoJSONSource,
  LngLatBounds,
  Map as MapLibreMap,
  NavigationControl,
  Popup,
} from 'maplibre-gl';
import 'maplibre-gl/dist/maplibre-gl.css';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { MapPoint } from '../api/map';

interface ContactMapProps {
  styleUrl: string;
  points: MapPoint[];
  onOpenContact: (contactId: number) => void;
}

// The MapLibre canvas. Popups are plain DOM-built (textContent only, never
// innerHTML) because contact names and addresses are user data. The canvas is
// not keyboard/screen-reader reachable; MapPointList is the accessible twin.
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

    const SOURCE = 'contacts';
    const popupFor = (p: MapPoint) => {
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
      return new Popup({ offset: 12 }).setLngLat([p.lng, p.lat]).setDOMContent(popupContent);
    };

    // One GeoJSON source + circle layers instead of a DOM Marker per point:
    // up to MaxContactMapPoints (5000) points render on the GPU and nearby
    // ones cluster. Features carry only the array index; the popup is built
    // from the typed point with textContent (contact data is user input).
    map.on('load', () => {
      map.addSource(SOURCE, {
        type: 'geojson',
        cluster: true,
        clusterRadius: 40,
        clusterMaxZoom: 13,
        data: {
          type: 'FeatureCollection',
          features: points.map((p, idx) => ({
            type: 'Feature' as const,
            properties: { idx },
            geometry: { type: 'Point' as const, coordinates: [p.lng, p.lat] },
          })),
        },
      });
      map.addLayer({
        id: 'contact-clusters',
        type: 'circle',
        source: SOURCE,
        filter: ['has', 'point_count'],
        paint: {
          'circle-color': '#2e7d32',
          'circle-radius': ['step', ['get', 'point_count'], 14, 10, 18, 100, 24],
          'circle-stroke-width': 2,
          'circle-stroke-color': '#ffffff',
        },
      });
      map.addLayer({
        id: 'contact-points',
        type: 'circle',
        source: SOURCE,
        filter: ['!', ['has', 'point_count']],
        paint: {
          'circle-color': '#2e7d32',
          'circle-radius': 8,
          'circle-stroke-width': 2,
          'circle-stroke-color': '#ffffff',
        },
      });
    });

    map.on('click', 'contact-points', (e) => {
      const idx = e.features?.[0]?.properties?.idx;
      const p = typeof idx === 'number' ? points[idx] : undefined;
      if (p) popupFor(p).addTo(map);
    });
    map.on('click', 'contact-clusters', (e) => {
      const feature = e.features?.[0];
      if (!feature || feature.geometry.type !== 'Point') return;
      const center = feature.geometry.coordinates as [number, number];
      const source = map.getSource(SOURCE) as GeoJSONSource | undefined;
      void source?.getClusterExpansionZoom(feature.properties?.cluster_id).then((zoom) => {
        map.easeTo({ center, zoom });
      });
    });
    for (const layer of ['contact-points', 'contact-clusters']) {
      map.on('mouseenter', layer, () => {
        map.getCanvas().style.cursor = 'pointer';
      });
      map.on('mouseleave', layer, () => {
        map.getCanvas().style.cursor = '';
      });
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
