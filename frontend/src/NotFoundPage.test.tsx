import { cleanup, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, expect, test } from 'vitest';
import './i18n/config';
import { AppThemeProvider } from './AppThemeProvider';
import NotFoundPage from './NotFoundPage';

afterEach(cleanup);

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/definitely-not-a-route']}>
      <AppThemeProvider>
        <NotFoundPage />
      </AppThemeProvider>
    </MemoryRouter>,
  );
}

test('renders a titled not-found state with routes back into the app', () => {
  renderPage();

  expect(screen.getByRole('heading', { level: 1, name: 'Page not found' })).toBeInTheDocument();
  expect(screen.getByText(/this page doesn't exist/i)).toBeInTheDocument();
  expect(screen.getByRole('link', { name: 'Go to Contacts' })).toHaveAttribute('href', '/contacts');
  expect(screen.getByRole('link', { name: 'Go to Dashboard' })).toHaveAttribute('href', '/');
  expect(document.title).toBe('Page not found · Mycorrhizal CRM');
});
