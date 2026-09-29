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

export function useWebAuthn() {
  const [passkeys, setPasskeys] = useState<Passkey[]>([]);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    try {
      setPasskeys(await listPasskeys());
    } catch {
      // Non-critical on the settings page; keep whatever we have.
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  // Errors propagate to the caller: the component owns the message wording
  // (cancelled ceremony vs backend failure).
  const register = useCallback(
    async (name?: string, proof?: SecondFactorProof): Promise<PasskeyEnrollment> => {
      const result = await registerPasskey(name, proof);
      await refresh();
      return result;
    },
    [refresh],
  );

  const remove = useCallback(
    async (id: string, proof: PasskeyRemovalProof) => {
      await removePasskey(id, proof);
      await refresh();
    },
    [refresh],
  );

  return { passkeys, loading, register, remove, refresh };
}
