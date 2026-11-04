-- Issue #2339: optional ntfy access token for self-hosted servers that
-- require authentication to publish. Stored encrypted via
-- services/credential_crypto.go, exactly like gotify_token_encrypted; empty
-- means "no token" (anonymous publish, the pre-existing behaviour). Additive:
-- existing configs are untouched.
ALTER TABLE notification_configs ADD COLUMN ntfy_token_encrypted TEXT NOT NULL DEFAULT '';
