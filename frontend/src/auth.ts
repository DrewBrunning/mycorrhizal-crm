// src/auth.ts
// Cookie-based JWT auth service for mycorrhizal crm frontend
// Token is stored in httpOnly cookie (not accessible from JS for security)
// User info is cached in localStorage for UI purposes

import i18n from './i18n/config';
import { localizeReason } from './i18n/reason';
import { initializeTimeFormatFromBackend } from './timeFormat';
import { clearAllSessionDrafts } from './utils/sessionDraftStorage';
import { UserFacingError } from './utils/userFacingError';
import { getAssertion, type WireRequestOptions } from './webauthnCeremony';

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
  time_format?: string;
  // N8: present and true when the account has 2FA enabled. In that case NO
  // session exists yet — call login2FA() with a TOTP/recovery code to
  // complete the login.
  two_factor_required?: boolean;
  // Issue #594: the second-factor methods the account can use at the 2FA
  // step. Absent on an older-shaped response, which means TOTP only.
  methods?: SecondFactorMethod[];
}

export type SecondFactorMethod = 'totp' | 'webauthn';

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
    throw new UserFacingError(await signInErrorMessage(response, i18n.t('login.loginFailed')));
  }
  const data = await response.json();

  // When 2FA is required the server set a short-lived 2fa_pending cookie but
  // NO session — user info must not be cached until the second step succeeds.
  if (data.two_factor_required) {
    return {
      two_factor_required: true,
      methods: Array.isArray(data.methods) ? data.methods : ['totp'],
    };
  }

  // Fetch user info and cache it (since we can't read the httpOnly cookie)
  await fetchAndCacheUserInfo();

  return {
    language: data.language,
    date_format: data.date_format,
    time_format: data.time_format,
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
    // "Invalid code" that would make a locked-out user keep guessing. Any
    // other failure shows its localized reason code (issue #2101).
    throw new UserFacingError(
      await signInErrorMessage(
        response,
        response.status === 429 ? i18n.t('login.tooManyAttempts') : i18n.t('login.invalidCode'),
      ),
    );
  }
  const data = await response.json();

  await fetchAndCacheUserInfo();

  return {
    language: data.language,
    date_format: data.date_format,
    time_format: data.time_format,
  };
}

// loginWithPasskey is the passkey alternative to login2FA (issue #594): the
// same 2fa_pending cookie, but the second factor is a WebAuthn assertion from
// navigator.credentials.get. A dismissed browser prompt rejects with the
// browser's DOMException (see isCeremonyCancelled).
export async function loginWithPasskey(): Promise<LoginResponse> {
  const begin = await fetch(`${API_BASE_URL}/webauthn/login/begin`, {
    method: 'POST',
    credentials: 'include',
  });
  if (!begin.ok) {
    throw new UserFacingError(await signInErrorMessage(begin, i18n.t('login.passkeyStartFailed')));
  }
  const assertion = await getAssertion(await begin.json());

  const finish = await fetch(`${API_BASE_URL}/webauthn/login/finish`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include',
    body: JSON.stringify(assertion),
  });
  if (!finish.ok) {
    throw new UserFacingError(
      await signInErrorMessage(finish, i18n.t('login.passkeyVerifyFailed')),
    );
  }
  const data = await finish.json();

  await fetchAndCacheUserInfo();

  return {
    language: data.language,
    date_format: data.date_format,
    time_format: data.time_format,
  };
}

// loginWithPasskeyPasswordless is the passwordless sign-in path: a discoverable
// passkey (fingerprint/face/PIN) identifies the account, so there is no
// username, password or 2fa_pending challenge. The server returns a short-lived
// ceremony token alongside the WebAuthn options and verifies it on finish.
export async function loginWithPasskeyPasswordless(): Promise<LoginResponse> {
  const begin = await fetch(`${API_BASE_URL}/webauthn/login/passwordless/begin`, {
    method: 'POST',
    credentials: 'include',
  });
  if (!begin.ok) {
    throw new UserFacingError(await signInErrorMessage(begin, i18n.t('login.passkeyStartFailed')));
  }
  const options = (await begin.json()) as WireRequestOptions & { ceremony_token?: unknown };
  if (typeof options.ceremony_token !== 'string' || options.ceremony_token === '') {
    throw new UserFacingError(i18n.t('login.passkeyStartFailed'));
  }
  const assertion = await getAssertion(options);

  const finish = await fetch(`${API_BASE_URL}/webauthn/login/passwordless/finish`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include',
    body: JSON.stringify({ ceremony_token: options.ceremony_token, assertion }),
  });
  if (!finish.ok) {
    throw new UserFacingError(
      await signInErrorMessage(finish, i18n.t('login.passkeyVerifyFailed')),
    );
  }
  const data = await finish.json();

  await fetchAndCacheUserInfo();

  return {
    language: data.language,
    date_format: data.date_format,
    time_format: data.time_format,
  };
}

