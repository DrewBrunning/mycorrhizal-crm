/**
 * Centralized error handling utility for consistent error management across the application.
 *
 * This module provides:
 * - Consistent error message extraction from various error types
 * - Structured logging with context information
 * - Integration with the snackbar notification system
 */

import { ApiError } from '../api/client';

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

/**
 * Extract a user-friendly error message from any error type
 */
export function getErrorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    return error.getDisplayMessage();
  }

  if (error instanceof Error) {
    return error.message;
  }

  if (typeof error === 'string') {
    return error;
  }

  return 'An unexpected error occurred';
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

  console.error(`[${context.operation}] Error:`, {
    message,
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

  if (notifier) {
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
