import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { useEffect, useState } from 'react';
import { Link, MemoryRouter, Route, Routes } from 'react-router';
import { afterEach, expect, test } from 'vitest';
import '../i18n/config';
import { useDocumentTitle } from '../hooks/useDocumentTitle';
import RouteAnnouncer from './RouteAnnouncer';

afterEach(() => {
  cleanup();
  document.title = '';
});

function TitlePage({ title }: { title: string }) {
  useDocumentTitle(title);
  return <h1>{title}</h1>;
}

// A detail page whose real name arrives with its data, like ContactDetailPage.
function AsyncTitlePage() {
  const [title, setTitle] = useState('Contacts');
  useDocumentTitle(title);
  useEffect(() => {
    const timer = window.setTimeout(() => setTitle('Alice Johnson'), 30);
    return () => window.clearTimeout(timer);
  }, []);
  // ContactDetailPage renders only a skeleton until the record arrives.
  return title === 'Contacts' ? <p>Loading…</p> : <h1>{title}</h1>;
}

function Harness({ initialPath = '/' }: { initialPath?: string }) {
  return (
    <MemoryRouter initialEntries={[initialPath]}>
      <nav>
        <Link to="/contacts">Contacts</Link>
        <Link to="/contacts/1">Contact detail</Link>
      </nav>
      <main id="main-content" tabIndex={-1}>
        <Routes>
          <Route path="/" element={<TitlePage title="Dashboard" />} />
          <Route path="/contacts" element={<TitlePage title="Contacts" />} />
          <Route path="/contacts/1" element={<AsyncTitlePage />} />
        </Routes>
        <RouteAnnouncer />
      </main>
    </MemoryRouter>
  );
}

test('moves focus to the new page heading on a client route change', async () => {
  render(<Harness />);

  // Initial load: the browser announces the document itself; do not steal focus.
  expect(document.activeElement).not.toBe(document.getElementById('main-content'));

  fireEvent.click(screen.getByRole('link', { name: 'Contacts' }));

  await waitFor(() =>
    expect(document.activeElement).toBe(
      screen.getByRole('heading', { level: 1, name: 'Contacts' }),
    ),
  );
});

test('focuses a heading that settles after the route has already rendered', async () => {
  render(<Harness />);

  fireEvent.click(screen.getByRole('link', { name: 'Contact detail' }));

  // While the record is loading the announcer parks focus on main...
  await waitFor(() => expect(document.activeElement).toBe(document.getElementById('main-content')));
  // ...then moves it to the name once that arrives.
  await waitFor(() =>
    expect(document.activeElement).toBe(
      screen.getByRole('heading', { level: 1, name: 'Alice Johnson' }),
    ),
  );
});

test('does not move focus again once the user has tabbed into the page', async () => {
  render(<Harness />);

  fireEvent.click(screen.getByRole('link', { name: 'Contact detail' }));
  await waitFor(() => expect(document.activeElement).toBe(document.getElementById('main-content')));

  // The user tabs on before the async heading arrives.
  const link = screen.getByRole('link', { name: 'Contacts' });
  link.focus();

  await waitFor(() =>
    expect(screen.getByRole('heading', { level: 1, name: 'Alice Johnson' })).toBeInTheDocument(),
  );
  expect(document.activeElement).toBe(link);
});

test('does not steal focus on the initial mount', () => {
  render(<Harness initialPath="/contacts" />);

  expect(document.activeElement).not.toBe(document.getElementById('main-content'));
  expect(document.activeElement).not.toBe(
    screen.getByRole('heading', { level: 1, name: 'Contacts' }),
  );
});
