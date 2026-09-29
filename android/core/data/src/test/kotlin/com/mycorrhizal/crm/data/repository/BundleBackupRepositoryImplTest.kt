package com.mycorrhizal.crm.data.repository

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.domain.profile.ServerProfile
import com.mycorrhizal.crm.domain.profile.ServerProfileKind
import io.mockk.coEvery
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first
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
import java.util.UUID

/**
 * Issue #1264: the per-profile last-export / dismissal bookkeeping. The DataStore
 * file is a process-wide singleton, so each test uses fresh profile IDs rather
 * than assuming an empty store.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
class BundleBackupRepositoryImplTest {

    private val active = MutableStateFlow<ServerProfile?>(null)
    private val session = mockk<SessionManager> {
        every { observeActiveProfile() } returns active
        coEvery { activeProfileId() } answers { active.value?.id }
    }
    private lateinit var repository: BundleBackupRepositoryImpl

    @Before
    fun setup() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        repository = BundleBackupRepositoryImpl(context, session)
    }

    private fun profile(kind: ServerProfileKind) =
        ServerProfile(id = UUID.randomUUID().toString(), kind = kind, label = "p")

    private fun local() = profile(ServerProfileKind.Local)
    private fun remote() = profile(ServerProfileKind.Remote("https://x.example"))

    @Test
    fun `with no active profile the status is empty and not local`() = runTest {
        active.value = null

        val status = repository.observeStatus().first()

        assertFalse(status.isLocalProfile)
        assertNull(status.lastExportAt)
        assertNull(status.dismissedAt)
    }

    @Test
    fun `a fresh local profile has never been exported`() = runTest {
        active.value = local()

        val status = repository.observeStatus().first()

        assertTrue(status.isLocalProfile)
        assertNull(status.lastExportAt)
    }

    @Test
    fun `a remote profile is reported as not local`() = runTest {
        active.value = remote()

        assertFalse(repository.observeStatus().first().isLocalProfile)
    }

    @Test
    fun `recordExport is stored against the active profile`() = runTest {
        active.value = local()

        repository.recordExport(1_234L)

        assertEquals(1_234L, repository.observeStatus().first().lastExportAt)
    }

    @Test
    fun `timestamps are isolated per profile`() = runTest {
        val a = local()
        val b = remote()
        active.value = a
        repository.recordExport(111L)
        repository.dismissReminder(222L)

        active.value = b
        val other = repository.observeStatus().first()
        assertNull(other.lastExportAt)
        assertNull(other.dismissedAt)

        active.value = a
        val back = repository.observeStatus().first()
        assertEquals(111L, back.lastExportAt)
        assertEquals(222L, back.dismissedAt)
    }

    @Test
    fun `dismissReminder stores the dismissal time`() = runTest {
        active.value = local()

        repository.dismissReminder(999L)

        assertEquals(999L, repository.observeStatus().first().dismissedAt)
    }

    @Test
    fun `a fresh export clears an earlier snooze`() = runTest {
        active.value = local()
        repository.dismissReminder(100L)

        repository.recordExport(200L)

        val status = repository.observeStatus().first()
        assertEquals(200L, status.lastExportAt)
        assertNull(status.dismissedAt)
    }

    @Test
    fun `forget removes a profile's bookkeeping`() = runTest {
        val p = local()
        active.value = p
        repository.recordExport(5L)
        repository.dismissReminder(6L)

        repository.forget(p.id)

        val status = repository.observeStatus().first()
        assertNull(status.lastExportAt)
        assertNull(status.dismissedAt)
    }

    @Test
    fun `writes with no active profile are ignored`() = runTest {
        active.value = null
        repository.recordExport(1L)
        repository.dismissReminder(2L)

        val p = local()
        active.value = p
        val status = repository.observeStatus().first()
        assertNull(status.lastExportAt)
        assertNull(status.dismissedAt)
    }
}
