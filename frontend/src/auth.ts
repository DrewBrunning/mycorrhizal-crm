// src/auth.ts
// Cookie-based JWT auth service for mycorrhizal crm frontend
// Token is stored in httpOnly cookie (not accessible from JS for security)
// User info is cached in localStorage for UI purposes

// `?.` (not just `||`) because this module is also loaded directly by Node in
// the Playwright harness (e2e imports contacts.ts → client.ts → auth.ts),
// where import.meta.env is undefined -- Vite always injects it, Node never
// does. The optional chain also prevents Vite from statically inlining the
// env value at build time (the property lookup survives as runtime code),
// which is a deliberate trade-off for cross-runtime compatibility.
const API_SERVER_URL = import.meta.env?.VITE_API_URL || '';
export const API_BASE_URL = `${API_SERVER_URL}/api/v1`;

const USER_INFO_KEY = 'user_info';

export interface LoginResponse {
  language?: string;
  date_format?: string;
  // N8: present and true when the account has 2FA enabled. In that case NO
  // session exists yet — call login2FA() with a TOTP/recovery code to
  // complete the login.
  two_factor_required?: boolean;
}

export interface LogoutResponse {
  redirect_url?: string;
}

export interface UserInfo {
  user_id: number;
  username: string;
  is_admin: boolean;
  self_contact_vcard_uid?: string | null;
}

export async function loginUser(identifier: string, password: string): Promise<LoginResponse> {
  const response = await fetch(`${API_BASE_URL}/login`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    credentials: 'include', // Send and receive cookies
    body: JSON.stringify({ identifier, password }),
  });
  if (!response.ok) {
    throw new Error('Login failed');
  }
  const data = await response.json();

  // When 2FA is required the server set a short-lived 2fa_pending cookie but
  // NO session — user info must not be cached until the second step succeeds.
  if (data.two_factor_required) {
    return { two_factor_required: true };
  }

  // Fetch user info and cache it (since we can't read the httpOnly cookie)
  await fetchAndCacheUserInfo();

  return {
    language: data.language,
    date_format: data.date_format,
  };
}

// login2FA completes interactive login for accounts with 2FA enabled: the
// server requires the 2fa_pending cookie from the preceding /login call, so
// this must run in the same browser context as loginUser.
export async function login2FA(code: string): Promise<LoginResponse> {
  const response = await fetch(`${API_BASE_URL}/login/2fa`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    credentials: 'include',
    body: JSON.stringify({ code }),
  });
  if (!response.ok) {
    // A 429 is the account lockout from repeated bad codes — the backend
    // body carries the retry_after, so surface that instead of a generic
    // "Invalid code" that would make a locked-out user keep guessing.
    if (response.status === 429) {
      let message = 'Too many failed attempts. Please try again later.';
      try {
        const data = await response.json();
        if (data?.message) message = data.message;
      } catch {
        // fall back to the generic lockout message
      }
      throw new Error(message);
    }
    throw new Error('Invalid code');
  }
  const data = await response.json();

  await fetchAndCacheUserInfo();

  return {
    language: data.language,
    date_format: data.date_format,
  };
}

// Fetch current user info from the server and cache it
export async function fetchAndCacheUserInfo(): Promise<UserInfo | null> {
  try {
    const response = await fetch(`${API_BASE_URL}/users/me`, {
      credentials: 'include',
    });
    if (!response.ok) {
      return null;
    }
    const data = await response.json();
    const userInfo: UserInfo = {
      user_id: data.ID,
      username: data.Username,
      is_admin: data.is_admin || false,
      self_contact_vcard_uid: data.self_contact_vcard_uid ?? null,
    };
    localStorage.setItem(USER_INFO_KEY, JSON.stringify(userInfo));
    return userInfo;
  } catch {
    return null;
  }
}

// Returns null - token is in httpOnly cookie (not accessible from JS)
// Use isAuthenticated() to check login status
export function getToken(): string | null {
  // Return a placeholder if user info exists (indicates logged in)
  // This maintains compatibility with existing code that checks for token presence
  const userInfo = localStorage.getItem(USER_INFO_KEY);
  return userInfo ? 'cookie-auth' : null;
}

// T90: the cached self-contact VCardUID, or null when none is set or the cache
// is absent/corrupt. The badge on the contact detail page compares a contact's
// uid against this. Callers that just PATCHed the self contact should call
// fetchAndCacheUserInfo() first so this reflects the change.
export function getCachedSelfContactVCardUID(): string | null {
  const userInfoStr = localStorage.getItem(USER_INFO_KEY);
  if (!userInfoStr) return null;
  try {
    const userInfo: UserInfo = JSON.parse(userInfoStr);
    return userInfo.self_contact_vcard_uid ?? null;
  } catch {
    return null;
  }
}

// Check if user is authenticated (has cached user info)
export function isAuthenticated(): boolean {
  return localStorage.getItem(USER_INFO_KEY) !== null;
}

// Returns the provider's RP-Initiated Logout URL when this session was
// authenticated via OIDC, so the caller can navigate there to also end the
// IdP's own session (otherwise "Sign in with SSO" silently re-authenticates
// without a prompt).
async function logoutUser(): Promise<string | undefined> {
  let redirectURL: string | undefined;
  try {
    const response = await fetch(`${API_BASE_URL}/logout`, {
      method: 'POST',
      credentials: 'include',
    });
    const data: LogoutResponse | null = await response.json().catch(() => null);
    redirectURL = data?.redirect_url;
  } catch {
    // Ignore errors - clear local state anyway
  }
  localStorage.removeItem(USER_INFO_KEY);
  return redirectURL;
}

export async function logoutAndRedirect() {
  const redirectURL = await logoutUser();
  // A real top-level navigation (not fetch) is required so the IdP can clear
  // its own first-party session cookie.
  window.location.href = redirectURL || '/login';
}

interface DecodedToken {
  user_id: number;
  username: string;
  is_admin: boolean;
  exp: number;
}

// Returns cached user info (previously decoded from token)
function decodeToken(): DecodedToken | null {
  const userInfoStr = localStorage.getItem(USER_INFO_KEY);
  if (!userInfoStr) {
    return null;
  }

  try {
    const userInfo: UserInfo = JSON.parse(userInfoStr);
    return {
      user_id: userInfo.user_id,
      username: userInfo.username,
      is_admin: userInfo.is_admin,
      exp: 0, // Expiry handled server-side via cookie
    };
  } catch {
    return null;
  }
}

// Check if the current user is an admin based on cached user info
export function isAdmin(): boolean {
  const decoded = decodeToken();
  return decoded?.is_admin || false;
}
