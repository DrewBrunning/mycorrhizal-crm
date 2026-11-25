/**
 * Centralized error handling utility for consistent error management across the application.
 *
 * This module provides:
 * - Consistent error message extraction from various error types
 * - Structured logging with context information
 * - Integration with the snackbar notification system
 */

import { ApiError } from '../api/client';
import i18n from '../i18n/config';
import { UserFacingError } from './userFacingError';

/**
 * Error context for logging and display
 */
export interface ErrorContext {
  /** The operation being performed (e.g., 'saving reminder', 'fetching contacts') */
  operation: string;
  /** Whether to suppress console logging (useful during tests) */
  silent?: boolean;
}

/**
 * Callbacks for error notifications
 */
export interface ErrorNotifier {
  showError: (message: string) => void;
}

// Fetch rejects with a TypeError on a network failure; the other messages are
// what some engines/browsers use instead (Safari, older Chrome).
const NETWORK_ERROR_MESSAGES = new Set([
  'Failed to fetch',
  'NetworkError when attempting to fetch resource.',
  'Load failed',
  'Network request failed',
  'The Internet connection appears to be offline.',
  'The network connection was lost.',
]);

function isNetworkError(error: unknown): boolean {
  // Match the known messages rather than every TypeError: a programming bug
  // ("Cannot read properties of undefined") is not a network failure and its
  // real message is more useful than a made-up offline state.
  return error instanceof Error && NETWORK_ERROR_MESSAGES.has(error.message);
}

function isOffline(): boolean {
  return typeof navigator !== 'undefined' && navigator.onLine === false;
}

// The message a user may see for [error], or undefined when it carries none
// (#2920). A plain Error's message is developer text -- usually English, e.g.
// "Cannot read properties of undefined" -- so only a network failure, an
// ApiError, a UserFacingError or a thrown string yields one.
function displayMessageOf(error: unknown): string | undefined {
  // A raw "Failed to fetch" tells the user nothing (#1915). Name the
  // condition instead, and distinguish a browser that is offline from a
  // reachable browser whose server is down.
  if (isNetworkError(error)) {
    return i18n.t(isOffline() ? 'common.offlineMessage' : 'common.networkError');
  }

  if (error instanceof ApiError) {
    return error.getDisplayMessage();
  }

  if (error instanceof UserFacingError) {
    // An empty message carries nothing to show; let the fallback speak.
    return error.message || undefined;
  }

  if (typeof error === 'string') {
    return error;
  }

  return undefined;
}

/**
 * Extract a user-friendly error message from any error type. Anything that
 * carries no user-facing text (a plain Error, a non-Error value) maps to the
 * translated generic `common.unexpectedError`, never to its raw message.
 */
export function getErrorMessage(error: unknown): string {
  return displayMessageOf(error) ?? i18n.t('common.unexpectedError');
}

/**
 * The display message for a thrown error, or [fallback] (the caller's own
 * translated "… failed" text) when it carries no user-facing text: a plain
 * Error, whose message is developer text (#2920), or a non-Error value.
 * One call instead of the `err instanceof Error ? getErrorMessage(err) :
 * fallback` ternary repeated across catch blocks.
 */
export function getErrorMessageOr(error: unknown, fallback: string): string {
  if (!(error instanceof Error)) return fallback;
  return displayMessageOf(error) ?? fallback;
}

/**
 * Get the error code if available
 */
function getErrorCode(error: unknown): string | undefined {
  if (error instanceof ApiError) {
    return error.code;
  }
  return undefined;
}

/**
 * Log an error with consistent formatting
 */
function logError(error: unknown, context: ErrorContext): void {
  if (context.silent) {
    return;
  }

  const message = getErrorMessage(error);
  const code = getErrorCode(error);
  const requestId = error instanceof ApiError ? error.requestId : undefined;
  // The raw message is for the developer reading the console, never the UI.
  const rawMessage = error instanceof Error ? error.message : undefined;

  console.error(`[${context.operation}] Error:`, {
    message,
    rawMessage,
    code,
    requestId,
    error,
  });
}

/**
 * Handle an error by logging it and optionally notifying the user.
 * Use this for operations where the error should be visible to users.
 *
 * @param error - The error to handle
 * @param context - Context about the operation
 * @param notifier - Optional notifier to show user-facing messages
 * @returns The user-friendly error message
 *
 * @example
 * ```ts
 * try {
 *   await saveReminder(data);
 * } catch (err) {
 *   handleError(err, { operation: 'saving reminder' }, { showError });
 *   throw err;
 * }
 * ```
 */
export function handleError(
  error: unknown,
  context: ErrorContext,
  notifier?: ErrorNotifier,
): string {
  const message = getErrorMessage(error);

  logError(error, context);

  // Issue #557: a SESSION_EXPIRED 401 already surfaces through
  // <SessionExpiredGate> (a passive banner or a blocking re-auth prompt,
  // depending on what triggered it) -- showing it a second time as a plain
  // error toast here would be redundant and, for a background request a user
  // never asked for, a more confusing message than the one they already see.
  if (notifier && getErrorCode(error) !== 'SESSION_EXPIRED') {
    notifier.showError(message);
  }

  return message;
}

/**
 * Hook-friendly error handler that returns a consistent error message.
 * Use this in hooks that need to set error state.
 *
 * @example
 * ```ts
 * const fetchData = async () => {
 *   try {
 *     const data = await api.getData();
 *     setData(data);
 *   } catch (err) {
 *     setError(handleFetchError(err, 'fetching data'));
 *   }
 * };
 * ```
 */
export function handleFetchError(error: unknown, operation: string): string {
  logError(error, { operation });
  return getErrorMessage(error);
}
