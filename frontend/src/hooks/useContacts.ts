// Custom hook for fetching and managing contacts
import {
  type Dispatch,
  type SetStateAction,
  useCallback,
  useEffect,
  useRef,
  useState,
} from 'react';
import { type Contact, type GetContactsParams, getContacts } from '../api/contacts';
import { isAuthenticated } from '../auth';
import { handleFetchError } from '../utils/errorHandler';

interface UseContactsResult {
  contacts: Contact[];
  // Opaque resume token (T17): non-empty while more rows exist. There is no
  // exact total — cursor pagination gives it up.
  nextCursor: string;
  // T103: present only while the contact-info filter is active — how many
  // contacts matched the other filters but were hidden by it, so the page can
  // disclose that rows are hidden rather than reading as lost data.
  hiddenCount?: number;
  loading: boolean;
  error: string | null;
  refetch: () => Promise<void>;
  loadMore: () => Promise<void>;
  // Raw state setter, exposed so a consumer can apply a local, optimistic
  // edit (e.g. ContactsPage's star toggle) without a full page-one refetch
  // that would bounce scroll position on paginated pages.
  setContacts: Dispatch<SetStateAction<Contact[]>>;
}

export function useContacts(params: GetContactsParams = {}): UseContactsResult {
  // Destructure params to use primitive values as dependencies
  // This prevents re-fetches when callers pass new object references with identical values
  const {
    cursor: _ignored,
    limit,
    search,
    circle,
    sort,
    order,
    includeArchived,
    archived,
    favorites,
    hasContactInfo,
  } = params;

  const [contacts, setContacts] = useState<Contact[]>([]);
  const [nextCursor, setNextCursor] = useState('');
  const [hiddenCount, setHiddenCount] = useState<number | undefined>(undefined);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Search-as-you-type request guard (issue #556): a keystroke changes
  // `search`, which re-creates fetchFirst and re-fetches. Without a sequence
  // check a slow response for an earlier query can land after a newer one and
  // overwrite the list with stale results. Every fetch captures an id at call
  // start and drops its state writes once a newer fetch has begun -- same
  // pattern as useGraph.ts.
  const requestRef = useRef(0);

  // fetchFirst replaces the list (page one); loadMore appends the next page.
  const fetchFirst = useCallback(async () => {
    const requestId = ++requestRef.current;
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
        favorites,
        hasContactInfo,
      });
      if (requestRef.current !== requestId) return;
      setContacts(data.contacts || []);
      setNextCursor(data.next_cursor || '');
      setHiddenCount(data.hidden_count);
    } catch (err) {
      if (requestRef.current !== requestId) return;
      const message = handleFetchError(err, 'fetching contacts');
      setError(message);
    } finally {
      if (requestRef.current === requestId) setLoading(false);
    }
  }, [limit, search, circle, sort, order, includeArchived, archived, favorites, hasContactInfo]);

  const loadMore = useCallback(async () => {
    if (!nextCursor) return;
    const requestId = ++requestRef.current;
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
        favorites,
        hasContactInfo,
      });
      if (requestRef.current !== requestId) return;
      setContacts((prev) => [...prev, ...(data.contacts || [])]);
      setNextCursor(data.next_cursor || '');
      setHiddenCount(data.hidden_count);
    } catch (err) {
      if (requestRef.current !== requestId) return;
      setError(handleFetchError(err, 'loading more contacts'));
    } finally {
      if (requestRef.current === requestId) setLoading(false);
    }
  }, [
    nextCursor,
    limit,
    search,
    circle,
    sort,
    order,
    includeArchived,
    archived,
    favorites,
    hasContactInfo,
  ]);

  useEffect(() => {
    fetchFirst();
  }, [fetchFirst]);

  return {
    contacts,
    nextCursor,
    hiddenCount,
    loading,
    error,
    refetch: fetchFirst,
    loadMore,
    setContacts,
  };
}
