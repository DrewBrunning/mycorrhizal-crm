// Custom hook for fetching and managing notes
import { useCallback, useEffect, useState } from 'react';
import { type GetNotesParams, getContactNotes, getUnassignedNotes, type Note } from '../api/notes';
import { isAuthenticated } from '../auth';
import { handleFetchError } from '../utils/errorHandler';

interface UseNotesResult {
  notes: Note[];
  // Opaque resume token (T17): non-empty while more rows exist.
  nextCursor: string;
  // Queue depth for the N4 inbox: how many notes match the current filters in
  // TOTAL, which is not the same as notes.length once pagination kicks in.
  // Rendering notes.length in the inbox chip under-counted anyone with more
  // than one page of unfiled notes, and grew as they clicked "Load more".
  // Falls back to notes.length for the contact-scoped list, which is
  // unpaginated and returns no total.
  total: number;
  limit: number;
  loading: boolean;
  error: string | null;
  refetch: () => Promise<void>;
  loadMore: () => Promise<void>;
}

export function useNotes(contactId?: string | number, params: GetNotesParams = {}): UseNotesResult {
  // Destructure params to use primitive values as dependencies
  // This prevents re-fetches when callers pass new object references with identical values
  const { cursor: _ignored, limit: paramLimit, search, fromDate, toDate } = params;

  const [notes, setNotes] = useState<Note[]>([]);
  const [nextCursor, setNextCursor] = useState('');
  const [total, setTotal] = useState<number | null>(null);
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
        setTotal(null);
        setLimit(normalized.length || paramLimit || 25);
      } else {
        const data = await getUnassignedNotes({ limit: paramLimit, search, fromDate, toDate });
        setNotes(data.notes || []);
        setNextCursor(data.next_cursor || '');
        setTotal(data.total ?? null);
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
      const data = await getUnassignedNotes({
        cursor: nextCursor,
        limit: paramLimit,
        search,
        fromDate,
        toDate,
      });
      setNotes((prev) => [...prev, ...(data.notes || [])]);
      setNextCursor(data.next_cursor || '');
      // The server recomputes the total each page; keeping it in sync means a
      // note filed in another tab is reflected as the user pages.
      if (data.total !== undefined) setTotal(data.total);
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
    total: total ?? notes.length,
    limit,
    loading,
    error,
    refetch: fetchFirst,
    loadMore,
  };
}
