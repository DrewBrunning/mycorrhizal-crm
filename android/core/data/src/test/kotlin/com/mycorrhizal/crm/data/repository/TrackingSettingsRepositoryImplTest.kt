package com.mycorrhizal.crm.data.repository

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
class TrackingSettingsRepositoryImplTest {

    private lateinit var repository: TrackingSettingsRepositoryImpl

    @Before
    fun setup() = kotlinx.coroutines.runBlocking {
        val context = ApplicationProvider.getApplicationContext<Context>()
        repository = TrackingSettingsRepositoryImpl(context)
        // DataStore's `by preferencesDataStore(...)` delegate keeps its own in-memory cache once
        // warm, and Robolectric reuses that warm instance across test methods in the same class --
        // deleting the backing file directly does NOT invalidate it (DataStore doesn't watch the
        // filesystem for external changes). Reset must go through the real write API instead, so
        // it updates the same in-memory state subsequent reads see. There's no single clear-all on
        // the interface (by design -- see TrackingSettingsRepository), so reset each key.
        repository.setCallTrackingEnabled(false)
        repository.setSmsTrackingEnabled(false)
        repository.setNotificationsEnabled(true)
        repository.setIncludeUnknownNumbers(false)
        repository.setLastCallLogTimestamp(0L)
        repository.setLastSmsTimestamp(0L)
    }

    @Test
    fun `callTrackingEnabled defaults to false`() = runTest {
        assertFalse(repository.callTrackingEnabled())
    }

    @Test
    fun `setCallTrackingEnabled persists the value`() = runTest {
        repository.setCallTrackingEnabled(true)

        assertTrue(repository.callTrackingEnabled())
    }

    @Test
    fun `smsTrackingEnabled defaults to false`() = runTest {
        assertFalse(repository.smsTrackingEnabled())
    }

    @Test
    fun `setSmsTrackingEnabled persists the value`() = runTest {
        repository.setSmsTrackingEnabled(true)

        assertTrue(repository.smsTrackingEnabled())
    }

    @Test
    fun `notificationsEnabled defaults to true`() = runTest {
        assertTrue(repository.notificationsEnabled())
    }

    @Test
    fun `setNotificationsEnabled persists false`() = runTest {
        repository.setNotificationsEnabled(false)

        assertFalse(repository.notificationsEnabled())
    }

    @Test
    fun `lastCallLogTimestamp defaults to zero`() = runTest {
        assertEquals(0L, repository.lastCallLogTimestamp())
    }

    @Test
    fun `setLastCallLogTimestamp persists the value`() = runTest {
        repository.setLastCallLogTimestamp(123_456L)

        assertEquals(123_456L, repository.lastCallLogTimestamp())
    }

    @Test
    fun `lastSmsTimestamp defaults to zero`() = runTest {
        assertEquals(0L, repository.lastSmsTimestamp())
    }

    @Test
    fun `setLastSmsTimestamp persists the value`() = runTest {
        repository.setLastSmsTimestamp(654_321L)

        assertEquals(654_321L, repository.lastSmsTimestamp())
    }

    @Test
    fun `lastInteractionSyncAt is null when never synced`() = runTest {
        assertNull(repository.lastInteractionSyncAt())
    }

    // ADR 0019 / issue #1127: the SMS Inbox reconciliation cursor. Combined
    // into one test (rather than a separate "defaults to null" test) so the
    // null-default assertion can't be perturbed by another test method's
    // write to this same key running first — mirrors why
    // `lastInteractionSyncAt is null when never synced` above is safe: no
    // other test in this class touches this key.
    @Test
    fun `lastSmsInboxId is null until set, then persists`() = runTest {
        assertNull(repository.lastSmsInboxId())

        repository.setLastSmsInboxId(42L)

        assertEquals(42L, repository.lastSmsInboxId())
    }

    // --- Issue #1029: capture-policy escape hatch + dropped-count ----------

    @Test
    fun `includeUnknownNumbers defaults to false`() = runTest {
        assertFalse(repository.includeUnknownNumbers())
    }

    @Test
    fun `setIncludeUnknownNumbers persists the value`() = runTest {
        repository.setIncludeUnknownNumbers(true)

        assertTrue(repository.includeUnknownNumbers())
    }

    @Test
    fun `incrementFilteredUnknownCount advances the counter`() = runTest {
        // The counter has no reset API (by design — it is a running local
        // diagnostic), and the DataStore stays warm across test methods, so
        // assert the delta rather than an absolute value.
        val before = repository.filteredUnknownCount()

        repository.incrementFilteredUnknownCount()
        repository.incrementFilteredUnknownCount()

        assertEquals(before + 2, repository.filteredUnknownCount())
    }
}
