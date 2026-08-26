import { describe, expect, test } from 'vitest';
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
    const err = new ApiError(
      'Database operation failed',
      'DATABASE_ERROR',
      500,
      undefined,
      'req-abc-123',
    );
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
      'req-1',
    );
    expect(err.getDisplayMessage()).toBe('Name is too long (ref: req-1)');
  });
});

// `details` carries field messages for validation errors but machine context
// for everything else. This method used to join the values regardless, so a
// missing contact — ErrNotFound("Contact").WithDetails("id", id), which 77
// backend call sites produce — rendered to the user as a bare "99999999".
// Caught by the prep-view e2e spec, which found the alert showing just an ID.
describe('ApiError.getDisplayMessage details handling', () => {
  test('shows the message, not the id, for a not-found error carrying context details', () => {
    const err = new ApiError('Contact not found', 'NOT_FOUND', 404, { id: '99999999' });
    expect(err.getDisplayMessage()).toBe('Contact not found');
  });

  test('folds in field messages for a validation error', () => {
    const err = new ApiError('Request validation failed', 'VALIDATION_ERROR', 400, {
      name: 'Name is required',
    });
    expect(err.getDisplayMessage()).toBe('Name is required');
  });

  test('folds in field messages for an invalid-input error', () => {
    const err = new ApiError('Invalid input', 'INVALID_INPUT', 400, {
      reason: 'limit must be a positive integer',
    });
    expect(err.getDisplayMessage()).toBe('limit must be a positive integer');
  });

  test('joins multiple field messages', () => {
    const err = new ApiError('Request validation failed', 'VALIDATION_ERROR', 400, {
      name: 'Name is required',
      email: 'Email is invalid',
    });
    expect(err.getDisplayMessage()).toBe('Name is required. Email is invalid');
  });

  test('ignores context details on a conflict error', () => {
    const err = new ApiError('Already exists', 'ALREADY_EXISTS', 409, { circle_id: 'abc-123' });
    expect(err.getDisplayMessage()).toBe('Already exists');
  });
});
