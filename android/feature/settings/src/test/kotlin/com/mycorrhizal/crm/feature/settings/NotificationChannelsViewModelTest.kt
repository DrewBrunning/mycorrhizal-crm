package com.mycorrhizal.crm.feature.settings

import com.mycorrhizal.crm.domain.repository.NotificationSettingsRepository
import com.mycorrhizal.crm.model.network.NotificationConfig
import com.mycorrhizal.crm.model.network.NotificationTestResult
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class NotificationChannelsViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val repository = mockk<NotificationSettingsRepository>()

    private fun config(
        ntfyUrl: String = "",
        ntfyTopic: String = "",
        notifyNtfy: Boolean = false,
        gotifyUrl: String = "",
        gotifyHasToken: Boolean = false,
        notifyGotify: Boolean = false,
    ) = NotificationConfig(
        ntfyUrl = ntfyUrl,
        ntfyTopic = ntfyTopic,
        notifyNtfy = notifyNtfy,
        gotifyUrl = gotifyUrl,
        gotifyHasToken = gotifyHasToken,
        notifyGotify = notifyGotify,
    )

    @Test
    fun `load populates the config into the editable state`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.config() } returns Result.success(
            config(ntfyUrl = "https://ntfy.sh", ntfyTopic = "reminders", notifyNtfy = true, gotifyUrl = "http://gotify:8080", gotifyHasToken = true, notifyGotify = true),
        )
        val vm = NotificationChannelsViewModel(repository)
        advanceUntilIdle()

        val state = vm.uiState.value
        assertTrue(!state.isLoading)
        assertEquals("https://ntfy.sh", state.ntfyUrl)
        assertEquals("reminders", state.ntfyTopic)
        assertTrue(state.notifyNtfy)
        assertTrue(state.gotifyHasToken)
        assertTrue(state.notifyGotify)
    }

    @Test
    fun `load failure surfaces the load error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.config() } returns Result.failure(ApiError.Server(500, "boom"))
        val vm = NotificationChannelsViewModel(repository)
        advanceUntilIdle()

        assertEquals("Server error (500)", vm.uiState.value.loadError)
    }

    @Test
    fun `save round-trips the config and clears the token field semantics`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.config() } returns Result.success(config())
            coEvery { repository.save(any()) } returns Result.success(
                config(ntfyUrl = "https://ntfy.sh", ntfyTopic = "t", notifyNtfy = true),
            )
            val vm = NotificationChannelsViewModel(repository)
            advanceUntilIdle()

            vm.onNtfyUrlChange("https://ntfy.sh")
            vm.onNtfyTopicChange("t")
            vm.onNotifyNtfyChange(true)
            vm.save()
            advanceUntilIdle()

            coVerify(exactly = 1) { repository.save(any()) }
            val state = vm.uiState.value
            assertEquals("https://ntfy.sh", state.ntfyUrl)
            assertTrue(state.notifyNtfy)
            assertNull(state.saveError)
        }

    @Test
    fun `a typed gotify token is sent on save and cleared from state afterwards`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val captured = mutableListOf<com.mycorrhizal.crm.model.network.NotificationConfigInput>()
            coEvery { repository.config() } returns Result.success(config())
            coEvery { repository.save(any()) } answers {
                captured.add(firstArg())
                Result.success(config(gotifyUrl = "http://gotify:8080", gotifyHasToken = true, notifyGotify = true))
            }
            val vm = NotificationChannelsViewModel(repository)
            advanceUntilIdle()

            vm.onGotifyUrlChange("http://gotify:8080")
            vm.onGotifyTokenChange("tok-123")
            vm.onNotifyGotifyChange(true)
            vm.save()
            advanceUntilIdle()

            // The token reaches the server exactly once, then is dropped from state.
            assertEquals("tok-123", captured.last().gotifyToken)
            assertEquals("", vm.uiState.value.gotifyToken)
            assertTrue(vm.uiState.value.gotifyHasToken)
        }

    @Test
    fun `save failure surfaces the save error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.config() } returns Result.success(config())
        coEvery { repository.save(any()) } returns Result.failure(ApiError.Client(400, "Invalid URL"))
        val vm = NotificationChannelsViewModel(repository)
        advanceUntilIdle()

        vm.onNtfyUrlChange("not-a-url")
        vm.save()
        advanceUntilIdle()

        assertEquals("Invalid URL", vm.uiState.value.saveError)
    }

    @Test
    fun `test reports success distinctly`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.config() } returns Result.success(config())
        coEvery { repository.save(any()) } returns Result.success(config())
        coEvery { repository.test("ntfy") } returns Result.success(NotificationTestResult(ok = true))
        val vm = NotificationChannelsViewModel(repository)
        advanceUntilIdle()

        vm.onNtfyUrlChange("https://ntfy.sh")
        vm.onNtfyTopicChange("t")
        vm.onNotifyNtfyChange(true)
        vm.test("ntfy")
        advanceUntilIdle()

        val outcome = vm.uiState.value.testResult["ntfy"]
        assertTrue(outcome?.ok == true)
        assertNull(outcome?.error)
        assertNull(vm.uiState.value.testingChannel)
    }

    @Test
    fun `test reports a diagnosed failure distinctly`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.config() } returns Result.success(config())
        coEvery { repository.save(any()) } returns Result.success(config())
        coEvery { repository.test("gotify") } returns Result.success(
            NotificationTestResult(ok = false, error = "gotify is not configured"),
        )
        val vm = NotificationChannelsViewModel(repository)
        advanceUntilIdle()

        vm.onGotifyUrlChange("http://gotify:8080")
        vm.onNotifyGotifyChange(true)
        vm.test("gotify")
        advanceUntilIdle()

        val outcome = vm.uiState.value.testResult["gotify"]
        assertTrue(outcome?.ok == false)
        assertEquals("gotify is not configured", outcome?.error)
    }

    @Test
    fun `test surfaces a transport failure as a failed outcome`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.config() } returns Result.success(config())
        coEvery { repository.save(any()) } returns Result.success(config())
        coEvery { repository.test("ntfy") } returns Result.failure(ApiError.Server(500, "boom"))
        val vm = NotificationChannelsViewModel(repository)
        advanceUntilIdle()

        vm.onNtfyUrlChange("https://ntfy.sh")
        vm.onNtfyTopicChange("t")
        vm.test("ntfy")
        advanceUntilIdle()

        val outcome = vm.uiState.value.testResult["ntfy"]
        assertTrue(outcome?.ok == false)
        assertEquals("Server error (500)", outcome?.error)
    }

    @Test
    fun `test never runs when the pre-test save fails - the save error is the test result`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.config() } returns Result.success(config())
            coEvery { repository.save(any()) } returns Result.failure(ApiError.Client(400, "Invalid URL"))
            val vm = NotificationChannelsViewModel(repository)
            advanceUntilIdle()

            vm.onNtfyUrlChange("not-a-url")
            vm.onNotifyNtfyChange(true)
            vm.test("ntfy")
            advanceUntilIdle()

            val outcome = vm.uiState.value.testResult["ntfy"]
            assertTrue(outcome?.ok == false)
            assertEquals("Invalid URL", outcome?.error)
            coVerify(exactly = 0) { repository.test(any()) }
        }

    @Test
    fun `editing a field clears a stale test result`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.config() } returns Result.success(config())
        coEvery { repository.save(any()) } returns Result.success(config())
        coEvery { repository.test("ntfy") } returns Result.success(NotificationTestResult(ok = true))
        val vm = NotificationChannelsViewModel(repository)
        advanceUntilIdle()

        vm.onNtfyUrlChange("https://ntfy.sh")
        vm.onNtfyTopicChange("t")
        vm.onNotifyNtfyChange(true)
        vm.test("ntfy")
        advanceUntilIdle()
        assertTrue(vm.uiState.value.testResult.containsKey("ntfy"))

        vm.onNtfyUrlChange("https://ntfy.sh/2")
        advanceUntilIdle()

        assertTrue(vm.uiState.value.testResult.isEmpty())
    }
}
