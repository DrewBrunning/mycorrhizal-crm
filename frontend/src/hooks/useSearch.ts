import { useCallback, useState } from 'react';
import { type SearchResult, searchAll } from '../api/search';
import { handleFetchError } from '../utils/errorHandler';

// useSearch backs the cross-entity (notes/activities) half of search. After
// T86 the standalone search page is gone; the Contacts page fires this in
// parallel with the contacts list so note/activity hits stay findable without
// a second contact list. There is no debounce here — the caller feeds it the
// already-debounced `?search=` term (the search field debounces input into the
// URL, and this effect keys off that committed value).
export function useSearch() {
  const [result, setResult] = useState<SearchResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const runSearch = useCallback(async (query: string) => {
    const trimmed = query.trim();
    if (trimmed.length < 2) {
      setResult(null);
      setError(null);
      return;
    }
    setLoading(true);
    setError(null);
    try {
      setResult(await searchAll(trimmed));
    } catch (err) {
      setError(handleFetchError(err, 'searching'));
      setResult(null);
    } finally {
      setLoading(false);
    }
  }, []);

  return {
    result,
    loading,
    error,
    runSearch,
  };
}
