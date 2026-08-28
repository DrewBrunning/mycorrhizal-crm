-- Storage-growth history (issue #652) — rollback. storage_samples holds only
-- timestamps and byte counts for the daily storage sampler; dropping it loses
-- that operational trend history and nothing else (no contact data, no PII).
DROP TABLE storage_samples;
