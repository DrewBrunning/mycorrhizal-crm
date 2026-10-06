package com.mycorrhizal.crm.feature.settings

import com.mycorrhizal.crm.domain.repository.GeoPulseRepository
import com.mycorrhizal.crm.model.network.GeoPulseConfigInput
import com.mycorrhizal.crm.model.network.GeoPulseConfigResponse
import com.mycorrhizal.crm.model.network.GeoPulseConnectionTestResult
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import com.mycorrhizal.crm.ui.R
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

class GeoPulseSettingsViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val repository = mockk<GeoPulseRepository>()

    private fun connected(url: String = "https://geo.example.com") =
        GeoPulseConfigResponse(baseUrl = url, hasApiKey = true)

    @Test
    fun `load populates the config and never the token`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getConfig() } returns Result.success(connected())
        val vm = GeoPulseSettingsViewModel(repository)
        advanceUntilIdle()

        val state = vm.uiState.value
        assertFalse(state.isLoading)
        assertEquals("https://geo.example.com", state.baseUrl)
        assertTrue(state.hasApiKey)
        assertEquals("", state.apiKey)
    }

    @Test
    fun `load failure surfaces the load error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getConfig() } returns Result.failure(ApiError.Server(500, "boom"))
        val vm = GeoPulseSettingsViewModel(repository)
        advanceUntilIdle()

        assertEquals("Server error (500)", vm.uiState.value.loadError)
    }

    @Test
    fun `first connect requires the token and does not call the server without it`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.getConfig() } returns Result.success(GeoPulseConfigResponse())
            val vm = GeoPulseSettingsViewModel(repository)
            advanceUntilIdle()

            vm.onBaseUrlChange("https://geo.example.com")
            assertTrue(vm.uiState.value.tokenRequired)
            vm.save()
            advanceUntilIdle()

            assertEquals(R.string.geopulse_settings_api_key_required, vm.uiState.value.saveErrorRes)
            coVerify(exactly = 0) { repository.saveConfig(any()) }
        }

    @Test
    fun `first connect saves base url and token then clears the token field`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.getConfig() } returns Result.success(GeoPulseConfigResponse())
            coEvery { repository.saveConfig(any()) } returns Result.success(connected())
            val vm = GeoPulseSettingsViewModel(repository)
            advanceUntilIdle()

            vm.onBaseUrlChange("  https://geo.example.com ")
            vm.onApiKeyChange("secret-token")
            vm.save()
            advanceUntilIdle()

            coVerify { repository.saveConfig(GeoPulseConfigInput("https://geo.example.com", "secret-token")) }
            val state = vm.uiState.value
            assertTrue(state.hasApiKey)
            assertEquals("", state.apiKey)
            assertNull(state.saveError)
            assertFalse(state.tokenRequired)
        }

    @Test
    fun `same-origin path change keeps the stored token and sends an empty one`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.getConfig() } returns Result.success(connected("https://geo.example.com"))
            coEvery { repository.saveConfig(any()) } returns Result.success(connected("https://GEO.example.com:443/app"))
            val vm = GeoPulseSettingsViewModel(repository)
            advanceUntilIdle()

            // Case-insensitive host and the default port equate: still the same origin.
            vm.onBaseUrlChange("https://GEO.example.com:443/app")
            assertFalse(vm.uiState.value.tokenRequired)
            vm.save()
            advanceUntilIdle()

            coVerify { repository.saveConfig(GeoPulseConfigInput("https://GEO.example.com:443/app", "")) }
        }

    @Test
    fun `changing the origin requires re-entering the token (issue 1501)`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.getConfig() } returns Result.success(connected("https://geo.example.com"))
            val vm = GeoPulseSettingsViewModel(repository)
            advanceUntilIdle()

            vm.onBaseUrlChange("https://evil.example.net")
            assertTrue(vm.uiState.value.tokenRequiredByOriginChange)
            vm.save()
            advanceUntilIdle()

            assertEquals(R.string.geopulse_settings_api_key_required_origin_change, vm.uiState.value.saveErrorRes)
            coVerify(exactly = 0) { repository.saveConfig(any()) }

            // Scheme and port changes are origin changes too.
            vm.onBaseUrlChange("http://geo.example.com")
            assertTrue(vm.uiState.value.tokenRequired)
            vm.onBaseUrlChange("https://geo.example.com:8443")
            assertTrue(vm.uiState.value.tokenRequired)
        }

    @Test
    fun `origin change with a fresh token is sent`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getConfig() } returns Result.success(connected("https://geo.example.com"))
        coEvery { repository.saveConfig(any()) } returns Result.success(connected("https://new.example.com"))
        val vm = GeoPulseSettingsViewModel(repository)
        advanceUntilIdle()

        vm.onBaseUrlChange("https://new.example.com")
        vm.onApiKeyChange("fresh")
        vm.save()
        advanceUntilIdle()

        coVerify { repository.saveConfig(GeoPulseConfigInput("https://new.example.com", "fresh")) }
        assertEquals("https://new.example.com", vm.uiState.value.storedBaseUrl)
    }

    @Test
    fun `invalid and blank base urls are rejected client-side`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getConfig() } returns Result.success(GeoPulseConfigResponse())
        val vm = GeoPulseSettingsViewModel(repository)
        advanceUntilIdle()

        vm.onBaseUrlChange("   ")
        vm.save()
        assertEquals(R.string.geopulse_settings_base_url_required, vm.uiState.value.saveErrorRes)

        vm.onBaseUrlChange("ftp://geo.example.com")
        vm.onApiKeyChange("t")
        vm.save()
        assertEquals(R.string.geopulse_settings_base_url_invalid, vm.uiState.value.saveErrorRes)
        coVerify(exactly = 0) { repository.saveConfig(any()) }
    }

    @Test
    fun `a server rejection of the token is shown`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getConfig() } returns Result.success(GeoPulseConfigResponse())
        coEvery { repository.saveConfig(any()) } returns
            Result.failure(ApiError.Client(400, "re-enter the API token when changing the GeoPulse server"))
        val vm = GeoPulseSettingsViewModel(repository)
        advanceUntilIdle()

        vm.onBaseUrlChange("https://geo.example.com")
        vm.onApiKeyChange("t")
        vm.save()
        advanceUntilIdle()

        assertEquals("re-enter the API token when changing the GeoPulse server", vm.uiState.value.saveError)
        assertFalse(vm.uiState.value.isSaving)
    }

    @Test
    fun `test connection reports the stage and message of a diagnosed failure`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.getConfig() } returns Result.success(connected())
            coEvery { repository.testConnection() } returns Result.success(
                GeoPulseConnectionTestResult(ok = false, stage = "auth", message = "API token rejected"),
            )
            val vm = GeoPulseSettingsViewModel(repository)
            advanceUntilIdle()

            vm.test()
            advanceUntilIdle()

            val result = vm.uiState.value.testResult
            assertEquals(GeoPulseTestOutcome(ok = false, stage = "auth", message = "API token rejected"), result)
            assertFalse(vm.uiState.value.isTesting)
        }

    @Test
    fun `test connection transport failure is a failed outcome`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getConfig() } returns Result.success(connected())
        coEvery { repository.testConnection() } returns Result.failure(ApiError.Client(400, "No GeoPulse connection"))
        val vm = GeoPulseSettingsViewModel(repository)
        advanceUntilIdle()

        vm.test()
        advanceUntilIdle()

        assertFalse(vm.uiState.value.testResult!!.ok)
        assertEquals("No GeoPulse connection", vm.uiState.value.testResult!!.message)
    }

    @Test
    fun `disconnect resets to the unconnected state`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getConfig() } returns Result.success(connected())
        coEvery { repository.deleteConfig() } returns Result.success(Unit)
        val vm = GeoPulseSettingsViewModel(repository)
        advanceUntilIdle()

        vm.remove()
        advanceUntilIdle()

        val state = vm.uiState.value
        assertFalse(state.hasApiKey)
        assertEquals("", state.baseUrl)
        assertFalse(state.isRemoving)
    }

    @Test
    fun `origin helper equates default ports and rejects non-http urls`() {
        assertTrue(sameGeoPulseOrigin("http://h", "http://H:80/x"))
        assertFalse(sameGeoPulseOrigin("http://h", "https://h"))
        assertFalse(sameGeoPulseOrigin("not a url", "http://h"))
        assertNull(geoPulseOriginOf("mailto:a@b.c"))
    }
}
