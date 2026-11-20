import { useCallback, useEffect, useRef, useState } from 'react';
import { type ContactScoreResponse, getContactScore } from '../api/contactScore';
import { handleFetchError } from '../utils/errorHandler';

// Fetches the relationship health score for a single contact (issue #383,
// ADR-0023), auto-refreshing whenever contactId changes -- same
// self-fetching shape as useCadencePolicy.ts.
export function useContactScore(contactId: number | undefined, refreshKey?: unknown) {
  const [score, setScore] = useState<ContactScoreResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // A contact change starts a new request while the previous one may still be
  // in flight. Only the newest response may write state, or contact A's score
  // can land on contact B.
  const requestRef = useRef(0);

  const refreshScore = useCallback(async () => {
    if (contactId == null) {
      // Invalidate any in-flight request when the contact disappears.
      requestRef.current += 1;
      return;
    }
    const requestId = ++requestRef.current;
    setLoading(true);
    setError(null);
    try {
      const next = await getContactScore(contactId);
      if (requestRef.current !== requestId) return;
      setScore(next);
    } catch (err) {
      if (requestRef.current !== requestId) return;
      setError(handleFetchError(err, 'fetching relationship health score'));
    } finally {
      if (requestRef.current === requestId) setLoading(false);
    }
  }, [contactId]);

  // A different contact is a clean slate: don't keep rendering the previous
  // contact's score while the new request is in flight.
  useEffect(() => {
    setScore(null);
    setError(null);
  }, [contactId]);

  useEffect(() => {
    void refreshScore();
    // refreshKey: a caller-supplied value (e.g. crm.ambient) whose change must refetch.
  }, [refreshScore, refreshKey]);

  return { score, loading, error, refreshScore };
}
