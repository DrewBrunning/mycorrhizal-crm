package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const androidPasskeyTestFP = "A3:65:E6:D6:28:35:03:7A:56:FD:DC:C8:88:80:F5:5F:EC:99:F4:FD:02:1B:A4:C0:AF:50:91:72:34:2E:6E:C0"

func TestLoadConfig_AndroidPasskeyVariables(t *testing.T) {
	t.Setenv("WEBAUTHN_ANDROID_ENABLED", "true")
	t.Setenv("WEBAUTHN_ANDROID_CERT_SHA256", androidPasskeyTestFP+", 000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	t.Setenv("FRONTEND_URL", "https://crm.example.com")
	cfg := LoadConfig()
	require.True(t, cfg.WebAuthnAndroidEnabled)
	require.Len(t, cfg.WebAuthnAndroidCertSHA256, 2)
	require.True(t, cfg.AndroidPasskeys().Effective)
}

func TestLoadConfig_AndroidPasskeyDefaultsOff(t *testing.T) {
	cfg := LoadConfig()
	require.False(t, cfg.WebAuthnAndroidEnabled)
	require.Empty(t, cfg.WebAuthnAndroidCertSHA256)
	require.False(t, cfg.AndroidPasskeys().Effective)
}

func TestCapabilities_WebAuthnAndroidOnlyWhenEffective(t *testing.T) {
	newCfg := func() *Config {
		c := baseConfig()
		c.FrontendURL = "https://crm.example.com"
		c.WebAuthnAndroidEnabled = true
		c.WebAuthnAndroidCertSHA256 = []string{androidPasskeyTestFP}
		return c
	}
	has := func(c *Config) bool {
		for _, tok := range c.Capabilities() {
			if tok == CapabilityWebAuthnAndroid {
				return true
			}
		}
		return false
	}

	require.True(t, has(newCfg()))

	c := newCfg()
	c.WebAuthnAndroidEnabled = false
	require.False(t, has(c), "switch off")

	c = newCfg()
	c.FrontendURL = "http://crm.example.com"
	require.False(t, has(c), "http FRONTEND_URL")

	c = newCfg()
	c.WebAuthnAndroidCertSHA256 = nil
	require.True(t, has(c), "the built-in obtainium fingerprint alone suffices")

	c = newCfg()
	c.Deployment = DeploymentEmbedded
	require.False(t, has(c), "embedded never advertises it")
	require.NotEmpty(t, c.AndroidPasskeys().Reason)

	// Stable position: right after two_factor.
	caps := newCfg().Capabilities()
	for i, tok := range caps {
		if tok == CapabilityTwoFactor {
			require.Equal(t, CapabilityWebAuthnAndroid, caps[i+1])
		}
	}
}
