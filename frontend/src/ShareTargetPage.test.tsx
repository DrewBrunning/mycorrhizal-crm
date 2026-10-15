import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import './i18n/config';
import { getContacts } from './api/contacts';
import { createUnassignedNote } from './api/notes';
import ShareTargetPage from './ShareTargetPage';
import { makeFakeCaches } from './test/fakeCaches';
import { storeSharedDraft } from './utils/shareTarget';

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.clearAllMocks();
  sessionStorage.clear();
});

vi.mock('./api/contacts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/contacts')>();
  return { ...actual, getContacts: vi.fn() };
});
vi.mock('./api/notes', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/notes')>();
  return { ...actual, createUnassignedNote: vi.fn() };
});

beforeEach(() => {
  vi.stubGlobal('caches', makeFakeCaches());
  vi.mocked(getContacts).mockResolvedValue({
    contacts: [],
    total: 0,
    page: 1,
    limit: 40,
  } as unknown as Awaited<ReturnType<typeof getContacts>>);
  vi.mocked(createUnassignedNote).mockResolvedValue(
    {} as Awaited<ReturnType<typeof createUnassignedNote>>,
  );
});

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/share" element={<ShareTargetPage />} />
        <Route path="/notes" element={<div>notes page</div>} />
        <Route path="/" element={<div>dashboard</div>} />
      </Routes>
    </MemoryRouter>,
  );
}

test('prefills the note composer from the stored share and consumes it', async () => {
  const key = await storeSharedDraft(caches, window.location.origin, 'shared words');
  renderAt(`/share?key=${key}`);
  const field = await screen.findByLabelText('Content *');
  expect((field as HTMLTextAreaElement).value).toBe('shared words');
  // Single use: the entry is gone after the page took it.
  const again = await caches.open('share-target-drafts');
  expect(await again.keys()).toHaveLength(0);
  expect(createUnassignedNote).not.toHaveBeenCalled();
});

test('saving creates the note only on the explicit tap, then goes to notes', async () => {
  const key = await storeSharedDraft(caches, window.location.origin, 'keep this');
  renderAt(`/share?key=${key}`);
  await screen.findByLabelText('Content *');
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  await waitFor(() => expect(createUnassignedNote).toHaveBeenCalledTimes(1));
  expect(vi.mocked(createUnassignedNote).mock.calls[0][0].content).toBe('keep this');
  expect(await screen.findByText('notes page')).toBeTruthy();
});

test('backing out discards the draft and writes nothing', async () => {
  const key = await storeSharedDraft(caches, window.location.origin, 'drop me');
  renderAt(`/share?key=${key}`);
  await screen.findByLabelText('Content *');
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
  fireEvent.click(await screen.findByRole('button', { name: /discard/i }));
  expect(await screen.findByText('dashboard')).toBeTruthy();
  expect(createUnassignedNote).not.toHaveBeenCalled();
});

test('a missing, spent or forged key shows the expired notice', async () => {
  renderAt('/share?key=not-a-key');
  expect(await screen.findByText(/no longer available/i)).toBeTruthy();
  fireEvent.click(screen.getByRole('button', { name: 'Back to dashboard' }));
  expect(await screen.findByText('dashboard')).toBeTruthy();
});
