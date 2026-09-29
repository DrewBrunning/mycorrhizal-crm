package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.domain.backup.BundleBackupStatus
import kotlinx.coroutines.flow.Flow

/**
 * Issue #1264: per-profile record of when the account bundle was last exported
 * and when the backup reminder was last dismissed (ADR 0028 Decision 5 — stored
 * per profile in DataStore). Everything is scoped to the *active* profile so
 * callers never handle profile IDs. Timestamps are non-secret metadata.
 */
interface BundleBackupRepository {
    /** The active profile's backup status; re-emits on profile switch and on every write. */
    fun observeStatus(): Flow<BundleBackupStatus>

    /** Record a successful bundle export for the active profile at [atMillis]. */
    suspend fun recordExport(atMillis: Long)

    /** Snooze the reminder for the active profile as of [atMillis]. */
    suspend fun dismissReminder(atMillis: Long)

    /** Drop the bookkeeping for [profileId] (called when a profile is removed). */
    suspend fun forget(profileId: String)
}
