import { useState, useCallback } from 'react';
import { searchAll, SearchResult } from '../api/search';
import { handleFetchError } from '../utils/errorHandler';

// useSearch backs the global search page (T11): debounced full-text search
// across contacts, notes, and interactions.
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
