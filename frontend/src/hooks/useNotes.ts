// Custom hook for fetching and managing notes
import { useState, useEffect, useCallback } from 'react';
import { isAuthenticated } from '../auth';
import {
  getUnassignedNotes,
  getContactNotes,
  Note,
  GetNotesParams,
} from '../api/notes';
import { handleFetchError } from '../utils/errorHandler';

interface UseNotesResult {
  notes: Note[];
  // Opaque resume token (T17): non-empty while more rows exist.
  nextCursor: string;
  limit: number;
  loading: boolean;
  error: string | null;
  refetch: () => Promise<void>;
  loadMore: () => Promise<void>;
}

export function useNotes(
	contactId?: string | number,
	params: GetNotesParams = {}
): UseNotesResult {
  // Destructure params to use primitive values as dependencies
  // This prevents re-fetches when callers pass new object references with identical values
  const { cursor: _ignored, limit: paramLimit, search, fromDate, toDate } = params;

  const [notes, setNotes] = useState<Note[]>([]);
  const [nextCursor, setNextCursor] = useState('');
  const [limit, setLimit] = useState(paramLimit || 25);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchFirst = useCallback(async () => {
    setLoading(true);
    setError(null);

    try {
      if (!isAuthenticated()) {
        throw new Error('No authentication token found');
      }

      if (contactId) {
        const data = await getContactNotes(contactId);
        const normalized = Array.isArray(data) ? data : data.notes || [];
        setNotes(normalized);
        setNextCursor('');
        setLimit(normalized.length || paramLimit || 25);
      } else {
        const data = await getUnassignedNotes({ limit: paramLimit, search, fromDate, toDate });
        setNotes(data.notes || []);
        setNextCursor(data.next_cursor || '');
        setLimit(data.limit || paramLimit || 25);
      }
    } catch (err) {
      const message = handleFetchError(err, 'fetching notes');
      setError(message);
    } finally {
      setLoading(false);
    }
  }, [contactId, paramLimit, search, fromDate, toDate]);

  const loadMore = useCallback(async () => {
    if (!nextCursor) return;
    setLoading(true);
    setError(null);
    try {
      const data = await getUnassignedNotes({ cursor: nextCursor, limit: paramLimit, search, fromDate, toDate });
      setNotes((prev) => [...prev, ...(data.notes || [])]);
      setNextCursor(data.next_cursor || '');
    } catch (err) {
      setError(handleFetchError(err, 'loading more notes'));
    } finally {
      setLoading(false);
    }
  }, [nextCursor, paramLimit, search, fromDate, toDate]);

  useEffect(() => {
    fetchFirst();
  }, [fetchFirst]);

  return {
    notes,
    nextCursor,
    limit,
    loading,
    error,
    refetch: fetchFirst,
    loadMore,
  };
}
