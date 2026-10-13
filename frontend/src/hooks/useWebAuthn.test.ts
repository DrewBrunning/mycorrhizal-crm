import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import {
  listPasskeys,
  type Passkey,
  type PasskeyRemovalProof,
  registerPasskey,
  removePasskey,
} from '../api/webauthn';
import { useWebAuthn } from './useWebAuthn';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(cleanup);

vi.mock('../api/webauthn', () => ({
  listPasskeys: vi.fn(),
  registerPasskey: vi.fn(),
  removePasskey: vi.fn(),
}));

beforeEach(() => {
  vi.mocked(listPasskeys).mockReset();
  vi.mocked(registerPasskey).mockReset();
  vi.mocked(removePasskey).mockReset();
});

const key = (id: string) => ({ id, name: id }) as unknown as Passkey;

test('loads the passkeys on mount and then stops loading', async () => {
  vi.mocked(listPasskeys).mockResolvedValue([key('a')]);
  const { result } = renderHook(() => useWebAuthn());
  expect(result.current.passkeys).toEqual([]);
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.passkeys).toEqual([key('a')]);
  expect(listPasskeys).toHaveBeenCalledTimes(1);
});

test('a failed load keeps the existing list and still stops loading', async () => {
  vi.mocked(listPasskeys).mockResolvedValueOnce([key('a')]);
  const { result } = renderHook(() => useWebAuthn());
  await waitFor(() => expect(result.current.passkeys).toEqual([key('a')]));

  vi.mocked(listPasskeys).mockRejectedValueOnce(new Error('offline'));
  await act(async () => {
    await result.current.refresh();
  });
  expect(result.current.passkeys).toEqual([key('a')]);
  expect(result.current.loading).toBe(false);
});

test('a failed first load leaves an empty list and stops loading', async () => {
  vi.mocked(listPasskeys).mockRejectedValue(new Error('offline'));
  const { result } = renderHook(() => useWebAuthn());
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.passkeys).toEqual([]);
});

test('register forwards the name and proof, reloads the list, and returns the enrollment', async () => {
  vi.mocked(listPasskeys)
    .mockResolvedValueOnce([])
    .mockResolvedValueOnce([key('new')]);
  vi.mocked(registerPasskey).mockResolvedValue({ id: 'new' } as never);
  const { result } = renderHook(() => useWebAuthn());
  await waitFor(() => expect(result.current.loading).toBe(false));
  let out: unknown;
  await act(async () => {
    out = await result.current.register('Laptop', { password: 'x' } as never);
  });
  expect(registerPasskey).toHaveBeenCalledWith('Laptop', { password: 'x' });
  expect(out).toEqual({ id: 'new' });
  expect(result.current.passkeys).toEqual([key('new')]);
});

test('register propagates a failure without reloading', async () => {
  vi.mocked(listPasskeys).mockResolvedValue([]);
  vi.mocked(registerPasskey).mockRejectedValue(new Error('cancelled'));
  const { result } = renderHook(() => useWebAuthn());
  await waitFor(() => expect(result.current.loading).toBe(false));
  vi.mocked(listPasskeys).mockClear();
  await act(async () => {
    await expect(result.current.register()).rejects.toThrow('cancelled');
  });
  expect(listPasskeys).not.toHaveBeenCalled();
});

test('remove forwards the id and proof, then reloads; failures propagate', async () => {
  vi.mocked(listPasskeys)
    .mockResolvedValueOnce([key('a')])
    .mockResolvedValueOnce([]);
  vi.mocked(removePasskey).mockResolvedValueOnce(undefined);
  const { result } = renderHook(() => useWebAuthn());
  await waitFor(() => expect(result.current.passkeys).toHaveLength(1));
  const proof = { password: 'x' } as unknown as PasskeyRemovalProof;
  await act(async () => {
    await result.current.remove('a', proof);
  });
  expect(removePasskey).toHaveBeenCalledWith('a', proof);
  expect(result.current.passkeys).toEqual([]);

  vi.mocked(removePasskey).mockRejectedValueOnce(new Error('denied'));
  await act(async () => {
    await expect(result.current.remove('a', proof)).rejects.toThrow('denied');
  });
});

test('register and remove use the latest refresh after the list was reloaded', async () => {
  vi.mocked(listPasskeys).mockResolvedValue([]);
  vi.mocked(registerPasskey).mockResolvedValue({ id: 'n' } as never);
  vi.mocked(removePasskey).mockResolvedValue(undefined);
  const { result, rerender } = renderHook(() => useWebAuthn());
  await waitFor(() => expect(result.current.loading).toBe(false));
  rerender();
  vi.mocked(listPasskeys).mockClear();
  await act(async () => {
    await result.current.register('x');
    await result.current.remove('a', {} as PasskeyRemovalProof);
  });
  expect(listPasskeys).toHaveBeenCalledTimes(2);
});
