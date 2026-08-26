import { render } from '@testing-library/react';
import { expect, test } from 'vitest';
import App from './App';
import { AppThemeProvider } from './AppThemeProvider';
import { AnnouncerProvider } from './context/AnnouncerContext';
import { SnackbarProvider } from './context/SnackbarContext';
import { DateFormatProvider } from './DateFormatProvider';

// Mirrors index.tsx's real provider composition -- App itself doesn't
// include these, they're only added at the root render call, so any
// component that reaches into their context (e.g. BrandLogo's
// useThemePreference, or a routed page's useAnnouncer()) needs the same
// wrapping here to render at all.
test('renders app component', () => {
  const { container } = render(
    <AppThemeProvider>
      <DateFormatProvider>
        <SnackbarProvider>
          <AnnouncerProvider>
            <App />
          </AnnouncerProvider>
        </SnackbarProvider>
      </DateFormatProvider>
    </AppThemeProvider>,
  );
  expect(container).toBeTruthy();
});
