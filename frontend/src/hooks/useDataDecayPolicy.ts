import { useCallback, useEffect, useState } from 'react';
import {
  createDataDecayPolicy,
  type DataDecayPolicy,
  type DataDecayPolicyInput,
  deleteDataDecayPolicy,
  getDataDecayPolicies,
  updateDataDecayPolicy,
  verifyDataDecayPolicy,
} from '../api/dataDecayPolicies';
import { type ErrorNotifier, handleError, handleFetchError } from '../utils/errorHandler';

export function useDataDecayPolicy(entityId: string | undefined, notifier?: ErrorNotifier) {
  const [policy, setPolicy] = useState<DataDecayPolicy | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(
    async (overrideEntityId?: string) => {
      const uid = overrideEntityId ?? entityId;
      if (!uid) return;
      setLoading(true);
      setError(null);
      try {
        const response = await getDataDecayPolicies(uid);
        const found = (response.data_decay_policies || [])[0] ?? null;
        setPolicy(found);
      } catch (err) {
        setError(handleFetchError(err, 'loading data decay policy'));
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
    async (input: DataDecayPolicyInput) => {
      if (policy && policy.entity_id === input.entity_id) {
        await updateDataDecayPolicy(policy.id, input);
      } else {
        await createDataDecayPolicy(input);
      }
      await refresh(input.entity_id);
    },
    [policy, refresh],
  );

  const handleDelete = useCallback(async () => {
    if (!policy) return;
    try {
      await deleteDataDecayPolicy(policy.id);
      setPolicy(null);
    } catch (err) {
      handleError(err, { operation: 'deleting data decay policy' }, notifier);
      throw err;
    }
  }, [policy, notifier]);

  const handleVerify = useCallback(async () => {
    if (!policy) return;
    try {
      const updated = await verifyDataDecayPolicy(policy.id);
      setPolicy(updated);
    } catch (err) {
      handleError(err, { operation: 'confirming contact info is current' }, notifier);
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
    handleVerify,
  };
}
