import { useCallback, useEffect, useState } from 'react';
import {
  createOccasionObligation,
  deleteOccasionObligation,
  getOccasionObligations,
  type OccasionObligation,
  type OccasionObligationInput,
  updateOccasionObligation,
} from '../api/occasionObligations';
import { type ErrorNotifier, handleError, handleFetchError } from '../utils/errorHandler';

// useOccasionObligations loads and manages the standing card/gift/invite
// obligations for one contact (ADR 0024, issue #387), keyed by the contact's
// VCardUID (entity_id). Mirrors usePreferences' exact shape.
export function useOccasionObligations(entityId: string | undefined, notifier?: ErrorNotifier) {
  const [obligations, setObligations] = useState<OccasionObligation[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    if (!entityId) return;
    setLoading(true);
    setError(null);
    try {
      const response = await getOccasionObligations({ entityId, limit: 100 });
      setObligations(response.occasion_obligations || []);
    } catch (err) {
      setError(handleFetchError(err, 'fetching occasion obligations'));
    } finally {
      setLoading(false);
    }
  }, [entityId]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const handleSave = useCallback(
    async (obligation: OccasionObligation | null, input: OccasionObligationInput) => {
      try {
        if (obligation) {
          await updateOccasionObligation(obligation.id, input);
        } else {
          await createOccasionObligation(input);
        }
        await refresh();
      } catch (err) {
        handleError(err, { operation: 'saving occasion obligation' }, notifier);
        throw err;
      }
    },
    [refresh, notifier],
  );

  const handleDelete = useCallback(
    async (id: string) => {
      try {
        await deleteOccasionObligation(id);
        await refresh();
      } catch (err) {
        handleError(err, { operation: 'deleting occasion obligation' }, notifier);
        throw err;
      }
    },
    [refresh, notifier],
  );

  return { obligations, loading, error, refresh, handleSave, handleDelete };
}
