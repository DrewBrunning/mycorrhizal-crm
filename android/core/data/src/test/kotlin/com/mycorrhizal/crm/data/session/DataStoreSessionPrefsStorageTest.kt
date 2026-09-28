package com.mycorrhizal.crm.data.session

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.domain.profile.ServerProfile
import com.mycorrhizal.crm.domain.profile.ServerProfileKind
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/**
 * First DataStore-under-Robolectric test in this repo (per issue #320's
 * Phase B plan) — DataStore is backed by ordinary Context.filesDir file I/O,
 * which Robolectric supports natively (same mechanism Room's in-memory DB
 * already relies on elsewhere in this test suite).
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
class DataStoreSessionPrefsStorageTest {

    private lateinit var storage: DataStoreSessionPrefsStorage

    @Before
    fun setup() = kotlinx.coroutines.runBlocking {
        val context = ApplicationProvider.getApplicationContext<Context>()
        storage = DataStoreSessionPrefsStorage(context)
        // The DataStore file lives under context.filesDir and Robolectric does not reset it
        // between test methods in the same class, so an earlier test's write leaks into the
        // next test's "before anything is saved" expectations without this.
        storage.clear()
    }

    @Test
    fun `loadServerUrl is null before anything is saved`() = runTest {
        assertNull(storage.loadServerUrl())
    }

    @Test
    fun `save then loadServerUrl round-trips`() = runTest {
        storage.save("https://crm.example.com")

        assertEquals("https://crm.example.com", storage.loadServerUrl())
    }

    @Test
    fun `saving null clears the stored url`() = runTest {
        storage.save("https://crm.example.com")

        storage.save(null)

        assertNull(storage.loadServerUrl())
    }

    @Test
    fun `clear removes the stored url`() = runTest {
        storage.save("https://crm.example.com")

        storage.clear()

        assertNull(storage.loadServerUrl())
    }

    // --- ADR 0028 Decision 1: the profile list + active id -------------------

    @Test
    fun `loadProfiles is empty before anything is saved`() = runTest {
        assertEquals(ProfilesSnapshot(), storage.loadProfiles())
    }

    @Test
    fun `saveProfiles then loadProfiles round-trips`() = runTest {
        val snapshot = ProfilesSnapshot(
            profiles = listOf(
                ServerProfile("p1", ServerProfileKind.Remote("https://one.example"), "One"),
                ServerProfile("p2", ServerProfileKind.Local, "On this device"),
            ),
            activeProfileId = "p2",
        )

        storage.saveProfiles(snapshot)

        assertEquals(snapshot, storage.loadProfiles())
    }

    @Test
    fun `saving an empty profile list clears them`() = runTest {
        storage.saveProfiles(
            ProfilesSnapshot(
                profiles = listOf(ServerProfile("p1", ServerProfileKind.Remote("https://one.example"), "One")),
                activeProfileId = "p1",
            ),
        )

        storage.saveProfiles(ProfilesSnapshot())

        assertEquals(ProfilesSnapshot(), storage.loadProfiles())
    }

    @Test
    fun `clear removes the profiles too`() = runTest {
        storage.saveProfiles(
            ProfilesSnapshot(
                profiles = listOf(ServerProfile("p1", ServerProfileKind.Remote("https://one.example"), "One")),
                activeProfileId = "p1",
            ),
        )

        storage.clear()

        assertEquals(ProfilesSnapshot(), storage.loadProfiles())
    }
}
