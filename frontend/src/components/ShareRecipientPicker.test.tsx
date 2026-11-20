import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { HttpResponse, http } from 'msw';
import { useState } from 'react';
import { afterEach, describe, expect, test, vi } from 'vitest';
import '../i18n/config';
import { API_BASE_URL } from '../api/client';
import type { UserDirectoryEntry } from '../api/contactShares';
import { errorEnvelope, mockApi, server, setupMswServer } from '../test/mswServer';
import ShareRecipientPicker from './ShareRecipientPicker';

setupMswServer();
afterEach(cleanup);
afterEach(() => vi.useRealTimers());

const HINT = 'Type at least 3 letters of the username';
const everyone = {
  users: [
    { id: 2, username: 'bob' },
    { id: 3, username: 'carol' },
  ],
};

function Harness({
  onPick,
  onError,
}: {
  onPick?: (u: UserDirectoryEntry | null) => void;
  onError?: (m: string) => void;
}) {
  const [value, setValue] = useState<UserDirectoryEntry | null>(null);
  return (
    <ShareRecipientPicker
      label="Recipient"
      value={value}
      onChange={(u) => {
        setValue(u);
        onPick?.(u);
      }}
      onError={onError ?? (() => {})}
    />
  );
}

const box = () => screen.findByRole('combobox', { name: 'Recipient' });
const type = (el: HTMLElement, v: string) => {
  fireEvent.focus(el); // MUI resets the text of an unfocused Autocomplete on re-render
  fireEvent.change(el, { target: { value: v } });
};
const settle = (ms = 400) =>
  act(async () => {
    await new Promise((r) => setTimeout(r, ms));
  });

describe('mode all (default): current behaviour', () => {
  test('lists every user, sends no q on load, and shows no hint', async () => {
    const calls = mockApi('get', '/users/directory', everyone);
    render(<Harness />);
    const input = await box();
    await waitFor(() => expect(calls).toHaveLength(1));
    expect(calls[0].search.has('q')).toBe(false);
    fireEvent.mouseDown(input);
    expect(await screen.findByRole('option', { name: 'bob' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'carol' })).toBeInTheDocument();
    expect(screen.queryByText(HINT)).not.toBeInTheDocument();
  });

  test('typing short text keeps the full list (server ignores q) and still shows no hint', async () => {
    const calls = mockApi('get', '/users/directory', everyone);
    render(<Harness />);
    const input = await box();
    await waitFor(() => expect(calls).toHaveLength(1));
    type(input, 'bo');
    await waitFor(() => expect(calls.at(-1)?.search.get('q')).toBe('bo'));
    fireEvent.mouseDown(input);
    expect(await screen.findByRole('option', { name: 'bob' })).toBeInTheDocument();
    expect(screen.queryByText(HINT)).not.toBeInTheDocument();
  });
});

describe('mode search (inferred from an empty list for a short query)', () => {
  function searchServer() {
    const calls: string[] = [];
    server.use(
      http.get(`${API_BASE_URL}/users/directory`, ({ request }) => {
        const q = (new URL(request.url).searchParams.get('q') ?? '').trim().toLowerCase();
        calls.push(q);
        const users = q.length < 3 ? [] : everyone.users.filter((u) => u.username.startsWith(q));
        return HttpResponse.json({ users });
      }),
    );
    return calls;
  }

  test('shows the hint, queries only from 3 characters, and picks a result', async () => {
    const calls = searchServer();
    const onPick = vi.fn();
    render(<Harness onPick={onPick} />);
    const input = await box();
    expect(await screen.findByText(HINT)).toBeInTheDocument();
    // Advance the 250 ms debounce on fake timers, rather than sleeping on the
    // real clock, so "no request for a 2-letter query" is a deterministic
    // assertion instead of a race with an under-scheduled timer.
    vi.useFakeTimers();
    type(input, 'bo');
    await act(async () => {
      await vi.advanceTimersByTimeAsync(300);
    });
    vi.useRealTimers();
    expect(calls).toEqual(['']); // no request for a 2-letter query once search mode is known
    type(input, 'bob');
    await waitFor(() => expect(calls).toEqual(['', 'bob']));
    fireEvent.mouseDown(input);
    fireEvent.click(await screen.findByRole('option', { name: 'bob' }));
    expect(onPick).toHaveBeenCalledWith({ id: 2, username: 'bob' });
    expect(screen.queryByRole('option', { name: 'carol' })).not.toBeInTheDocument();
  });

  test('debounces: several quick keystrokes send one request', async () => {
    const calls = searchServer();
    render(<Harness />);
    const input = await box();
    await screen.findByText(HINT);
    type(input, 'car');
    type(input, 'caro');
    type(input, 'carol');
    await waitFor(() => expect(calls).toEqual(['', 'carol']));
  });

  test('a slow response for an older query never overwrites a newer one', async () => {
    let release: () => void = () => {};
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const seen: string[] = [];
    server.use(
      http.get(`${API_BASE_URL}/users/directory`, async ({ request }) => {
        const q = new URL(request.url).searchParams.get('q') ?? '';
        seen.push(q);
        if (q === 'caro') {
          await gate; // held in flight until the test releases it
          return HttpResponse.json({ users: [{ id: 9, username: 'carol-stale' }] });
        }
        if (q === 'carol') return HttpResponse.json({ users: [{ id: 3, username: 'carol' }] });
        return HttpResponse.json({ users: [] });
      }),
    );
    render(<Harness />);
    const input = await box();
    await screen.findByText(HINT);
    type(input, 'caro');
    await waitFor(() => expect(seen).toContain('caro'));
    type(input, 'carol');
    await waitFor(() => expect(seen).toContain('carol'));
    expect(await screen.findByRole('option', { name: 'carol' })).toBeInTheDocument();
    release(); // the older 'caro' response now arrives after the newer one
    await settle(100);
    expect(screen.getByRole('option', { name: 'carol' })).toBeInTheDocument();
    expect(screen.queryByRole('option', { name: 'carol-stale' })).not.toBeInTheDocument();
  });

  test('a long query with no match says there are no matching users, not the hint', async () => {
    searchServer();
    render(<Harness />);
    const input = await box();
    await screen.findByText(HINT);
    type(input, 'zzz');
    fireEvent.mouseDown(input);
    expect(await screen.findByText('No matching users.')).toBeInTheDocument();
  });
});

test('a failed directory load is reported through onError', async () => {
  mockApi('get', '/users/directory', errorEnvelope(500, 'internal_error', 'directory down'));
  const onError = vi.fn();
  render(<Harness onError={onError} />);
  await waitFor(() =>
    expect(onError).toHaveBeenCalledWith(expect.stringContaining('directory down')),
  );
});
