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
vi.mock('./utils/errorHandler', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./utils/errorHandler')>();
  return { ...actual, handleFetchError: vi.fn(actual.handleFetchError) };
});
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
import { handleFetchError } from './utils/errorHandler';

const handleFetchErrorMock = vi.mocked(handleFetchError);

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
  h.getMapPoints.mockResolvedValue({ points: [pt, { ...pt, contactId: 2 }], truncated: false });
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
  h.getMapPoints.mockResolvedValue({ points: [pt], truncated: false });
  renderPage();
  expect(await screen.findByText('1 address on the map')).toBeInTheDocument();
});

test('explains how to get contacts onto the map when none have coordinates', async () => {
  h.getMapConfig.mockResolvedValue({ tile_style_url: 's' });
  h.getMapPoints.mockResolvedValue({ points: [], truncated: false });
  renderPage();
  expect(await screen.findByText(/No contacts have coordinates yet/)).toBeInTheDocument();
  expect(screen.getByTestId('map-stub')).toHaveAttribute('data-count', '0');
});

test('shows an error and no map when loading fails', async () => {
  h.getMapConfig.mockRejectedValue(new Error('boom'));
  h.getMapPoints.mockResolvedValue({ points: [], truncated: false });
  renderPage();
  expect(await screen.findByRole('alert')).toBeInTheDocument();
  expect(screen.queryByTestId('map-stub')).not.toBeInTheDocument();
  expect(screen.queryByRole('status')).not.toBeInTheDocument();
  // Mounted control: the same handler the unmount test asserts is not called.
  expect(handleFetchErrorMock).toHaveBeenCalledTimes(1);
});

test('applies a delayed response that arrives while still mounted', async () => {
  let resolveConfig!: (v: { tile_style_url: string }) => void;
  let resolvePoints!: (v: { points: MapPoint[]; truncated: boolean }) => void;
  h.getMapConfig.mockReturnValue(new Promise((r) => (resolveConfig = r)));
  h.getMapPoints.mockReturnValue(new Promise((r) => (resolvePoints = r)));
  renderPage();
  expect(screen.getByRole('status', { name: 'Loading…' })).toBeInTheDocument();

  resolveConfig({ tile_style_url: 's' });
  resolvePoints({ points: [pt], truncated: false });

  const stub = await screen.findByTestId('map-stub');
  expect(stub).toHaveAttribute('data-count', '1');
  expect(handleFetchErrorMock).not.toHaveBeenCalled();
});

test('opening a contact from the map navigates to its page', async () => {
  h.getMapConfig.mockResolvedValue({ tile_style_url: 's' });
  h.getMapPoints.mockResolvedValue({ points: [pt], truncated: false });
  renderPage();
  (await screen.findByText('open-42')).click();
  await waitFor(() => expect(screen.getByTestId('where')).toHaveTextContent('/contacts/42'));
});

test('does not update state after unmount', async () => {
  let resolve!: (v: { tile_style_url: string }) => void;
  h.getMapConfig.mockReturnValue(new Promise((r) => (resolve = r)));
  h.getMapPoints.mockResolvedValue({ points: [], truncated: false });
  const { unmount } = renderPage();
  unmount();
  resolve({ tile_style_url: 's' });
  await Promise.resolve();
  await Promise.resolve();
  // No error handler, no rendered map: the cancelled-response path is inert.
  expect(handleFetchErrorMock).not.toHaveBeenCalled();
  expect(screen.queryByTestId('map-stub')).not.toBeInTheDocument();
});

test('ignores a failure that arrives after unmount', async () => {
  let reject!: (e: Error) => void;
  h.getMapConfig.mockReturnValue(new Promise((_, r) => (reject = r)));
  h.getMapPoints.mockResolvedValue({ points: [], truncated: false });
  const { unmount } = renderPage();
  unmount();
  reject(new Error('late'));
  await Promise.resolve();
  await Promise.resolve();
  expect(handleFetchErrorMock).not.toHaveBeenCalled();
});

test('warns when the server truncated the point list', async () => {
  h.getMapConfig.mockResolvedValue({ tile_style_url: 's' });
  h.getMapPoints.mockResolvedValue({ points: [pt], truncated: true });
  renderPage();
  expect(await screen.findByText(/Showing the first 1 address only/)).toBeInTheDocument();
  expect(screen.getByTestId('map-stub')).toBeInTheDocument();
});

test('shows no truncation warning for a complete list', async () => {
  h.getMapConfig.mockResolvedValue({ tile_style_url: 's' });
  h.getMapPoints.mockResolvedValue({ points: [pt], truncated: false });
  renderPage();
  await screen.findByTestId('map-stub');
  expect(screen.queryByText(/Showing the first/)).not.toBeInTheDocument();
});

test('renders a keyboard-reachable list of contacts under a heading, linking to each contact', async () => {
  h.getMapConfig.mockResolvedValue({ tile_style_url: 's' });
  h.getMapPoints.mockResolvedValue({
    points: [
      { ...pt, contactId: 7, contactName: 'Ada Lovelace', label: '1 Main St' },
      { ...pt, contactId: 8, addressId: 'b', contactName: '' },
    ],
    truncated: false,
  });
  renderPage();
  expect(
    await screen.findByRole('heading', { level: 2, name: 'Contacts on the map' }),
  ).toBeInTheDocument();
  expect(screen.getByRole('link', { name: /Ada Lovelace/ })).toHaveAttribute('href', '/contacts/7');
  expect(screen.getByRole('link', { name: /1 Main St/ })).toBeInTheDocument();
  expect(screen.getByRole('link', { name: 'Unnamed contact' })).toHaveAttribute(
    'href',
    '/contacts/8',
  );
});

test('shows no list when there are no points', async () => {
  h.getMapConfig.mockResolvedValue({ tile_style_url: 's' });
  h.getMapPoints.mockResolvedValue({ points: [], truncated: false });
  renderPage();
  await screen.findByTestId('map-stub');
  expect(screen.queryByRole('heading', { level: 2 })).not.toBeInTheDocument();
});