type ReasonParams = Record<string, string | number>;

const ACCOUNT_LOCKED_REASON = 'auth_account_locked';

type ErrorBody = {
  message?: unknown;
  reason_code?: unknown;
  reason_params?: ReasonParams;
  retry_after?: unknown;
  details?: { retry_after?: unknown };
};

// The message to show for an error body from the sign-in endpoints. The
// account-lockout 429 is, today, a flat body whose fields sit at the top level
// (issue #2101, ADR 0038); every other failure is the error envelope with them
// under `error`. The flat lockout is deprecated for removal in v2.0.0 (issues
// #2107, #2108), when it becomes the envelope (code ACCOUNT_LOCKED, retry values
// under error.details), so both shapes resolve through the same lookup here,
// falling back to the server's English `message` when the client cannot render
// the code.
function serverErrorMessage(data: unknown): string | undefined {
  if (typeof data !== 'object' || data === null) return undefined;
  const flat = data as ErrorBody & { error?: ErrorBody | string };
  const envelope = typeof flat.error === 'object' ? flat.error : undefined;
  const code = flat.reason_code ?? envelope?.reason_code;
  let params = flat.reason_params ?? envelope?.reason_params;
  if (code === ACCOUNT_LOCKED_REASON && params?.retry_after === undefined) {
    // retry_after at the top level (flat) or under error.details (envelope).
    const retryAfter = flat.retry_after ?? envelope?.details?.retry_after;
    if (typeof retryAfter === 'number') params = { ...params, retry_after: retryAfter };
  }
  const localized = typeof code === 'string' ? localizeReason(code, params) : undefined;
  if (localized) return localized;
  // 429 lockout carries a top-level message; other failures nest it.
  const message = flat.message ?? envelope?.message;
  return typeof message === 'string' && message ? message : undefined;
}

// The localized message for a failed sign-in response (password, 2FA code or
// passkey), or `fallback` when the body carries neither a renderable reason
// code nor a message.
async function signInErrorMessage(response: Response, fallback: string): Promise<string> {
  try {
    const data = await response.json();
    const message = serverErrorMessage(data);
    if (message) return message;
  } catch {
    // fall through
  }
  return fallback;
}

// Fetch current user info from the server and cache it
// What /users/me returns beyond the cached identity: the account's saved UI
// language, so a start-up refresh can apply a change made on another device.
export interface FetchedUserInfo extends UserInfo {
  language?: string;
}

export async function fetchAndCacheUserInfo(): Promise<FetchedUserInfo | null> {
  try {
    const response = await fetch(`${API_BASE_URL}/users/me`, {
      credentials: 'include',
    });
    if (!response.ok) {
      return null;
    }
    const data = await response.json();
    const userInfo: UserInfo = {
      user_id: data.id,
      username: data.username,
      is_admin: data.is_admin || false,
      self_contact_vcard_uid: data.self_contact_vcard_uid ?? null,
    };
    localStorage.setItem(USER_INFO_KEY, JSON.stringify(userInfo));
    // Issue #1830: keep the time-format preference in step with the server (it
    // may have been changed on another device).
    initializeTimeFormatFromBackend(data.time_format);
    return { ...userInfo, language: typeof data.language === 'string' ? data.language : undefined };
  } catch {
    return null;
  }
}

// #2718: the session cookie is httpOnly, so JS cannot tell whether one exists.
// A browser with no cached user can only have a live session by coming back
// from the SSO redirect, so that round trip leaves this one-shot hint in
// sessionStorage. Without it (a plain signed-out visit) the start-up
// /users/me probe would only produce a 401 console error.
const OIDC_PENDING_KEY = 'oidc_pending';

// Start the SSO sign-in: leave the hint, then hand the tab to the provider.
export function startOidcLogin(): void {
  try {
    sessionStorage.setItem(OIDC_PENDING_KEY, '1');
  } catch {
    // Storage blocked: the return trip then relies on a manual reload.
  }
  window.location.href = `${API_BASE_URL}/auth/oidc/login`;
}

// Whether the app should ask /users/me at start: always for a cached user (to
// refresh preferences such as the language), and for an uncached browser only
// right after the SSO redirect. Reading consumes the one-shot SSO hint.
export function shouldProbeSession(): boolean {
  let returningFromSso = false;
  try {
    returningFromSso = sessionStorage.getItem(OIDC_PENDING_KEY) !== null;
    sessionStorage.removeItem(OIDC_PENDING_KEY);
  } catch {
    // Storage blocked.
  }
  return returningFromSso || localStorage.getItem(USER_INFO_KEY) !== null;
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
  // Drafts belong to the signed-in user: an explicit logout must not leave
  // note/activity content for the next account on this tab to restore.
  clearAllSessionDrafts();
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
