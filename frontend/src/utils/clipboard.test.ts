import { test, expect, vi, afterEach } from 'vitest';
import { copyToClipboard } from './clipboard';

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

test('uses navigator.clipboard.writeText in a secure context', async () => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  vi.stubGlobal('navigator', { clipboard: { writeText } });
  vi.stubGlobal('window', { ...window, isSecureContext: true });

  const ok = await copyToClipboard('hello');

  expect(writeText).toHaveBeenCalledWith('hello');
  expect(ok).toBe(true);
});

test('falls back to document.execCommand when clipboard.writeText throws', async () => {
  const writeText = vi.fn().mockRejectedValue(new Error('denied'));
  vi.stubGlobal('navigator', { clipboard: { writeText } });
  vi.stubGlobal('window', { ...window, isSecureContext: true });
  const execCommand = vi.fn().mockReturnValue(true);
  document.execCommand = execCommand;

  const ok = await copyToClipboard('hello');

  expect(execCommand).toHaveBeenCalledWith('copy');
  expect(ok).toBe(true);
});

test('falls back to document.execCommand in a non-secure context', async () => {
  vi.stubGlobal('navigator', { clipboard: undefined });
  const execCommand = vi.fn().mockReturnValue(true);
  document.execCommand = execCommand;

  const ok = await copyToClipboard('hello');

  expect(execCommand).toHaveBeenCalledWith('copy');
  expect(ok).toBe(true);
});

test('returns false when every copy mechanism fails', async () => {
  vi.stubGlobal('navigator', { clipboard: undefined });
  document.execCommand = vi.fn().mockReturnValue(false);

  const ok = await copyToClipboard('hello');

  expect(ok).toBe(false);
});
