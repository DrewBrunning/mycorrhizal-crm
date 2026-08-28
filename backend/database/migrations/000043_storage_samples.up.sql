-- Storage-growth history for the admin system-status endpoint (issue #652).
--
-- storage_samples is the small time-series behind the storage *trend* block on
-- /admin/system-status: one row per daily sampling of the on-disk footprint
-- (database + siblings, filesystem used/total, per-directory totals). With
-- ~180 days retained, it answers "how fast is my data growing / when does the
-- disk run out" — questions the point-in-time storage block (issue #388) cannot.
--
-- Hard-state operational bookkeeping, no deleted_at (CLAUDE.md backend trap
-- #7): rows are removed only by the sampler's own retention prune
-- (STORAGE_SAMPLE_RETENTION_DAYS, default 180), mirroring the job_runs /
-- system_events lifecycle. Not user-scoped, no user_id — instance-wide
-- operational history read only by the admin surface.
--
-- No high-cardinality or sensitive fields: timestamps and byte counts only.
-- fs_used_bytes / fs_total_bytes describe the filesystem holding the database
-- (used = total - free), so the projection to capacity exhaustion can be
-- computed from the persisted series without re-statfs'ing at read time.
CREATE TABLE storage_samples (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    taken_at            DATETIME NOT NULL,
    database_bytes      INTEGER NOT NULL,
    fs_used_bytes       INTEGER NOT NULL,
    fs_total_bytes      INTEGER NOT NULL,
    photo_dir_bytes     INTEGER NOT NULL DEFAULT 0,
    attachment_dir_bytes INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_storage_samples_taken_at ON storage_samples(taken_at);
