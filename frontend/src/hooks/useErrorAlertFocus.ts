import { useCallback, useEffect, useRef, useState } from 'react';

/**
 * Issue #192: a failed form submission (login, register, password reset)
 * used to drop keyboard focus to <body> with no programmatic link between
 * the error and the fields it concerns. This hook owns the error string
 * alongside a ref that gets focused whenever a new error is reported --
 * `<Alert id={...} ref={errorRef} tabIndex={-1}>` picks up the ref, and
 * callers reference the same id via `aria-describedby` on the relevant
 * fields.
 *
 * Reporting the exact same message twice in a row must still move focus:
 * a plain `useState<string>` + `useEffect(() => {...}, [error])` breaks for
 * that repeat case. React bails out of re-rendering (and therefore effects)
 * when a state update is Object.is-equal to the current value, so two
 * consecutive validation failures with identical text would move focus only
 * the first time -- e.g. ForgotPasswordDialog's password-mismatch check,
 * which (unlike the empty-field checks) isn't redundant with any TextField's
 * `required` attribute, so a user can genuinely submit the same mismatch
 * twice. A monotonically-increasing nonce, bumped every time a non-empty
 * error is reported, sidesteps the bailout: the effect keys on the nonce,
 * not the message text, so it always fires on a fresh report regardless of
 * whether the text happens to repeat.
 */
export function useErrorAlertFocus(initial: string | (() => string) = '') {
  const [error, setErrorMessage] = useState(initial);
  // Seeds from `error` (not `initial` again) so a lazy initializer function
  // is only ever invoked once, the same as a plain useState call would.
  const [reportNonce, setReportNonce] = useState(() => (error ? 1 : 0));
  const errorRef = useRef<HTMLDivElement>(null);

  // useState setters are guaranteed stable across renders; wrapping in
  // useCallback keeps that same guarantee for this hook's own setError, so
  // it's safe for a caller to list it in another effect's dependency array
  // (as ForgotPasswordDialog's open-reset effect does) without it forcing
  // that effect to re-run every render.
  const setError = useCallback((message: string) => {
    setErrorMessage(message);
    if (message) setReportNonce((n) => n + 1);
  }, []);

  useEffect(() => {
    if (reportNonce > 0) errorRef.current?.focus();
  }, [reportNonce]);

  return { error, setError, errorRef };
}
