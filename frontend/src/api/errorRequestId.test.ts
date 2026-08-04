import { describe, test, expect } from 'vitest';
import { ApiError } from './client';

// Every backend error response and every backend log line carries a request_id
// (errors/middleware.go's RespondWithError + LogError). It was parsed into
// ApiError and then only ever console.error'd — which no user reads. A user
// reporting "it broke" had nothing to quote, so their failure could not be
// found in the logs.
//
// 5xx now carries the id into the user-facing message. 4xx does not: that is
// the user's own input, they need to fix a field, not file a report.
describe('ApiError.getDisplayMessage request id', () => {
  test('appends the request id on a server error', () => {
    const err = new ApiError('Database operation failed', 'DATABASE_ERROR', 500, undefined, 'req-abc-123');
    expect(err.getDisplayMessage()).toBe('Database operation failed (ref: req-abc-123)');
  });

  test('appends it on other 5xx statuses too', () => {
    const err = new ApiError('Service unavailable', 'INTERNAL', 503, undefined, 'req-xyz');
    expect(err.getDisplayMessage()).toContain('(ref: req-xyz)');
  });

  test('does NOT append it on a client error', () => {
    const err = new ApiError('Name is required', 'VALIDATION_ERROR', 400, undefined, 'req-abc-123');
    expect(err.getDisplayMessage()).toBe('Name is required');
  });

  test('does not append it on a 404', () => {
    const err = new ApiError('Contact not found', 'NOT_FOUND', 404, undefined, 'req-abc-123');
    expect(err.getDisplayMessage()).toBe('Contact not found');
  });

  test('omits the suffix entirely when no request id was returned', () => {
    const err = new ApiError('Something failed', 'INTERNAL', 500);
    expect(err.getDisplayMessage()).toBe('Something failed');
  });

  test('still prefers field details, and appends the id after them on 5xx', () => {
    const err = new ApiError(
      'Validation failed',
      'VALIDATION_ERROR',
      500,
      { name: 'Name is too long' },
      'req-1'
    );
    expect(err.getDisplayMessage()).toBe('Name is too long (ref: req-1)');
  });
});
