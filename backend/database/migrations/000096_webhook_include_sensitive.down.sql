-- Rollback of 000096. Dropping the column loses only the explicit per-webhook
-- choice; behaviour reverts to the server default (full fidelity in v1.x).
ALTER TABLE webhooks DROP COLUMN include_sensitive;
