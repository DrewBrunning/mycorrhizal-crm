import type { Circle } from '../api/circles';
import { ApiError } from '../api/client';

// Reason code the server sends (409) when a circle name is already taken by
// another of the user's circles (issue #2362). Create and rename only.
export const CIRCLE_ALREADY_EXISTS_REASON = 'circle_already_exists';

export function isDuplicateCircleNameError(err: unknown): err is ApiError {
  return (
    err instanceof ApiError && err.status === 409 && err.reasonCode === CIRCLE_ALREADY_EXISTS_REASON
  );
}

// Name order used for every circle list: case-insensitive, then id so the order
// is total. The server's list order is its pagination cursor (updated_at, id),
// so clients present the name order themselves.
export function compareCirclesByName(a: Circle, b: Circle): number {
  return (
    a.name.localeCompare(b.name, undefined, { sensitivity: 'base' }) || a.id.localeCompare(b.id)
  );
}
