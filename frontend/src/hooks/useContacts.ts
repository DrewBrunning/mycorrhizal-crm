// Custom hook for fetching and managing contacts
import { useState, useEffect, useCallback } from 'react';
import { isAuthenticated } from '../auth';
import {
  getContacts,
  GetContactsParams,
  Contact
} from '../api/contacts';
import { handleFetchError } from '../utils/errorHandler';

interface UseContactsResult {
  contacts: Contact[];
  // Opaque resume token (T17): non-empty while more rows exist. There is no
  // exact total — cursor pagination gives it up.
  nextCursor: string;
  loading: boolean;
  error: string | null;
  refetch: () => Promise<void>;
  loadMore: () => Promise<void>;
}

export function useContacts(params: GetContactsParams = {}): UseContactsResult {
  // Destructure params to use primitive values as dependencies
  // This prevents re-fetches when callers pass new object references with identical values
  const { cursor: _ignored, limit, search, circle, sort, order, includeArchived, archived } = params;

  const [contacts, setContacts] = useState<Contact[]>([]);
  const [nextCursor, setNextCursor] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // fetchFirst replaces the list (page one); loadMore appends the next page.
  const fetchFirst = useCallback(async () => {
    setLoading(true);
    setError(null);

    try {
      if (!isAuthenticated()) {
        throw new Error('No authentication token found');
      }

      const data = await getContacts({
        limit,
        search,
        circle,
        sort,
        order,
        includeArchived,
        archived,
      });
      setContacts(data.contacts || []);
      setNextCursor(data.next_cursor || '');
    } catch (err) {
      const message = handleFetchError(err, 'fetching contacts');
      setError(message);
    } finally {
      setLoading(false);
    }
  }, [limit, search, circle, sort, order, includeArchived, archived]);

  const loadMore = useCallback(async () => {
    if (!nextCursor) return;
    setLoading(true);
    setError(null);
    try {
      const data = await getContacts({
        cursor: nextCursor,
        limit,
        search,
        circle,
        sort,
        order,
        includeArchived,
        archived,
      });
      setContacts((prev) => [...prev, ...(data.contacts || [])]);
      setNextCursor(data.next_cursor || '');
    } catch (err) {
      setError(handleFetchError(err, 'loading more contacts'));
    } finally {
      setLoading(false);
    }
  }, [nextCursor, limit, search, circle, sort, order, includeArchived, archived]);

  useEffect(() => {
    fetchFirst();
  }, [fetchFirst]);

  return {
    contacts,
    nextCursor,
    loading,
    error,
    refetch: fetchFirst,
    loadMore,
  };
}
