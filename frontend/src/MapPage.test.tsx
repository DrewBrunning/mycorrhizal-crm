import { cleanup, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import './i18n/config';
import type { MapPoint } from './api/map';

const h = vi.hoisted(() => ({
  getMapConfig: vi.fn(),
  getMapPoints: vi.fn(),
}));

vi.mock('./api/map', () => ({
  getMapConfig: h.getMapConfig,
  getMapPoints: h.getMapPoints,
}));
vi.mock('./components/ContactMap', () => ({
  default: (props: {
    styleUrl: string;
    points: MapPoint[];
    onOpenContact: (id: number) => void;
  }) => (
    <div data-testid="map-stub" data-style={props.styleUrl} data-count={props.points.length}>
      <button type="button" onClick={() => props.onOpenContact(42)}>
        open-42
      </button>
    </div>
  ),
}));

import MapPage from './MapPage';

afterEach(cleanup);
beforeEach(() => {
  vi.clearAllMocks();
});

function Where() {
  return <div data-testid="where">{useLocation().pathname}</div>;
}

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/map']}>
      <Routes>
        <Route path="/map" element={<MapPage />} />
        <Route path="*" element={<Where />} />
      </Routes>
    </MemoryRouter>,
  );
}

const pt: MapPoint = {
  contactId: 1,
  contactName: 'Ada',
  addressId: 'a',
  label: '',
  lat: 1,
  lng: 2,
};

test('shows a loading status, then the map with the configured style', async () => {
  h.getMapConfig.mockResolvedValue({ tile_style_url: 'https://tiles.example/s' });
  h.getMapPoints.mockResolvedValue([pt, { ...pt, contactId: 2 }]);
  renderPage();
  expect(screen.getByRole('status', { name: 'Loading…' })).toBeInTheDocument();
  const stub = await screen.findByTestId('map-stub');
  expect(stub).toHaveAttribute('data-style', 'https://tiles.example/s');
  expect(stub).toHaveAttribute('data-count', '2');
  expect(screen.getByRole('heading', { level: 1, name: 'Contact map' })).toBeInTheDocument();
  expect(screen.getByText('2 addresses on the map')).toBeInTheDocument();
  expect(screen.queryByRole('status')).not.toBeInTheDocument();
});

test('uses the singular count for one address', async () => {
  h.getMapConfig.mockResolvedValue({ tile_style_url: 's' });
  h.getMapPoints.mockResolvedValue([pt]);
  renderPage();
  expect(await screen.findByText('1 address on the map')).toBeInTheDocument();
});

test('explains how to get contacts onto the map when none have coordinates', async () => {
  h.getMapConfig.mockResolvedValue({ tile_style_url: 's' });
  h.getMapPoints.mockResolvedValue([]);
  renderPage();
  expect(await screen.findByText(/No contacts have coordinates yet/)).toBeInTheDocument();
  expect(screen.getByTestId('map-stub')).toHaveAttribute('data-count', '0');
});

test('shows an error and no map when loading fails', async () => {
  h.getMapConfig.mockRejectedValue(new Error('boom'));
  h.getMapPoints.mockResolvedValue([]);
  renderPage();
  expect(await screen.findByRole('alert')).toBeInTheDocument();
  expect(screen.queryByTestId('map-stub')).not.toBeInTheDocument();
  expect(screen.queryByRole('status')).not.toBeInTheDocument();
});

test('opening a contact from the map navigates to its page', async () => {
  h.getMapConfig.mockResolvedValue({ tile_style_url: 's' });
  h.getMapPoints.mockResolvedValue([pt]);
  renderPage();
  (await screen.findByText('open-42')).click();
  await waitFor(() => expect(screen.getByTestId('where')).toHaveTextContent('/contacts/42'));
});

test('does not update state after unmount', async () => {
  let resolve!: (v: { tile_style_url: string }) => void;
  h.getMapConfig.mockReturnValue(new Promise((r) => (resolve = r)));
  h.getMapPoints.mockResolvedValue([]);
  const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
  const { unmount } = renderPage();
  unmount();
  resolve({ tile_style_url: 's' });
  await Promise.resolve();
  expect(errorSpy).not.toHaveBeenCalled();
  errorSpy.mockRestore();
});

test('ignores a failure that arrives after unmount', async () => {
  let reject!: (e: Error) => void;
  h.getMapConfig.mockReturnValue(new Promise((_, r) => (reject = r)));
  h.getMapPoints.mockResolvedValue([]);
  const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
  const { unmount } = renderPage();
  unmount();
  reject(new Error('late'));
  await Promise.resolve();
  await Promise.resolve();
  expect(errorSpy).not.toHaveBeenCalled();
  errorSpy.mockRestore();
});
