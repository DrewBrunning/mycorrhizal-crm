// API client with authentication and error handling
// Uses httpOnly cookies for authentication (credentials: 'include')
import { API_BASE_URL } from '../auth';

export { API_BASE_URL };

// Default request timeout in milliseconds (30 seconds). `?.` (not just `||`)
// because this module is also loaded directly by Node in the Playwright
// harness (e2e/global-setup.ts imports contacts.ts), where import.meta.env is
// undefined -- Vite always injects it, Node never does.
const DEFAULT_TIMEOUT = parseInt(import.meta.env?.VITE_REQUEST_TIMEOUT || '30000', 10);

// Backend error response structure
interface BackendErrorResponse {
  error: {
    code: string;
    message: string;
    details?: Record<string, string>;
  };
  request_id?: string;
  timestamp?: string;
}

// Custom API error class with detailed information
export class ApiError extends Error {
  code: string;
  status: number;
  details?: Record<string, string>;
  requestId?: string;

  constructor(
    message: string,
    code: string = 'UNKNOWN_ERROR',
    status: number = 500,
    details?: Record<string, string>,
    requestId?: string
  ) {
    super(message);
    this.name = 'ApiError';
    this.code = code;
    this.status = status;
    this.details = details;
    this.requestId = requestId;
  }

  // Get a user-friendly error message including field details.
  //
  // `details` carries two different things depending on the error code, and
  // conflating them was a real bug: this method used to join the detail
  // VALUES for every error that had any.
  //
  //   - VALIDATION_ERROR / INVALID_INPUT: details maps a field name to a
  //     human-readable message ({name: "Name is required"}), so joining the
  //     values is exactly right.
  //   - everything else: details carries machine context, not prose.
  //     `ErrNotFound("Contact").WithDetails("id", id)` — 77 call sites do
  //     this — meant a missing contact rendered to the user as a bare
  //     "99999999" instead of "Contact not found".
  //
  // So details are only folded in for the validation-shaped codes; every
  // other error shows its message.
  //
  // Server-fault errors (5xx) additionally append the request id. Every
  // backend error response and every backend log line carries that id, so it
  // is the only thing that lets a user's "it broke" be located in the logs —
  // but it was previously only ever console.error'd, which no user reads. A
  // 4xx is the user's own input and needs no correlation id.
  getDisplayMessage(): string {
    const detailsAreFieldMessages =
      this.code === 'VALIDATION_ERROR' || this.code === 'INVALID_INPUT';

    let message: string;
    if (detailsAreFieldMessages && this.details && Object.keys(this.details).length > 0) {
      message = Object.entries(this.details)
        .map(([, msg]) => `${msg}`)
        .join('. ');
    } else {
      message = this.message;
    }

    if (this.status >= 500 && this.requestId) {
      return `${message} (ref: ${this.requestId})`;
    }
    return message;
  }
}

// Parse error response from backend
export async function parseErrorResponse(response: Response): Promise<ApiError> {
  try {
    const data: BackendErrorResponse = await response.json();
    if (data.error) {
      return new ApiError(
        data.error.message,
        data.error.code,
        response.status,
        data.error.details,
        data.request_id
      );
    }
  } catch {
    // JSON parsing failed, fall back to status text
  }
  
  return new ApiError(
    response.statusText || 'An error occurred',
    'UNKNOWN_ERROR',
    response.status
  );
}

// Centralized fetch wrapper that handles session expiration and timeouts
export async function apiFetch(
  url: string,
  options: RequestInit = {},
  timeout: number = DEFAULT_TIMEOUT
): Promise<Response> {
  // Create AbortController for timeout
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), timeout);

  try {
    const response = await fetch(url, {
      ...options,
      credentials: 'include', // Send httpOnly auth cookie
      signal: controller.signal,
    });

    clearTimeout(timeoutId);

    // Check if session has expired (401 Unauthorized)
    if (response.status === 401) {
      // Session is invalid or expired - logout and redirect
      localStorage.removeItem('user_info');
      window.location.href = '/login';
      throw new Error('Session expired. Please login again.');
    }

    return response;
  } catch (error) {
    clearTimeout(timeoutId);
    
    // Handle timeout error
    if (error instanceof Error && error.name === 'AbortError') {
      throw new Error(`Request timeout after ${timeout / 1000} seconds. Please check your connection.`);
    }
    
    throw error;
  }
}

// Helper to create request headers (no longer includes Authorization - using cookies)
export function getAuthHeaders(): HeadersInit {
  return {
    'Content-Type': 'application/json',
  };
}
