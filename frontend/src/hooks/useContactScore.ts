import { useCallback, useEffect, useState } from 'react';
import { type ContactScoreResponse, getContactScore } from '../api/contactScore';
import { handleFetchError } from '../utils/errorHandler';

// Fetches the relationship health score for a single contact (issue #383,
// ADR-0023), auto-refreshing whenever contactId changes -- same
// self-fetching shape as useCadencePolicy.ts.
export function useContactScore(contactId: number | undefined) {
  const [score, setScore] = useState<ContactScoreResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refreshScore = useCallback(async () => {
    if (contactId == null) return;
    setLoading(true);
    setError(null);
    try {
      setScore(await getContactScore(contactId));
    } catch (err) {
      setError(handleFetchError(err, 'fetching relationship health score'));
    } finally {
      setLoading(false);
    }
  }, [contactId]);

  useEffect(() => {
    refreshScore();
  }, [refreshScore]);

  return { score, loading, error, refreshScore };
}
