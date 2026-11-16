// The app-shell navigation fallback used by src/service-worker.ts. Kept as a
// pure function so the routing predicate can be unit-tested without a browser
// or Workbox; the service worker only supplies request.mode and url.pathname.

const fileExtensionRegexp = /\/[^/?]+\.[^/]+$/;

// Paths served by the backend, never by the SPA. When the API is same-origin
// (the standard all-in-one/nginx deployment, VITE_API_URL unset), a top-level
// navigation to one of these is still a backend request: OIDC's login and
// callback are navigations (issue #2636), and the health/CardDAV/CalDAV/
// MCP/well-known endpoints can be navigated to directly. Intercepting them
// and answering with index.html reloads the SPA and the request never
// reaches the server.
const backendPathPrefixes = [
  '/api',
  '/health',
  '/mcp',
  '/carddav',
  '/caldav',
  '/.well-known',
] as const;

function isBackendPath(pathname: string): boolean {
  return backendPathPrefixes.some(
    (prefix) => pathname === prefix || pathname.startsWith(`${prefix}/`),
  );
}

interface NavigationRequest {
  request: { mode: string };
  url: { pathname: string };
}

// True when a navigation request should be answered with the precached
// index.html app shell.
export function isAppShellNavigation({ request, url }: NavigationRequest): boolean {
  // If this isn't a navigation, skip.
  if (request.mode !== 'navigate') {
    return false;
  }

  // If this is a URL that starts with /_, skip.
  if (url.pathname.startsWith('/_')) {
    return false;
  }

  // If this looks like a URL for a resource, because it contains a file
  // extension, skip.
  if (url.pathname.match(fileExtensionRegexp)) {
    return false;
  }

  // Backend paths are not SPA routes, even when the browser is navigating.
  if (isBackendPath(url.pathname)) {
    return false;
  }

  return true;
}
