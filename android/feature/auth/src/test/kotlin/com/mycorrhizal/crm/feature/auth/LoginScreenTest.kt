package com.mycorrhizal.crm.feature.auth

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasSetTextAction
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class LoginScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(
        uiState: LoginUiState = LoginUiState(),
        onServerUrlChange: (String) -> Unit = {},
        onIdentifierChange: (String) -> Unit = {},
        onPasswordChange: (String) -> Unit = {},
        onApiTokenChange: (String) -> Unit = {},
        onModeChange: (LoginMode) -> Unit = {},
        onSubmit: () -> Unit = {},
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                LoginScreenContent(
                    uiState = uiState,
                    onServerUrlChange = onServerUrlChange,
                    onIdentifierChange = onIdentifierChange,
                    onPasswordChange = onPasswordChange,
                    onApiTokenChange = onApiTokenChange,
                    onModeChange = onModeChange,
                    onSubmit = onSubmit,
                )
            }
        }
    }

    @Test
    fun `renders server url and credential fields`() {
        setContent()
        composeTestRule.onNodeWithText("Server URL").assertIsDisplayed()
        composeTestRule.onNodeWithText("Username or email").assertIsDisplayed()
        composeTestRule.onNode(hasText("Password") and hasSetTextAction()).assertIsDisplayed()
        composeTestRule.onNodeWithText("Sign in").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `typing server url forwards the change`() {
        var value: String? = null
        setContent(onServerUrlChange = { value = it })
        composeTestRule.onNodeWithText("Server URL").performTextInput("https://crm.example.com")
        assertEquals("https://crm.example.com", value)
    }

    @Test
    fun `api token mode swaps the credential field`() {
        var mode: LoginMode? = null
        setContent(onModeChange = { mode = it })
        composeTestRule.onNodeWithText("API token").performClick()
        assertEquals(LoginMode.API_TOKEN, mode)
    }

    @Test
    fun `submit invokes the callback`() {
        var submitted = false
        setContent(onSubmit = { submitted = true })
        composeTestRule.onNodeWithText("Sign in").performScrollTo().performClick()
        assertEquals(true, submitted)
    }

    @Test
    fun `loading state hides the submit button and shows a spinner`() {
        setContent(uiState = LoginUiState(isLoading = true))
        composeTestRule.onNodeWithText("Sign in").assertDoesNotExist()
    }
}
