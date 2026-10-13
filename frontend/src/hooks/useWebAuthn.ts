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
  // Stryker disable next-line BooleanLiteral: the mount effect's refresh settles loading before any consumer can act on the first value
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(
    async () => {
      try {
        setPasskeys(await listPasskeys());
      } catch {
        // Non-critical on the settings page; keep whatever we have.
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
