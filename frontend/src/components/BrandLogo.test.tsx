import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, expect, test } from 'vitest';
import { AppThemeProvider } from '../AppThemeProvider';
// Registers the app's i18n instance: the logo resolves `app.title` through it.
import '../i18n/config';
import en from '../i18n/locales/en.json';
import BrandLogo from './BrandLogo';

// #2536: the logo's accessible name was the one hardcoded string the
// pseudo-locale pass found in the app shell. It is UI copy like any other and
// now comes from `app.title` -- so a brand rename reaches it, and the qps
// pseudo-locale renders it bracketed like every other translated string.
// (The value is a proper noun, so it is identical in every locale.)
afterEach(cleanup);

test('names the logo from the app title translation', () => {
  render(
    <AppThemeProvider>
      <BrandLogo />
    </AppThemeProvider>,
  );

  expect(screen.getByRole('img')).toHaveAttribute('alt', en.app.title);
});
