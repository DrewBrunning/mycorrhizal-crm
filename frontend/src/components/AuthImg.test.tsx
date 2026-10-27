import { act, cleanup, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import AuthImg from './AuthImg';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// AuthImg used to be covered only incidentally, by other components' tests
// whose fetch happened to settle before teardown, so its line coverage moved
// between CI runs and tripped the per-file ratchet. These pin every path.

// A minimal Response: jsdom's Blob can't be handed to Node's Response.
function okResponse(): Response {
  return {
    ok: true,
    status: 200,
    blob: async () => new Blob(['png-bytes'], { type: 'image/png' }),
  } as Response;
}

test('fetches with credentials and renders the image as a data URL', async () => {
  const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(okResponse());

  render(<AuthImg src="/api/v1/photos/1" alt="Alice" />);

  expect(screen.getByRole('progressbar')).toBeTruthy();
  const img = await screen.findByRole('img', { name: 'Alice' });
  expect(img.getAttribute('src')).toMatch(/^data:image\/png;base64,/);
  expect(fetchSpy).toHaveBeenCalledWith('/api/v1/photos/1', { credentials: 'include' });
});

test('renders nothing for a non-OK response', async () => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('nope', { status: 404 }));

  const { container } = render(<AuthImg src="/api/v1/photos/missing" alt="Missing" />);

  await waitFor(() => expect(screen.queryByRole('progressbar')).toBeNull());
  expect(screen.queryByRole('img')).toBeNull();
  expect(container.innerHTML).toBe('');
});

test('renders nothing when the request fails', async () => {
  vi.spyOn(globalThis, 'fetch').mockRejectedValue(new TypeError('network down'));

  const { container } = render(<AuthImg src="/api/v1/photos/2" alt="Bob" />);

  await waitFor(() => expect(screen.queryByRole('progressbar')).toBeNull());
  expect(container.innerHTML).toBe('');
});

test('a response that lands after unmount is dropped before decoding', async () => {
  let resolveFetch: (r: Response) => void = () => {};
  vi.spyOn(globalThis, 'fetch').mockReturnValue(
    new Promise<Response>((resolve) => {
      resolveFetch = resolve;
    }),
  );

  const readAsDataURL = vi.spyOn(FileReader.prototype, 'readAsDataURL');

  const { unmount } = render(<AuthImg src="/api/v1/photos/3" alt="Carol" />);
  unmount();

  await act(async () => {
    resolveFetch(okResponse());
    // Let response.blob() and the cancelled check run.
    await new Promise((r) => setTimeout(r, 0));
  });
  // The cancelled guard stops the work before the blob is decoded.
  expect(readAsDataURL).not.toHaveBeenCalled();
  expect(screen.queryByRole('img')).toBeNull();
});

test('a decoded image that lands after unmount is dropped before the state update', async () => {
  let resolveBlob: ((blob: Blob) => void) | undefined;
  let markBlobRequested: (() => void) | undefined;
  const blobRequested = new Promise<void>((resolve) => {
    markBlobRequested = resolve;
  });
  vi.spyOn(globalThis, 'fetch').mockResolvedValue({
    ok: true,
    status: 200,
    blob: () =>
      new Promise<Blob>((resolve) => {
        resolveBlob = resolve;
        markBlobRequested?.();
      }),
  } as Response);

  let finishDecode: (() => void) | undefined;
  const readAsDataURL = vi
    .spyOn(FileReader.prototype, 'readAsDataURL')
    .mockImplementation(function (this: FileReader) {
      finishDecode = () => {
        Object.defineProperty(this, 'result', {
          configurable: true,
          value: 'data:image/png;base64,b2s=',
        });
        this.onloadend?.(new ProgressEvent('loadend') as ProgressEvent<FileReader>);
      };
    });

  const { unmount } = render(<AuthImg src="/api/v1/photos/4" alt="Dana" />);

  await act(async () => {
    await blobRequested;
    resolveBlob?.(new Blob(['png-bytes'], { type: 'image/png' }));
    await new Promise((r) => setTimeout(r, 0));
  });
  expect(readAsDataURL).toHaveBeenCalled();

  unmount();
  await act(async () => {
    finishDecode?.();
    await Promise.resolve();
  });
  expect(screen.queryByRole('img')).toBeNull();
});

test('a failure that lands after unmount does not set the error state', async () => {
  let rejectFetch: (reason?: unknown) => void = () => {};
  vi.spyOn(globalThis, 'fetch').mockReturnValue(
    new Promise<Response>((_, reject) => {
      rejectFetch = reject;
    }),
  );

  const { unmount } = render(<AuthImg src="/api/v1/photos/5" alt="Eve" />);
  unmount();

  await act(async () => {
    rejectFetch(new TypeError('network down'));
    await Promise.resolve();
  });
  expect(screen.queryByRole('img')).toBeNull();
});
