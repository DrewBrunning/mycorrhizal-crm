-- Reverses 000071. Destructive by nature: the per-user GeoPulse connection
-- (base URL + encrypted API token) is discarded. Activities already confirmed
-- from GeoPulse stays survive — they are ordinary Activity rows carrying only
-- an opaque `geopulse:stay:<id>` external_ref.

DROP TABLE geopulse_configs;
