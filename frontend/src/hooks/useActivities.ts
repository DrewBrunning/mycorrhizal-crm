// Custom hook for fetching and managing activities
import { useCallback, useEffect, useRef, useState } from 'react';
import {
  type Activity,
  type GetActivitiesParams,
  getActivities,
  getContactActivities,
} from '../api/activities';
import { isAuthenticated } from '../auth';
import { handleFetchError } from '../utils/errorHandler';

interface UseActivitiesResult {
  activities: Activity[];
  // Opaque resume token (T17): non-empty while more rows exist.
  nextCursor: string;
  limit: number;
  loading: boolean;
  error: string | null;
  refetch: () => Promise<void>;
  loadMore: () => Promise<void>;
}

export function useActivities(
  params: GetActivitiesParams = {},
  contactId?: string | number,
): UseActivitiesResult {
  // Destructure params to use primitive values as dependencies
  // This prevents re-fetches when callers pass new object references with identical values
  const { cursor: _ignored, limit: paramLimit, includeContacts, search, fromDate, toDate } = params;

  const [activities, setActivities] = useState<Activity[]>([]);
  const [nextCursor, setNextCursor] = useState('');
  const [limit, setLimit] = useState(paramLimit || 25);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const requestRef = useRef(0);

  const fetchFirst = useCallback(async () => {
    const requestId = requestRef.current + 1;
    requestRef.current = requestId;
    setLoading(true);
    setError(null);

    try {
      if (!isAuthenticated()) {
        throw new Error('No authentication token found');
      }

      if (contactId) {
        const data = await getContactActivities(contactId);
        if (requestRef.current !== requestId) {
          return;
        }
        setActivities(data.activities || []);
        setNextCursor('');
        setLimit(paramLimit || data.activities?.length || 25);
      } else {
        const data = await getActivities({
          limit: paramLimit,
          includeContacts,
          search,
          fromDate,
          toDate,
        });
        if (requestRef.current !== requestId) {
          return;
        }
        setActivities(data.activities || []);
        setNextCursor(data.next_cursor || '');
        setLimit(data.limit || paramLimit || 25);
      }
    } catch (err) {
      if (requestRef.current !== requestId) {
        return;
      }
      const message = handleFetchError(err, 'fetching activities');
      setError(message);
    } finally {
      if (requestRef.current === requestId) {
        setLoading(false);
      }
    }
  }, [contactId, paramLimit, includeContacts, search, fromDate, toDate]);

  const loadMore = useCallback(async () => {
    if (!nextCursor) return;
    const requestId = requestRef.current + 1;
    requestRef.current = requestId;
    setLoading(true);
    setError(null);
    try {
      const data = await getActivities({
        cursor: nextCursor,
        limit: paramLimit,
        includeContacts,
        search,
        fromDate,
        toDate,
      });
      if (requestRef.current !== requestId) {
        return;
      }
      setActivities((prev) => [...prev, ...(data.activities || [])]);
      setNextCursor(data.next_cursor || '');
    } catch (err) {
      if (requestRef.current !== requestId) {
        return;
      }
      setError(handleFetchError(err, 'loading more activities'));
    } finally {
      if (requestRef.current === requestId) {
        setLoading(false);
      }
    }
  }, [nextCursor, paramLimit, includeContacts, search, fromDate, toDate]);

  useEffect(() => {
    fetchFirst();
  }, [fetchFirst]);

  return {
    activities,
    nextCursor,
    limit,
    loading,
    error,
    refetch: fetchFirst,
    loadMore,
  };
}
