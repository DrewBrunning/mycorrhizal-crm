import { cleanup, render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { afterEach, expect, test, vi } from 'vitest';
import { page, userEvent } from 'vitest/browser';
import './i18n/config';
import type { ContactRecordResponse } from './api/contacts';
import ContactDetailPage from './ContactDetailPage';
import { SnackbarProvider } from './context/SnackbarContext';
import { DateFormatProvider } from './DateFormatProvider';

// Real-browser tier (issue #2539). ContactDetailPage.test.tsx (jsdom) replaces
// Element.prototype.scrollIntoView with a spy, so it only proves the page asked
// for a scroll, not that the browser actually moved the section under the
// header. Here a real Chromium click on the jump nav performs a real scroll and
// the assertion reads getBoundingClientRect().top.
//
// frontend/AGENTS.md trap 1: explicit cleanup, no globals.
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const record: ContactRecordResponse = {
  id: 1,
  uid: 'alice-uid',
  etag: 'e-1-7',
  revision: 7,
  gender: '',
  photo: '',
  archived: false,
  is_favorite: false,
  card: {
    name: {
      components: [
        { kind: 'given', value: 'Alice' },
        { kind: 'surname', value: 'Wonder' },
      ],
    },
    emails: [{ address: 'alice.wonder@example.com', contexts: ['home'] }],
  },
  crm: { kind: 'human' },
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  });
}

const emptyPage = { total: 0, next_cursor: '', limit: 100 };

// Every GET the page makes on mount, answered with the shape the real backend
// returns for an empty account (the jsdom suite's mockFetch is the fuller
// version). Routing all of them means the page reaches its final layout before
// the click, instead of settling late and moving the section under test.
const GET_ROUTES: Record<string, () => Response> = {
  '/contacts/1': () => json(record),
  '/contacts/1/notes': () => json({ notes: [] }),
  '/contacts/1/activities': () => json({ activities: [] }),
  '/contacts/1/reminder-completions': () => json({ completions: [] }),
  '/contacts/1/reminders': () => json({ reminders: [] }),
  '/contacts/1/field-values': () => json({ field_values: [] }),
  '/contacts/1/entity-links': () => json({ links: [] }),
  '/contacts/1/attachments': () => json({ attachments: [], total: 0 }),
  '/contacts/1/photos': () => json({ photos: [], total: 0 }),
  '/contacts/1/life-event-suggestions': () => json({ suggestions: [] }),
  '/contacts/1/field-visibility': () =>
    json({
      overrides: {},
      hide_empty: 'inherit',
      defaults: { fields: {}, hide_empty: false },
      resolved: { fields: {}, hide_empty: false },
    }),
  '/users/me': () =>
    json({
      id: 1,
      email: 'me@example.com',
      username: 'me',
      enabled_contact_fields: null,
      self_contact_vcard_uid: null,
    }),
  '/contacts': () => json({ contacts: [], ...emptyPage }),
  '/circles': () => json({ circles: [], members: [], ...emptyPage, limit: 200 }),
  '/tags': () => json({ tags: [], contacts: [], ...emptyPage, limit: 200 }),
  '/preferences': () => json({ preferences: [], ...emptyPage, limit: 200 }),
  '/field-definitions': () => json({ field_definitions: [], ...emptyPage }),
  '/relationship-edges': () => json({ relationship_edges: [], ...emptyPage }),
  '/life-events': () => json({ life_events: [], ...emptyPage, limit: 50 }),
  '/conversation-agenda': () => json({ conversation_agenda: [], ...emptyPage }),
  '/gifts': () => json({ gifts: [], ...emptyPage }),
  '/occasion-obligations': () => json({ occasion_obligations: [], ...emptyPage }),
  '/cadence-policies': () => json({ cadence_policies: [], ...emptyPage }),
  '/data-decay-policies': () => json({ data_decay_policies: [], ...emptyPage }),
  '/external-identities': () => json({ external_identities: [], ...emptyPage }),
  '/external-activities': () => json({ external_activities: [], ...emptyPage }),
  '/contacts/1/timeline': () => json({ items: [], ...emptyPage }),
  '/graph/connections': () =>
    json({ from_vcard_uid: 'alice-uid', from_name: 'Alice', depth: 1, chains: [] }),
  '/immich/contacts/alice-uid/summary': () => json({ summary: null }),
  '/contacts/1/score': () =>
    json({
      contact_id: 1,
      score: 50,
      band: 'steady',
      recency: { value: 1, weight: 1, reason: '' },
      frequency: { value: 1, weight: 1, reason: '' },
      closeness: { value: 1, weight: 1, reason: '' },
      reach_out: { value: 1, weight: 1, reason: '' },
      last_updated: { value: 1, weight: 1, reason: '' },
    }),
};

