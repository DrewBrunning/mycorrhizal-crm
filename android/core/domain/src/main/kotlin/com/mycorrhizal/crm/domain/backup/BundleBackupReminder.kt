package com.mycorrhizal.crm.domain.backup

import java.util.concurrent.TimeUnit

/**
 * Issue #1264 / ADR 0028 Decision 5: a Local profile holds the only copy of the
 * user's data, so the dashboard nags (dismissibly) when the account bundle has
 * never been exported or the last export is stale. The rule lives here, as a
 * pure function of timestamps, so it is unit-testable without a clock or UI.
 */
object BundleBackupReminder {
    /** An export older than this is stale and re-triggers the banner. */
    val STALE_AFTER_MILLIS: Long = TimeUnit.DAYS.toMillis(30)

    /** Dismissing the banner hides it for this long before it may reappear. */
    val SNOOZE_MILLIS: Long = TimeUnit.DAYS.toMillis(7)

    /**
     * Whether to show the banner. Only a [BundleBackupStatus.isLocalProfile]
     * ever does (a Remote profile's data lives on the server). It is due when
     * [BundleBackupStatus.lastExportAt] is null or more than 30 days before
     * [nowMillis], and is suppressed while a dismissal is still inside the
     * snooze window. A dismissal timestamp in the
     * future (clock skew) is not treated as a snooze.
     */
    fun shouldShow(status: BundleBackupStatus, nowMillis: Long): Boolean {
        if (!status.isLocalProfile) return false
        val last = status.lastExportAt
        val due = last == null || nowMillis - last > STALE_AFTER_MILLIS
        if (!due) return false
        val dismissed = status.dismissedAt
        val snoozed = dismissed != null && nowMillis - dismissed in 0L until SNOOZE_MILLIS
        return !snoozed
    }
}

/**
 * The per-profile backup bookkeeping for the *active* profile. Epoch-millis
 * timestamps; null means "never".
 */
data class BundleBackupStatus(
    val isLocalProfile: Boolean = false,
    val lastExportAt: Long? = null,
    val dismissedAt: Long? = null,
)
