import { useCallback, useEffect, useState } from 'react';
import {
  type CadencePolicy,
  type CadencePolicyInput,
  createCadencePolicy,
  deleteCadencePolicy,
  getCadencePolicies,
  updateCadencePolicy,
} from '../api/cadencePolicies';
import { type ErrorNotifier, handleError, handleFetchError } from '../utils/errorHandler';

export function useCadencePolicy(entityId: string | undefined, notifier?: ErrorNotifier) {
  const [policy, setPolicy] = useState<CadencePolicy | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(
    async (overrideEntityId?: string) => {
      const uid = overrideEntityId ?? entityId;
      if (!uid) return;
      setLoading(true);
      setError(null);
      try {
        const response = await getCadencePolicies(uid);
        const found = (response.cadence_policies || [])[0] ?? null;
        setPolicy(found);
      } catch (err) {
        setError(handleFetchError(err, 'loading cadence policy'));
      } finally {
        setLoading(false);
      }
    },
    [entityId],
  );

  useEffect(() => {
    refresh();
  }, [refresh]);

  const handleSave = useCallback(
    async (input: CadencePolicyInput) => {
      if (policy && policy.entity_id === input.entity_id) {
        await updateCadencePolicy(policy.id, input);
      } else {
        await createCadencePolicy(input);
      }
      await refresh(input.entity_id);
    },
    [policy, refresh],
  );

  const handleDelete = useCallback(async () => {
    if (!policy) return;
    try {
      await deleteCadencePolicy(policy.id);
      setPolicy(null);
    } catch (err) {
      handleError(err, { operation: 'deleting cadence policy' }, notifier);
      throw err;
    }
  }, [policy, notifier]);

  return {
    policy,
    loading,
    error,
    refresh,
    handleSave,
    handleDelete,
  };
}
