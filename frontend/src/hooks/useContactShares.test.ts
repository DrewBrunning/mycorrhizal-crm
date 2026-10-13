import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import {
  acceptContactShare,
  confirmContactShare,
  declineContactShare,
  getIncomingContactShares,
  getOutgoingContactShares,
} from '../api/contactShares';
import { useContactShares } from './useContactShares';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock('../api/contactShares', () => ({
  acceptContactShare: vi.fn(),
  confirmContactShare: vi.fn(),
  declineContactShare: vi.fn(),
  getIncomingContactShares: vi.fn(),
  getOutgoingContactShares: vi.fn(),
}));

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  for (const fn of [
    acceptContactShare,
    confirmContactShare,
    declineContactShare,
    getIncomingContactShares,
    getOutgoingContactShares,
  ]) {
    vi.mocked(fn).mockReset();
  }
  vi.mocked(getIncomingContactShares).mockResolvedValue({
    contact_shares: [],
    usernames: {},
  } as never);
  vi.mocked(getOutgoingContactShares).mockResolvedValue({
    contact_shares: [],
    usernames: {},
  } as never);
});

const share = (id: string) => ({ id }) as never;

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

test('starts empty and idle', () => {
  const { result } = renderHook(() => useContactShares());
  expect(result.current.incoming).toEqual([]);
  expect(result.current.outgoing).toEqual([]);
  expect(result.current.usernames).toEqual({});
  expect(result.current.loading).toBe(false);
  expect(result.current.error).toBeNull();
});

test('refresh loads both directions and merges usernames, incoming winning on a clash', async () => {
  vi.mocked(getIncomingContactShares).mockResolvedValue({
    contact_shares: [share('in')],
    usernames: { u1: 'incoming-name', u2: 'bob' },
  } as never);
  vi.mocked(getOutgoingContactShares).mockResolvedValue({
    contact_shares: [share('out')],
    usernames: { u1: 'outgoing-name', u3: 'cy' },
  } as never);
  const { result } = renderHook(() => useContactShares());
  await act(async () => {
    await result.current.refresh();
  });
  expect(result.current.incoming).toEqual([{ id: 'in' }]);
  expect(result.current.outgoing).toEqual([{ id: 'out' }]);
  expect(result.current.usernames).toEqual({ u1: 'incoming-name', u2: 'bob', u3: 'cy' });
});

test('refresh tolerates responses without contact_shares or usernames', async () => {
  vi.mocked(getIncomingContactShares).mockResolvedValue({} as never);
  vi.mocked(getOutgoingContactShares).mockResolvedValue({} as never);
  const { result } = renderHook(() => useContactShares());
  await act(async () => {
    await result.current.refresh();
  });
  expect(result.current.incoming).toEqual([]);
  expect(result.current.outgoing).toEqual([]);
  expect(result.current.usernames).toEqual({});
});

test('refresh reports loading, surfaces a failure with its operation, and a retry clears it', async () => {
  const d = deferred<never>();
  vi.mocked(getIncomingContactShares).mockReturnValueOnce(d.promise);
  const { result } = renderHook(() => useContactShares());
  act(() => {
    void result.current.refresh();
  });
  await waitFor(() => expect(result.current.loading).toBe(true));
  await act(async () => {
    d.resolve({ contact_shares: [] } as never);
  });
  await waitFor(() => expect(result.current.loading).toBe(false));

  vi.mocked(getOutgoingContactShares).mockRejectedValueOnce(new Error('down'));
  await act(async () => {
    await result.current.refresh();
  });
  expect(result.current.error).toBe('down');
  expect(result.current.loading).toBe(false);
  expect(console.error).toHaveBeenCalledWith('[fetching contact shares] Error:', expect.anything());

  const d2 = deferred<never>();
  vi.mocked(getIncomingContactShares).mockReturnValueOnce(d2.promise);
  act(() => {
    void result.current.refresh();
  });
  await waitFor(() => expect(result.current.loading).toBe(true));
  expect(result.current.error).toBeNull();
  await act(async () => {
    d2.resolve({ contact_shares: [] } as never);
  });
});

test('accept-preview returns the server preview without refreshing', async () => {
  vi.mocked(acceptContactShare).mockResolvedValue({ session_id: 's' } as never);
  const { result } = renderHook(() => useContactShares());
  let preview: unknown;
  await act(async () => {
    preview = await result.current.handleAcceptPreview('sh-1');
  });
  expect(acceptContactShare).toHaveBeenCalledWith('sh-1');
  expect(preview).toEqual({ session_id: 's' });
  expect(getIncomingContactShares).not.toHaveBeenCalled();
});

test('confirm sends the share, session and actions, refreshes, and returns the result', async () => {
  vi.mocked(confirmContactShare).mockResolvedValue({ created: 1 } as never);
  const { result } = renderHook(() => useContactShares());
  const actions = [{ row_index: 0, action: 'add' }] as never;
  let out: unknown;
  await act(async () => {
    out = await result.current.handleConfirmShare('sh-1', 'sess-1', actions);
  });
  expect(confirmContactShare).toHaveBeenCalledWith('sh-1', 'sess-1', actions);
  expect(out).toEqual({ created: 1 });
  expect(getIncomingContactShares).toHaveBeenCalledTimes(1);
});

test('decline calls the API and refreshes', async () => {
  vi.mocked(declineContactShare).mockResolvedValue(undefined as never);
  const { result } = renderHook(() => useContactShares());
  await act(async () => {
    await result.current.handleDeclineShare('sh-2');
  });
  expect(declineContactShare).toHaveBeenCalledWith('sh-2');
  expect(getIncomingContactShares).toHaveBeenCalledTimes(1);
});

test('each failing action notifies, logs its operation, rethrows, and does not refresh', async () => {
  vi.mocked(acceptContactShare).mockRejectedValue(new Error('p'));
  vi.mocked(confirmContactShare).mockRejectedValue(new Error('c'));
  vi.mocked(declineContactShare).mockRejectedValue(new Error('d'));
  const showError = vi.fn();
  const { result } = renderHook(() => useContactShares({ showError }));
  await act(async () => {
    await expect(result.current.handleAcceptPreview('x')).rejects.toThrow('p');
    await expect(result.current.handleConfirmShare('x', 's', [])).rejects.toThrow('c');
    await expect(result.current.handleDeclineShare('x')).rejects.toThrow('d');
  });
  expect(showError.mock.calls.map((c) => c[0])).toEqual(['p', 'c', 'd']);
  for (const op of [
    'previewing contact share',
    'confirming contact share',
    'declining contact share',
  ]) {
    expect(console.error).toHaveBeenCalledWith(`[${op}] Error:`, expect.anything());
  }
  expect(getIncomingContactShares).not.toHaveBeenCalled();
});
