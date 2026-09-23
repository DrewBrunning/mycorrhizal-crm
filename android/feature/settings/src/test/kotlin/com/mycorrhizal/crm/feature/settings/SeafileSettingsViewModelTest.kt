package com.mycorrhizal.crm.feature.settings

import com.mycorrhizal.crm.domain.repository.SeafileRepository
import com.mycorrhizal.crm.model.network.SeafileConfigResponse
import com.mycorrhizal.crm.model.network.SeafileConnectionTestResult
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

class SeafileSettingsViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val repository = mockk<SeafileRepository>()

    @Test
    fun `load populates the config into the editable state`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getConfig() } returns Result.success(
            SeafileConfigResponse(baseUrl = "http://seafile:8000", hasApiToken = true),
        )
        val vm = SeafileSettingsViewModel(repository)
        advanceUntilIdle()

        val state = vm.uiState.value
        assertFalse(state.isLoading)
        assertEquals("http://seafile:8000", state.baseUrl)
        assertTrue(state.hasApiToken)
    }

    @Test
    fun `load failure surfaces the load error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getConfig() } returns Result.failure(ApiError.Server(500, "boom"))
        val vm = SeafileSettingsViewModel(repository)
        advanceUntilIdle()

        assertEquals("Server error (500)", vm.uiState.value.loadError)
    }

    @Test
    fun `save round-trips the config and clears the token field`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getConfig() } returns Result.success(SeafileConfigResponse())
        coEvery { repository.saveConfig(any()) } returns Result.success(
            SeafileConfigResponse(baseUrl = "http://seafile:8000", hasApiToken = true),
        )
        val vm = SeafileSettingsViewModel(repository)
        advanceUntilIdle()

        vm.onBaseUrlChange("http://seafile:8000")
        vm.onApiTokenChange("token-123")
        vm.save()
        advanceUntilIdle()

        coVerify(exactly = 1) { repository.saveConfig(any()) }
        val state = vm.uiState.value
        assertEquals("http://seafile:8000", state.baseUrl)
        assertTrue(state.hasApiToken)
        assertEquals("", state.apiToken)
        assertNull(state.saveError)
    }

    @Test
    fun `a typed api token is sent on save exactly once`() = runTest(mainDispatcherRule.testDispatcher) {
        val captured = mutableListOf<com.mycorrhizal.crm.model.network.SeafileConfigInput>()
        coEvery { repository.getConfig() } returns Result.success(SeafileConfigResponse())
        coEvery { repository.saveConfig(any()) } answers {
            captured.add(firstArg())
            Result.success(SeafileConfigResponse(baseUrl = "http://seafile:8000", hasApiToken = true))
        }
        val vm = SeafileSettingsViewModel(repository)
        advanceUntilIdle()

        vm.onBaseUrlChange("http://seafile:8000")
        vm.onApiTokenChange("token-123")
        vm.save()
        advanceUntilIdle()

        assertEquals("token-123", captured.last().apiToken)
        assertEquals("", vm.uiState.value.apiToken)
    }

    @Test
    fun `save failure surfaces the save error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getConfig() } returns Result.success(SeafileConfigResponse())
        coEvery { repository.saveConfig(any()) } returns Result.failure(ApiError.Client(400, "Invalid URL"))
        val vm = SeafileSettingsViewModel(repository)
        advanceUntilIdle()

        vm.onBaseUrlChange("not-a-url")
        vm.save()
        advanceUntilIdle()

        assertEquals("Invalid URL", vm.uiState.value.saveError)
    }

    @Test
    fun `test reports success distinctly`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getConfig() } returns Result.success(SeafileConfigResponse())
        coEvery { repository.saveConfig(any()) } returns Result.success(
            SeafileConfigResponse(baseUrl = "http://seafile:8000", hasApiToken = true),
        )
        coEvery { repository.testConnection() } returns Result.success(SeafileConnectionTestResult(ok = true))
        val vm = SeafileSettingsViewModel(repository)
        advanceUntilIdle()

        vm.onBaseUrlChange("http://seafile:8000")
        vm.onApiTokenChange("token-123")
        vm.test()
        advanceUntilIdle()

        val outcome = vm.uiState.value.testResult
        assertTrue(outcome?.ok == true)
        assertFalse(vm.uiState.value.isTesting)
    }

    @Test
    fun `test reports a diagnosed failure distinctly`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getConfig() } returns Result.success(SeafileConfigResponse())
        coEvery { repository.saveConfig(any()) } returns Result.success(
            SeafileConfigResponse(baseUrl = "http://seafile:8000", hasApiToken = true),
        )
        coEvery { repository.testConnection() } returns Result.success(
            SeafileConnectionTestResult(ok = false, message = "invalid API token"),
        )
        val vm = SeafileSettingsViewModel(repository)
        advanceUntilIdle()

        vm.onBaseUrlChange("http://seafile:8000")
        vm.test()
        advanceUntilIdle()

        val outcome = vm.uiState.value.testResult
        assertTrue(outcome?.ok == false)
        assertEquals("invalid API token", outcome?.message)
    }

    @Test
    fun `test never calls test-connection when the pre-test save fails`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getConfig() } returns Result.success(SeafileConfigResponse())
        coEvery { repository.saveConfig(any()) } returns Result.failure(ApiError.Client(400, "Invalid URL"))
        val vm = SeafileSettingsViewModel(repository)
        advanceUntilIdle()

        vm.onBaseUrlChange("not-a-url")
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
            SeafileConfigResponse(baseUrl = "http://seafile:8000", hasApiToken = true),
        )
        coEvery { repository.deleteConfig() } returns Result.success(Unit)
        val vm = SeafileSettingsViewModel(repository)
        advanceUntilIdle()
        assertTrue(vm.uiState.value.hasApiToken)

        vm.remove()
        advanceUntilIdle()

        coVerify(exactly = 1) { repository.deleteConfig() }
        assertFalse(vm.uiState.value.hasApiToken)
        assertEquals("", vm.uiState.value.baseUrl)
    }

    @Test
    fun `remove failure surfaces the save error and keeps the connection`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getConfig() } returns Result.success(
            SeafileConfigResponse(baseUrl = "http://seafile:8000", hasApiToken = true),
        )
        coEvery { repository.deleteConfig() } returns Result.failure(ApiError.Server(500, "boom"))
        val vm = SeafileSettingsViewModel(repository)
        advanceUntilIdle()

        vm.remove()
        advanceUntilIdle()

        assertEquals("Server error (500)", vm.uiState.value.saveError)
        assertTrue(vm.uiState.value.hasApiToken)
    }

    @Test
    fun `editing a field clears a stale test result`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getConfig() } returns Result.success(SeafileConfigResponse())
        coEvery { repository.saveConfig(any()) } returns Result.success(
            SeafileConfigResponse(baseUrl = "http://seafile:8000", hasApiToken = true),
        )
        coEvery { repository.testConnection() } returns Result.success(SeafileConnectionTestResult(ok = true))
        val vm = SeafileSettingsViewModel(repository)
        advanceUntilIdle()

        vm.onBaseUrlChange("http://seafile:8000")
        vm.test()
        advanceUntilIdle()
        assertTrue(vm.uiState.value.testResult != null)

        vm.onBaseUrlChange("http://seafile:8000/2")
        advanceUntilIdle()

        assertNull(vm.uiState.value.testResult)
    }
}
