import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import type { MapPoint } from '../api/map';

// maplibre-gl needs WebGL, which jsdom lacks. The stub records what
// ContactMap asks of it.
const h = vi.hoisted(() => ({
  mapCtor: vi.fn(),
  remove: vi.fn(),
  setCenter: vi.fn(),
  setZoom: vi.fn(),
  fitBounds: vi.fn(),
  addControl: vi.fn(),
  markers: [] as { lngLat?: [number, number]; popupContent?: HTMLElement }[],
  boundsExtend: vi.fn(),
  throwOnConstruct: false,
}));

vi.mock('maplibre-gl/dist/maplibre-gl.css', () => ({}));
vi.mock('maplibre-gl', () => ({
  Map: class {
    constructor(opts: unknown) {
      if (h.throwOnConstruct) throw new Error('no webgl');
      h.mapCtor(opts);
    }
    addControl = h.addControl;
    setCenter = h.setCenter;
    setZoom = h.setZoom;
    fitBounds = h.fitBounds;
    remove = h.remove;
  },
  NavigationControl: class {},
  LngLatBounds: class {
    extend = h.boundsExtend;
  },
  Popup: class {
    content?: HTMLElement;
    setDOMContent(el: HTMLElement) {
      this.content = el;
      return this;
    }
  },
  Marker: class {
    rec: { lngLat?: [number, number]; popupContent?: HTMLElement } = {};
    constructor() {
      h.markers.push(this.rec);
    }
    setLngLat(ll: [number, number]) {
      this.rec.lngLat = ll;
      return this;
    }
    setPopup(p: { content?: HTMLElement }) {
      this.rec.popupContent = p.content;
      return this;
    }
    addTo() {
      return this;
    }
  },
}));

import ContactMap from './ContactMap';

afterEach(cleanup);
beforeEach(() => {
  vi.clearAllMocks();
  h.markers.length = 0;
  h.throwOnConstruct = false;
});

const point = (patch: Partial<MapPoint> = {}): MapPoint => ({
  contactId: 1,
  contactName: 'Ada <b>Lovelace</b>',
  addressId: 'a1',
  label: '1 Main St',
  lat: 51.5,
  lng: -0.12,
  ...patch,
});

test('creates the map with the configured style and a navigation control', () => {
  render(<ContactMap styleUrl="https://tiles.example/style" points={[]} onOpenContact={vi.fn()} />);
  expect(h.mapCtor).toHaveBeenCalledWith(
    expect.objectContaining({ style: 'https://tiles.example/style' }),
  );
  expect(h.addControl).toHaveBeenCalledTimes(1);
  expect(screen.getByRole('region', { name: 'Contact map' })).toBeInTheDocument();
  expect(h.fitBounds).not.toHaveBeenCalled();
  expect(h.setCenter).not.toHaveBeenCalled();
});

test('a single point centers and zooms in instead of fitting bounds', () => {
  render(<ContactMap styleUrl="s" points={[point()]} onOpenContact={vi.fn()} />);
  expect(h.markers).toHaveLength(1);
  expect(h.markers[0].lngLat).toEqual([-0.12, 51.5]);
  expect(h.setCenter).toHaveBeenCalledWith([-0.12, 51.5]);
  expect(h.setZoom).toHaveBeenCalledWith(10);
  expect(h.fitBounds).not.toHaveBeenCalled();
});

test('several points fit the map to their bounds', () => {
  render(
    <ContactMap
      styleUrl="s"
      points={[point(), point({ contactId: 2, lat: 40, lng: -74 })]}
      onOpenContact={vi.fn()}
    />,
  );
  expect(h.markers).toHaveLength(2);
  expect(h.boundsExtend).toHaveBeenCalledTimes(2);
  expect(h.fitBounds).toHaveBeenCalledTimes(1);
});

test('popup content is built from text nodes (no HTML injection) and opens the contact', () => {
  const onOpen = vi.fn();
  render(<ContactMap styleUrl="s" points={[point({ contactId: 9 })]} onOpenContact={onOpen} />);
  const content = h.markers[0].popupContent as HTMLElement;
  expect(content.querySelector('b')).toBeNull();
  expect(content.querySelector('strong')?.textContent).toBe('Ada <b>Lovelace</b>');
  expect(content.textContent).toContain('1 Main St');
  (content.querySelector('button') as HTMLButtonElement).click();
  expect(onOpen).toHaveBeenCalledWith(9);
});

test('omits the address line when the label is empty', () => {
  render(<ContactMap styleUrl="s" points={[point({ label: '' })]} onOpenContact={vi.fn()} />);
  const content = h.markers[0].popupContent as HTMLElement;
  expect(content.querySelectorAll('div')).toHaveLength(0);
});

test('removes the map on unmount', () => {
  const { unmount } = render(<ContactMap styleUrl="s" points={[]} onOpenContact={vi.fn()} />);
  unmount();
  expect(h.remove).toHaveBeenCalledTimes(1);
});

test('shows an error instead of crashing when the map cannot be constructed', () => {
  h.throwOnConstruct = true;
  render(<ContactMap styleUrl="s" points={[point()]} onOpenContact={vi.fn()} />);
  expect(screen.getByRole('alert')).toHaveTextContent(/could not be displayed/i);
});

test('a style change after a construction failure does not retry without a container', () => {
  h.throwOnConstruct = true;
  const { rerender } = render(<ContactMap styleUrl="a" points={[]} onOpenContact={vi.fn()} />);
  expect(screen.getByRole('alert')).toBeInTheDocument();
  h.throwOnConstruct = false;
  rerender(<ContactMap styleUrl="b" points={[]} onOpenContact={vi.fn()} />);
  expect(h.mapCtor).not.toHaveBeenCalled();
  expect(screen.getByRole('alert')).toBeInTheDocument();
});
