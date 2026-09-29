package com.mycorrhizal.crm.domain.backup

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** Issue #1264: the 30-day banner rule, as a pure function of timestamps. */
class BundleBackupReminderTest {

    private val day = 24L * 60 * 60 * 1000
    private val now = 1_800_000_000_000L

    private fun local(lastExportAt: Long? = null, dismissedAt: Long? = null) =
        BundleBackupStatus(isLocalProfile = true, lastExportAt = lastExportAt, dismissedAt = dismissedAt)

    @Test
    fun `thresholds are 30 days stale and a 7 day snooze`() {
        assertEquals(30 * day, BundleBackupReminder.STALE_AFTER_MILLIS)
        assertEquals(7 * day, BundleBackupReminder.SNOOZE_MILLIS)
    }

    @Test
    fun `never exported shows the banner`() {
        assertTrue(BundleBackupReminder.shouldShow(local(), now))
    }

    @Test
    fun `an export today hides the banner`() {
        assertFalse(BundleBackupReminder.shouldShow(local(lastExportAt = now), now))
    }

    @Test
    fun `an export 29 days ago hides the banner`() {
        assertFalse(BundleBackupReminder.shouldShow(local(lastExportAt = now - 29 * day), now))
    }

    @Test
    fun `an export exactly 30 days ago is not yet stale`() {
        assertFalse(BundleBackupReminder.shouldShow(local(lastExportAt = now - 30 * day), now))
    }

    @Test
    fun `an export one millisecond past 30 days is stale`() {
        assertTrue(BundleBackupReminder.shouldShow(local(lastExportAt = now - 30 * day - 1), now))
    }

    @Test
    fun `a stale export shows the banner`() {
        assertTrue(BundleBackupReminder.shouldShow(local(lastExportAt = now - 45 * day), now))
    }

    @Test
    fun `a remote profile never shows the banner`() {
        assertFalse(BundleBackupReminder.shouldShow(BundleBackupStatus(isLocalProfile = false), now))
        assertFalse(
            BundleBackupReminder.shouldShow(
                BundleBackupStatus(isLocalProfile = false, lastExportAt = now - 90 * day),
                now,
            ),
        )
    }

    @Test
    fun `a dismissal within the snooze window hides a due banner`() {
        assertFalse(BundleBackupReminder.shouldShow(local(dismissedAt = now - 3 * day), now))
        assertFalse(BundleBackupReminder.shouldShow(local(lastExportAt = now - 60 * day, dismissedAt = now - day), now))
    }

    @Test
    fun `a dismissal exactly at the snooze boundary has expired`() {
        assertTrue(BundleBackupReminder.shouldShow(local(dismissedAt = now - 7 * day), now))
    }

    @Test
    fun `an old dismissal no longer hides the banner`() {
        assertTrue(BundleBackupReminder.shouldShow(local(dismissedAt = now - 8 * day), now))
    }

    @Test
    fun `a dismissal from the future is not a snooze`() {
        assertTrue(BundleBackupReminder.shouldShow(local(dismissedAt = now + day), now))
    }
}
