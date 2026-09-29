package com.mycorrhizal.crm.feature.settings

import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.data.session.SwitchProfileResult
import com.mycorrhizal.crm.domain.profile.ServerProfile
import com.mycorrhizal.crm.domain.profile.ServerProfileKind
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.flow.MutableStateFlow
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ServersScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val profiles = MutableStateFlow<List<ServerProfile>>(emptyList())
    private val active = MutableStateFlow<ServerProfile?>(null)
    private val session = mockk<SessionManager>(relaxed = true) {
        every { observeProfiles() } returns profiles
        every { observeActiveProfile() } returns active
    }

    private fun profile(id: String, label: String, url: String = "https://$id.example") =
        ServerProfile(id = id, kind = ServerProfileKind.Remote(url), label = label)

    private fun setScreen(localModeEnabled: Boolean = false) {
        val viewModel = ServersViewModel(session, mockk(relaxed = true), mockk(relaxed = true))
        composeTestRule.setContent {
            MycorrhizalTheme {
                ServersScreen(onBack = {}, localModeEnabled = localModeEnabled, viewModel = viewModel)
            }
        }
        composeTestRule.waitForIdle()
    }

    @Test
    fun `renders each profile and marks the active one`() {
        profiles.value = listOf(profile("p1", "Home"), profile("p2", "Work"))
        active.value = profile("p1", "Home")
        setScreen()

        composeTestRule.onNodeWithText("Home").assertIsDisplayed()
        composeTestRule.onNodeWithText("Work").assertIsDisplayed()
        composeTestRule.onNodeWithText("Active").assertIsDisplayed()
    }

    @Test
    fun `adding a server sends the label and url to the view model`() {
        coEvery { session.addRemoteProfile("Work", "https://work.example.com") } returns
            profile("p9", "Work", "https://work.example.com")
        coEvery { session.switchProfile("p9", false) } returns SwitchProfileResult.Switched
        setScreen()

        composeTestRule.onNodeWithContentDescription("Add server").performClick()
        composeTestRule.onNodeWithTag("servers-add-label").performTextInput("Work")
        composeTestRule.onNodeWithTag("servers-add-url").performTextInput("https://work.example.com")
        composeTestRule.onNodeWithTag("servers-add-confirm").performClick()
        composeTestRule.waitForIdle()

        coVerify { session.addRemoteProfile("Work", "https://work.example.com") }
    }

    @Test
    fun `a switch blocked by unsent activity names the count`() {
        profiles.value = listOf(profile("p1", "Home"), profile("p2", "Work"))
        active.value = profile("p1", "Home")
        coEvery { session.switchProfile("p2", false) } returns SwitchProfileResult.NeedsConfirmation(5)
        setScreen()

        composeTestRule.onNodeWithTag("server-row-p2").performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Unsent activity").assertIsDisplayed()
        composeTestRule.onNodeWithText("Discard and switch").assertIsDisplayed()
    }

    @Test
    fun `the local-only entry is hidden unless the build flag is on`() {
        setScreen(localModeEnabled = false)
        composeTestRule.onAllNodesWithText("Use on this device only").assertCountEquals(0)
    }

    @Test
    fun `the local-only entry is shown when the build flag is on`() {
        setScreen(localModeEnabled = true)
        composeTestRule.onNodeWithText("Use on this device only").assertIsDisplayed()
    }
}
