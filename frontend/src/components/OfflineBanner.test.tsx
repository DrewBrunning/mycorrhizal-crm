import { act, cleanup, render, screen } from '@testing-library/react';
import { afterEach, expect, test } from 'vitest';
import '../i18n/config';
import i18n from '../i18n/config';
import OfflineBanner from './OfflineBanner';

const originalOnLine = Object.getOwnPropertyDescriptor(window.navigator, 'onLine');

function setOnline(value: boolean) {
  Object.defineProperty(window.navigator, 'onLine', { configurable: true, value });
}

afterEach(() => {
  cleanup();
  if (originalOnLine) {
    Object.defineProperty(window.navigator, 'onLine', originalOnLine);
  } else {
    delete (window.navigator as unknown as { onLine?: boolean }).onLine;
  }
});

test('renders nothing while the browser is online', () => {
  setOnline(true);
  render(<OfflineBanner />);

  expect(screen.queryByText(i18n.t('app.offline.banner'))).not.toBeInTheDocument();
});

test('renders a polite status banner as soon as the browser is offline', () => {
  setOnline(false);
  render(<OfflineBanner />);

  const banner = screen.getByRole('status');
  expect(banner).toHaveTextContent(i18n.t('app.offline.banner'));
});

test('removes the banner when the connection comes back', () => {
  setOnline(false);
  render(<OfflineBanner />);
  expect(screen.getByRole('status')).toBeInTheDocument();

  act(() => {
    setOnline(true);
    window.dispatchEvent(new Event('online'));
  });

  expect(screen.queryByRole('status')).not.toBeInTheDocument();
});
