package com.mycorrhizal.crm.feature.settings

import com.mycorrhizal.crm.data.local.LocalServerHost
import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.data.session.SwitchProfileResult
import com.mycorrhizal.crm.domain.profile.ServerProfile
import com.mycorrhizal.crm.domain.profile.ServerProfileKind
import com.mycorrhizal.crm.testing.MainDispatcherRule
import com.mycorrhizal.crm.ui.R
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test

class ServersViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val profiles = MutableStateFlow<List<ServerProfile>>(emptyList())
    private val active = MutableStateFlow<ServerProfile?>(null)

    private val session = mockk<SessionManager>(relaxed = true) {
        every { observeProfiles() } returns profiles
        every { observeActiveProfile() } returns active
    }

    private fun profile(id: String, label: String, url: String = "https://$id.example") =
        ServerProfile(id = id, kind = ServerProfileKind.Remote(url), label = label)

    @Test
    fun `profiles and the active id are surfaced`() = runTest(mainDispatcherRule.testDispatcher) {
        profiles.value = listOf(profile("p1", "One"), profile("p2", "Two"))
        active.value = profile("p2", "Two")
        val vm = ServersViewModel(session, mockk(relaxed = true))
        advanceUntilIdle()

        assertEquals(2, vm.uiState.value.profiles.size)
        assertEquals("p2", vm.uiState.value.activeProfileId)
    }

    @Test
    fun `select switches and emits Switched`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { session.switchProfile("p2", false) } returns SwitchProfileResult.Switched
        val vm = ServersViewModel(session, mockk(relaxed = true))

        vm.select("p2")
        advanceUntilIdle()

        assertEquals(ServersEvent.Switched, vm.events.first())
        assertNull(vm.uiState.value.pendingSwitch)
    }

    @Test
    fun `deleteLocalData stops the host, removes the profile, and emits Removed`() =
        runTest(mainDispatcherRule.testDispatcher) {
            profiles.value = listOf(
                ServerProfile(id = "loc", kind = ServerProfileKind.Local, label = "On this device"),
            )
            val host = mockk<LocalServerHost>(relaxed = true)
            val vm = ServersViewModel(session, host)

            vm.deleteLocalData("loc")
            advanceUntilIdle()

            coVerify { host.deleteLocalData() }
            coVerify { session.removeProfile("loc") }
            assertEquals(ServersEvent.Removed, vm.events.first())
        }

    @Test
    fun `select surfaces the confirmation without switching when the outbox is non-empty`() =
        runTest(mainDispatcherRule.testDispatcher) {
            profiles.value = listOf(profile("p1", "One"), profile("p2", "Two"))
            coEvery { session.switchProfile("p2", false) } returns SwitchProfileResult.NeedsConfirmation(4)
            val vm = ServersViewModel(session, mockk(relaxed = true))

            vm.select("p2")
            advanceUntilIdle()

            assertEquals(PendingSwitch("p2", 4), vm.uiState.value.pendingSwitch)
            coVerify(exactly = 0) { session.switchProfile("p2", true) }
        }

    @Test
    fun `confirmDiscard switches with discardPending true`() = runTest(mainDispatcherRule.testDispatcher) {
        profiles.value = listOf(profile("p1", "One"), profile("p2", "Two"))
        coEvery { session.switchProfile("p2", false) } returns SwitchProfileResult.NeedsConfirmation(4)
        coEvery { session.switchProfile("p2", true) } returns SwitchProfileResult.Switched
        val vm = ServersViewModel(session, mockk(relaxed = true))

        vm.select("p2")
        advanceUntilIdle()
        vm.confirmDiscard()
        advanceUntilIdle()

        coVerify { session.switchProfile("p2", true) }
        assertNull(vm.uiState.value.pendingSwitch)
    }

    @Test
    fun `dismissPendingSwitch clears the dialog without switching`() = runTest(mainDispatcherRule.testDispatcher) {
        profiles.value = listOf(profile("p1", "One"), profile("p2", "Two"))
        coEvery { session.switchProfile("p2", false) } returns SwitchProfileResult.NeedsConfirmation(4)
        val vm = ServersViewModel(session, mockk(relaxed = true))

        vm.select("p2")
        advanceUntilIdle()
        vm.dismissPendingSwitch()

        assertNull(vm.uiState.value.pendingSwitch)
        coVerify(exactly = 0) { session.switchProfile("p2", true) }
    }

    @Test
    fun `addRemote rejects an invalid url`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = ServersViewModel(session, mockk(relaxed = true))

        vm.addRemote("Bad", "not a url")
        advanceUntilIdle()

        assertEquals(R.string.servers_add_invalid_url, vm.uiState.value.errorRes)
        coVerify(exactly = 0) { session.addRemoteProfile(any(), any()) }
    }

    @Test
    fun `addRemote creates the profile and activates it`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { session.addRemoteProfile("Work", "https://work.example.com") } returns
            profile("p9", "Work", "https://work.example.com")
        coEvery { session.switchProfile("p9", false) } returns SwitchProfileResult.Switched
        val vm = ServersViewModel(session, mockk(relaxed = true))

        vm.addRemote("Work", "https://work.example.com/")
        advanceUntilIdle()

        coVerify { session.addRemoteProfile("Work", "https://work.example.com") }
        coVerify { session.switchProfile("p9", false) }
        assertEquals(ServersEvent.Switched, vm.events.first())
    }

    @Test
    fun `rename delegates the trimmed label`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = ServersViewModel(session, mockk(relaxed = true))

        vm.rename("p1", "  Home  ")
        advanceUntilIdle()

        coVerify { session.renameProfile("p1", "Home") }
    }

    @Test
    fun `remove delegates and emits Removed`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = ServersViewModel(session, mockk(relaxed = true))

        vm.remove("p1")
        advanceUntilIdle()

        coVerify { session.removeProfile("p1") }
        assertEquals(ServersEvent.Removed, vm.events.first())
    }
}
