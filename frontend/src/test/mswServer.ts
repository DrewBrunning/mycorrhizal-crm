// msw-based request-level test harness for src/api/* (issue #1482).
//
// Unlike the hand-mocked `vi.fn()` fetch stubs elsewhere, requests here run
// through the real `apiFetch` wrapper and the real `fetch` -> interceptor
// path, so a test can assert on the request that actually went out (method,
// URL incl. query string, headers, body) and on how real HTTP responses
// (4xx envelopes, 204s, non-JSON) are mapped to typed errors.

import { Blob as NodeBlob, File as NodeFile } from 'node:buffer';
import { HttpResponse, http, type JsonBodyType } from 'msw';
import { setupServer } from 'msw/node';
import { afterAll, afterEach, beforeAll, vi } from 'vitest';
import { API_BASE_URL } from '../api/client';

export const server = setupServer();

export interface Captured {
  method: string;
  /** Path + query as sent, relative to the origin. */
  url: string;
  pathname: string;
  search: URLSearchParams;
  headers: Headers;
  /** Parsed JSON body, or undefined when the request had none / was not JSON. */
  body: unknown;
  /** FormData body when the request was multipart. */
  form?: FormData;
}

type Method = 'get' | 'post' | 'put' | 'patch' | 'delete';

/** Call once at the top of a test file: starts/resets/stops the server. */
export function setupMswServer(): void {
  beforeAll(() => server.listen({ onUnhandledFrame: 'error' }));
  afterEach(() => server.resetHandlers());
  afterAll(() => server.close());
}

/**
 * jsdom ships its own FormData/File/Blob classes, which Node's undici `fetch`
 * (what msw intercepts) cannot serialize. Multipart tests call this once to swap
 * in Node's native trio, and get back a `File` factory.
 *
 * `File` and `Blob` matter as much as `FormData`: undici's multipart parser
 * validates every parsed entry with `webidl.is.File` against the *global*
 * `File`. Node 22's undici happened to accept a jsdom-shadowed global here;
 * Node 24 (which `engines.node`'s `>=22.22.2` resolves to in CI) asserts, so a
 * multipart `Request.formData()` throws unless the native brands are global.
 */
export function setupNativeMultipart(): (content: string, name: string, type?: string) => File {
  beforeAll(async () => {
    const NodeFormData = (await new Response(new URLSearchParams('a=1')).formData())
      .constructor as typeof FormData;
    vi.stubGlobal('FormData', NodeFormData);
    vi.stubGlobal('File', NodeFile);
    vi.stubGlobal('Blob', NodeBlob);
  });
  afterAll(() => {
    vi.unstubAllGlobals();
  });
  return (content, name, type = 'text/plain') =>
    new NodeFile([content], name, { type }) as unknown as File;
}

/**
 * Registers a one-route handler for `${API_BASE_URL}${path}` and returns the
 * list of requests it captured. `response` is a JSON body (default 200), or a
 * full `Response` for status/headers control.
 */
export function mockApi(
  method: Method,
  path: string,
  response: JsonBodyType | Response = {},
): Captured[] {
  const captured: Captured[] = [];
  server.use(
    http[method](`${API_BASE_URL}${path}`, async ({ request }) => {
      const u = new URL(request.url);
      let body: unknown;
      let form: FormData | undefined;
      const ct = request.headers.get('content-type') ?? '';
      if (ct.includes('multipart/form-data')) {
        form = await request.clone().formData();
      } else {
        const text = await request.clone().text();
        if (text) {
          try {
            body = JSON.parse(text);
          } catch {
            body = text;
          }
        }
      }
      captured.push({
        method: request.method,
        url: u.pathname + u.search,
        pathname: u.pathname,
        search: u.searchParams,
        headers: request.headers,
        body,
        form,
      });
      return response instanceof Response ? response.clone() : HttpResponse.json(response);
    }),
  );
  return captured;
}

/** A backend error envelope response (see parseErrorResponse / handleResponse). */
export function errorEnvelope(
  status: number,
  code: string,
  message: string,
  details?: Record<string, string>,
  requestId = 'req-test',
): Response {
  return HttpResponse.json(
    { error: { code, message, ...(details ? { details } : {}) }, request_id: requestId },
    { status },
  );
}
