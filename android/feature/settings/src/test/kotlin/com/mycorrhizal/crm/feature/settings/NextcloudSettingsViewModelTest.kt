package com.mycorrhizal.crm.feature.settings

import com.mycorrhizal.crm.domain.repository.NextcloudRepository
import com.mycorrhizal.crm.model.network.NextcloudConfigResponse
import com.mycorrhizal.crm.model.network.NextcloudConnectionTestResult
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class NextcloudSettingsViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val repository = mockk<NextcloudRepository>()

    @Test
    fun `load populates the config into the editable state`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getConfig() } returns Result.success(
            NextcloudConfigResponse(baseUrl = "https://nextcloud.example.com", username = "drew", hasAppPassword = true),
        )
        val vm = NextcloudSettingsViewModel(repository)
        advanceUntilIdle()

        val state = vm.uiState.value
        assertFalse(state.isLoading)
        assertEquals("https://nextcloud.example.com", state.baseUrl)
        assertEquals("drew", state.username)
        assertTrue(state.hasAppPassword)
    }

    @Test
    fun `load failure surfaces the load error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getConfig() } returns Result.failure(ApiError.Server(500, "boom"))
        val vm = NextcloudSettingsViewModel(repository)
        advanceUntilIdle()

        assertEquals("Server error (500)", vm.uiState.value.loadError)
    }

    @Test
    fun `save round-trips the config and clears the password field`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getConfig() } returns Result.success(NextcloudConfigResponse())
        coEvery { repository.saveConfig(any()) } returns Result.success(
            NextcloudConfigResponse(baseUrl = "https://nextcloud.example.com", username = "drew", hasAppPassword = true),
        )
        val vm = NextcloudSettingsViewModel(repository)
        advanceUntilIdle()

        vm.onBaseUrlChange("https://nextcloud.example.com")
        vm.onUsernameChange("drew")
        vm.onAppPasswordChange("password-123")
        vm.save()
        advanceUntilIdle()

        coVerify(exactly = 1) { repository.saveConfig(any()) }
        val state = vm.uiState.value
        assertEquals("https://nextcloud.example.com", state.baseUrl)
        assertEquals("drew", state.username)
        assertTrue(state.hasAppPassword)
        assertEquals("", state.appPassword)
        assertNull(state.saveError)
    }

    @Test
    fun `a typed app password is sent on save exactly once`() = runTest(mainDispatcherRule.testDispatcher) {
        val captured = mutableListOf<com.mycorrhizal.crm.model.network.NextcloudConfigInput>()
        coEvery { repository.getConfig() } returns Result.success(NextcloudConfigResponse())
        coEvery { repository.saveConfig(any()) } answers {
            captured.add(firstArg())
            Result.success(NextcloudConfigResponse(baseUrl = "https://nextcloud.example.com", username = "drew", hasAppPassword = true))
        }
        val vm = NextcloudSettingsViewModel(repository)
        advanceUntilIdle()

        vm.onBaseUrlChange("https://nextcloud.example.com")
        vm.onUsernameChange("drew")
        vm.onAppPasswordChange("password-123")
        vm.save()
        advanceUntilIdle()

        assertEquals("password-123", captured.last().appPassword)
        assertEquals("drew", captured.last().username)
        assertEquals("", vm.uiState.value.appPassword)
    }

    @Test
    fun `save failure surfaces the save error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getConfig() } returns Result.success(NextcloudConfigResponse())
        coEvery { repository.saveConfig(any()) } returns Result.failure(ApiError.Client(400, "Invalid URL"))
        val vm = NextcloudSettingsViewModel(repository)
        advanceUntilIdle()

        vm.onBaseUrlChange("not-a-url")
        vm.onUsernameChange("drew")
        vm.save()
        advanceUntilIdle()

        assertEquals("Invalid URL", vm.uiState.value.saveError)
    }

    @Test
    fun `test reports success distinctly`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getConfig() } returns Result.success(NextcloudConfigResponse())
        coEvery { repository.saveConfig(any()) } returns Result.success(
            NextcloudConfigResponse(baseUrl = "https://nextcloud.example.com", username = "drew", hasAppPassword = true),
        )
        coEvery { repository.testConnection() } returns Result.success(NextcloudConnectionTestResult(ok = true))
        val vm = NextcloudSettingsViewModel(repository)
        advanceUntilIdle()

        vm.onBaseUrlChange("https://nextcloud.example.com")
        vm.onUsernameChange("drew")
        vm.onAppPasswordChange("password-123")
        vm.test()
        advanceUntilIdle()

        val outcome = vm.uiState.value.testResult
        assertTrue(outcome?.ok == true)
        assertFalse(vm.uiState.value.isTesting)
    }

    @Test
    fun `test reports a diagnosed failure distinctly`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getConfig() } returns Result.success(NextcloudConfigResponse())
        coEvery { repository.saveConfig(any()) } returns Result.success(
            NextcloudConfigResponse(baseUrl = "https://nextcloud.example.com", username = "drew", hasAppPassword = true),
        )
        coEvery { repository.testConnection() } returns Result.success(
            NextcloudConnectionTestResult(ok = false, message = "invalid credentials"),
        )
        val vm = NextcloudSettingsViewModel(repository)
        advanceUntilIdle()

        vm.onBaseUrlChange("https://nextcloud.example.com")
        vm.onUsernameChange("drew")
        vm.test()
        advanceUntilIdle()

        val outcome = vm.uiState.value.testResult
        assertTrue(outcome?.ok == false)
        assertEquals("invalid credentials", outcome?.message)
    }

    @Test
    fun `test never calls test-connection when the pre-test save fails`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getConfig() } returns Result.success(NextcloudConfigResponse())
        coEvery { repository.saveConfig(any()) } returns Result.failure(ApiError.Client(400, "Invalid URL"))
        val vm = NextcloudSettingsViewModel(repository)
        advanceUntilIdle()

        vm.onBaseUrlChange("not-a-url")
        vm.onUsernameChange("drew")
        vm.test()
        advanceUntilIdle()

        val outcome = vm.uiState.value.testResult
        assertTrue(outcome?.ok == false)
        assertEquals("Invalid URL", outcome?.message)
        coVerify(exactly = 0) { repository.testConnection() }
    }

    @Test
    fun `remove clears the connection back to a fresh state`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getConfig() } returns Result.success(
            NextcloudConfigResponse(baseUrl = "https://nextcloud.example.com", username = "drew", hasAppPassword = true),
        )
        coEvery { repository.deleteConfig() } returns Result.success(Unit)
        val vm = NextcloudSettingsViewModel(repository)
        advanceUntilIdle()
        assertTrue(vm.uiState.value.hasAppPassword)

        vm.remove()
        advanceUntilIdle()

        coVerify(exactly = 1) { repository.deleteConfig() }
        assertFalse(vm.uiState.value.hasAppPassword)
        assertEquals("", vm.uiState.value.baseUrl)
        assertEquals("", vm.uiState.value.username)
    }

    @Test
    fun `remove failure surfaces the save error and keeps the connection`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getConfig() } returns Result.success(
            NextcloudConfigResponse(baseUrl = "https://nextcloud.example.com", username = "drew", hasAppPassword = true),
        )
        coEvery { repository.deleteConfig() } returns Result.failure(ApiError.Server(500, "boom"))
        val vm = NextcloudSettingsViewModel(repository)
        advanceUntilIdle()

        vm.remove()
        advanceUntilIdle()

        assertEquals("Server error (500)", vm.uiState.value.saveError)
        assertTrue(vm.uiState.value.hasAppPassword)
    }

    @Test
    fun `editing a field clears a stale test result`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getConfig() } returns Result.success(NextcloudConfigResponse())
        coEvery { repository.saveConfig(any()) } returns Result.success(
            NextcloudConfigResponse(baseUrl = "https://nextcloud.example.com", username = "drew", hasAppPassword = true),
        )
        coEvery { repository.testConnection() } returns Result.success(NextcloudConnectionTestResult(ok = true))
        val vm = NextcloudSettingsViewModel(repository)
        advanceUntilIdle()

        vm.onBaseUrlChange("https://nextcloud.example.com")
        vm.onUsernameChange("drew")
        vm.test()
        advanceUntilIdle()
        assertTrue(vm.uiState.value.testResult != null)

        vm.onUsernameChange("drew2")
        advanceUntilIdle()

        assertNull(vm.uiState.value.testResult)
    }
}
