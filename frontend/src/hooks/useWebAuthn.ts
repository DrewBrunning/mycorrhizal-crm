import { useCallback, useEffect, useState } from 'react';
import {
  listPasskeys,
  type Passkey,
  type PasskeyEnrollment,
  type PasskeyRemovalProof,
  registerPasskey,
  removePasskey,
  type SecondFactorProof,
} from '../api/webauthn';
import { handleFetchError } from '../utils/errorHandler';

export function useWebAuthn() {
  const [passkeys, setPasskeys] = useState<Passkey[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(
    async () => {
      setError(null);
      try {
        setPasskeys(await listPasskeys());
      } catch (err) {
        // Surfaced, not swallowed (issue #1779): a failed load must not read as "no passkeys".
        setError(handleFetchError(err, 'fetching passkeys'));
      } finally {
        setLoading(false);
      }
    },
    // Stryker disable next-line ArrayDeclaration: refresh closes over state setters only (stable), so its deps array is equivalent either way
    [],
  );

  useEffect(
    () => {
      void refresh();
    },
    // Stryker disable next-line ArrayDeclaration: refresh is stable, so the mount effect runs exactly once under either deps array
    [refresh],
  );

  // Errors propagate to the caller: the component owns the message wording
  // (cancelled ceremony vs backend failure).
  const register = useCallback(
    async (name?: string, proof?: SecondFactorProof): Promise<PasskeyEnrollment> => {
      const result = await registerPasskey(name, proof);
      await refresh();
      return result;
    },
    // Stryker disable next-line ArrayDeclaration: refresh is stable, so the deps array is equivalent either way
    [refresh],
  );

  const remove = useCallback(
    async (id: string, proof: PasskeyRemovalProof) => {
      await removePasskey(id, proof);
      await refresh();
    },
    // Stryker disable next-line ArrayDeclaration: refresh is stable, so the deps array is equivalent either way
    [refresh],
  );

  return { passkeys, loading, error, register, remove, refresh };
}
