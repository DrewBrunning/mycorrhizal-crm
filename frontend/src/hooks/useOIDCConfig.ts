import { useEffect, useState } from 'react';
import { API_BASE_URL } from '../auth';

interface OIDCConfig {
  enabled: boolean;
  provider_name: string;
  // Public auth-config flag: DISABLE_REGISTRATION on the server. Fetched
  // from the same public /auth/oidc/config endpoint (it's the one
  // unauthenticated "what can a client do here" surface) so LoginPage and
  // RegisterPage can hide/block self-registration in the UI instead of only
  // finding out from a 403 after a full form submit.
  registration_disabled: boolean;
}

export function useOIDCConfig(): OIDCConfig {
  const [config, setConfig] = useState<OIDCConfig>({
    enabled: false,
    provider_name: 'SSO',
    // Fail open: if the fetch fails, the register form/link behaves exactly
    // as it did before this flag existed — the server's own 403 is still
    // the real enforcement, this is only ever a head start on the UI.
    registration_disabled: false,
  });

  useEffect(() => {
    fetch(`${API_BASE_URL}/auth/oidc/config`)
      .then((res) => (res.ok ? res.json() : null))
      .then((data) => {
        if (data) {
          setConfig({
            enabled: data.enabled === true,
            provider_name: data.provider_name || 'SSO',
            registration_disabled: data.registration_disabled === true,
          });
        }
      })
      .catch(() => {
        // Best-effort; see the registration_disabled default above.
      });
  }, []);

  return config;
}
