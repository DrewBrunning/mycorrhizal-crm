package com.mycorrhizal.crm.domain.repository

/** A device-detected interaction staged for optional server sync (§6.1/6.2). */
data class PendingInteraction(
    val id: Long = 0,
    val timestampMillis: Long,
    val kind: String,
    val direction: String? = null,
    val phoneNumber: String? = null,
    val matchedContactId: Int? = null,
    val synced: Boolean = false,
    val syncedAt: String? = null,
)

/**
 * Staging of device-detected interactions (Phase 4 §6.1/6.2). Call/SMS
 * tracking writes a PendingInteraction locally instead of hitting the server
 * directly; the sync worker converts unsynced rows into server Activities.
 */
interface PendingInteractionRepository {
    suspend fun record(interaction: PendingInteraction)
    suspend fun unsynced(): List<PendingInteraction>
    suspend fun markSynced(id: Long, syncedAt: String)
    suspend fun deleteSynced()
}

/** User prefs controlling which device features are enabled (§8.3 opt-in toggles). */
interface TrackingSettingsRepository {
    suspend fun callTrackingEnabled(): Boolean
    suspend fun setCallTrackingEnabled(enabled: Boolean)
    suspend fun smsTrackingEnabled(): Boolean
    suspend fun setSmsTrackingEnabled(enabled: Boolean)
    suspend fun notificationsEnabled(): Boolean
    suspend fun setNotificationsEnabled(enabled: Boolean)
    /** Last timestamp processed by the call-log reader (dedupe). */
    suspend fun lastCallLogTimestamp(): Long
    suspend fun setLastCallLogTimestamp(ts: Long)
    /** When the periodic interaction sync last ran. */
    suspend fun lastInteractionSyncAt(): Long?
}