function stubFetch() {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL) => {
      const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
      const path = new URL(url, window.location.origin).pathname.replace(/^.*\/api\/v1/, '');
      const route = GET_ROUTES[path];
      if (route) return route();
      return json({ error: { code: 'NOT_FOUND', message: `no route for ${path}` } }, 404);
    }),
  );
}

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/contacts/1']}>
      <SnackbarProvider>
        <DateFormatProvider>
          <Routes>
            <Route path="/contacts/:id" element={<ContactDetailPage />} />
          </Routes>
        </DateFormatProvider>
      </SnackbarProvider>
    </MemoryRouter>,
  );
}

// Reading the landing position only after the scroll offset has been still for
// a beat: an anchor jump is instant, but the app's own narrow-layout jump nav
// asks for behavior: 'smooth', and a future change to either must not race the
// assertion.
async function settledScrollTop(section: HTMLElement, timeoutMs = 8000): Promise<number> {
  const start = performance.now();
  let lastY = window.scrollY;
  let stableSince = performance.now();
  while (performance.now() - start < timeoutMs) {
    await new Promise((resolve) => requestAnimationFrame(resolve));
    if (Math.abs(window.scrollY - lastY) < 0.5) {
      if (performance.now() - stableSince > 400) break;
    } else {
      lastY = window.scrollY;
      stableSince = performance.now();
    }
  }
  return section.getBoundingClientRect().top;
}

// The page fills its sections in as each hook's fetch resolves -- some
// sections render their content a few hundred milliseconds after the header.
// An anchor jump taken while that is still happening lands where the section
// *was*, so wait for both the document height and this section's offset to
// hold steady for half a second before clicking.
async function layoutSettled(section: HTMLElement, timeoutMs = 8000): Promise<void> {
  const start = performance.now();
  let last = '';
  let stableSince = performance.now();
  while (performance.now() - start < timeoutMs) {
    await new Promise((resolve) => requestAnimationFrame(resolve));
    const snapshot = `${document.documentElement.scrollHeight}:${section.offsetTop}`;
    if (snapshot === last) {
      if (performance.now() - stableSince > 500) return;
    } else {
      last = snapshot;
      stableSince = performance.now();
    }
  }
}

test('clicking a jump-nav section really scrolls it under the header', async () => {
  await page.viewport(1280, 800);
  stubFetch();
  renderPage();

  await screen.findByRole('heading', { name: 'Alice Wonder' });
  const section = document.getElementById('timeline') as HTMLElement;
  await layoutSettled(section);

  const before = section.getBoundingClientRect().top;
  // The timeline is far below the fold to begin with -- otherwise the
  // assertion below could pass without any scrolling at all.
  expect(before).toBeGreaterThan(800);

  await userEvent.click(screen.getByRole('link', { name: 'Timeline' }));

  const top = await settledScrollTop(section);

  // The browser really moved: the page left the top and the section came up
  // from below the fold.
  expect(window.scrollY).toBeGreaterThan(0);
  expect(top).toBeLessThan(before);

  // Where exactly it lands is not arbitrary -- it is the app's declared
  // clearance. SectionGroup reserves scroll-margin-top (AppBar + sticky jump
  // nav) and ContactDetailPage reserves the same amount of scroll-padding-top
  // on the root for descendant scrolling, so the section lands at
  // top === margin + padding (224px here). The issue's "0-100px from the top"
  // predates that clearance: what matters is that the browser put the section
  // exactly where the page asked, under the header rather than at the top of
  // the viewport or still below the fold. Reading the values back from the
  // live stylesheet keeps this a scroll assertion rather than a snapshot of
  // the current constants.
  const clearance =
    parseFloat(getComputedStyle(section).scrollMarginTop || '0') +
    parseFloat(getComputedStyle(document.documentElement).scrollPaddingTop || '0');
  expect(clearance).toBeGreaterThan(0);
  expect(top).toBeGreaterThanOrEqual(0);
  expect(Math.abs(top - clearance)).toBeLessThanOrEqual(2);
  expect(top).toBeLessThan(document.documentElement.clientHeight / 2);
});
