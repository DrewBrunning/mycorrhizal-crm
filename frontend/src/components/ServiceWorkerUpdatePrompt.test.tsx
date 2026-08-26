import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { notifyUpdateAvailable, resetServiceWorkerUpdatesForTest } from '../serviceWorkerUpdates';
import ServiceWorkerUpdatePrompt from './ServiceWorkerUpdatePrompt';

const applyUpdate = vi.hoisted(() => vi.fn());

vi.mock('../serviceWorkerUpdates', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../serviceWorkerUpdates')>();
  return { ...actual, applyUpdate };
});

beforeEach(() => {
  resetServiceWorkerUpdatesForTest();
  applyUpdate.mockClear();
});

afterEach(cleanup);

function fakeRegistration() {
  return { waiting: { postMessage: vi.fn() } } as unknown as ServiceWorkerRegistration;
}

test('renders nothing until an update is available', () => {
  render(<ServiceWorkerUpdatePrompt />);

  expect(screen.queryByText(/new version/i)).not.toBeInTheDocument();
});

test('offers a reload once a new build has been precached', () => {
  render(<ServiceWorkerUpdatePrompt />);

  act(() => notifyUpdateAvailable(fakeRegistration()));

  expect(screen.getByText('A new version of Mycorrhizal CRM is available.')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Reload' })).toBeInTheDocument();
});

test('applies the update for the registration it was notified about', () => {
  const registration = fakeRegistration();
  render(<ServiceWorkerUpdatePrompt />);

  act(() => notifyUpdateAvailable(registration));
  fireEvent.click(screen.getByRole('button', { name: 'Reload' }));

  expect(applyUpdate).toHaveBeenCalledWith(registration);
});

// The prompt mounts with the rest of the tree, but register() fires on window's
// load event and can beat it. This is the ordering that would otherwise leave a
// user on a stale bundle with nothing on screen telling them so.
test('shows an update that arrived before it mounted', () => {
  notifyUpdateAvailable(fakeRegistration());

  render(<ServiceWorkerUpdatePrompt />);

  expect(screen.getByText('A new version of Mycorrhizal CRM is available.')).toBeInTheDocument();
});
