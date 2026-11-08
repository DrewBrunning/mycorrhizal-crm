// Issue #2364: dev-server proxy so the split-port dev setup (Vite on :7300,
// API on :8080) is same-origin from the browser's point of view. Image
// responses carry `Cross-Origin-Resource-Policy: same-origin` (ADR 0040) and
// that header is deliberately NOT relaxed anywhere; proxying is what lets the
// browser load them. Production gets the same shape from nginx.
//
// Only the dev server (`vite serve`) reads this; builds ignore `server.proxy`.
export const DEFAULT_DEV_PROXY_TARGET = 'http://localhost:8080';

export const DEV_PROXY_PATHS = ['/api', '/health', '/.well-known'] as const;

export function devProxy(target?: string) {
  const origin = (target || DEFAULT_DEV_PROXY_TARGET).replace(/\/+$/, '');
  return Object.fromEntries(
    DEV_PROXY_PATHS.map((p) => [p, { target: origin, changeOrigin: false }]),
  );
}
