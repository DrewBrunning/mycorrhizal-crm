/**
 * An Error whose message is meant for the user: text its thrower already
 * translated, or a server message meant for display (#2920).
 *
 * getErrorMessage() (./errorHandler) shows a UserFacingError's message as-is.
 * It never shows a plain Error's message, which is developer text and usually
 * English ("Cannot read properties of undefined"). Throw this, not a plain
 * Error, when a catch block further up should show the message.
 *
 * A leaf module with no imports, so the API layer (auth.ts, api/*) can throw
 * it without importing errorHandler's dependencies.
 */
export class UserFacingError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'UserFacingError';
  }
}
