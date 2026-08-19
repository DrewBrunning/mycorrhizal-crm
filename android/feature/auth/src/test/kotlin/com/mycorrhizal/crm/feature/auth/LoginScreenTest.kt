package com.mycorrhizal.crm.feature.auth

import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
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
        onModeChange: (LoginMode) -> Unit = {},
        onSubmit: (String, String, String, String) -> Unit = { _, _, _, _ -> },
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                LoginScreenContent(
                    uiState = uiState,
                    onServerUrlChange = onServerUrlChange,
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
        composeTestRule.onNodeWithText("Username or email").performScrollTo().assertIsDisplayed()
        composeTestRule.onNode(hasText("Password") and hasSetTextAction()).performScrollTo().assertIsDisplayed()
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
    fun `submit passes the entered values to the callback`() {
        var captured: List<String>? = null
        setContent(onSubmit = { serverUrl, identifier, password, apiToken ->
            captured = listOf(serverUrl, identifier, password, apiToken)
        })
        composeTestRule.onNodeWithText("Server URL").performTextInput("https://crm.example.com")
        composeTestRule.onNodeWithText("Username or email").performTextInput("alice")
        composeTestRule.onNode(hasText("Password") and hasSetTextAction()).performTextInput("secret")
        composeTestRule.onNodeWithText("Sign in").performScrollTo().performClick()

        assertEquals(listOf("https://crm.example.com", "alice", "secret", ""), captured)
    }

    @Test
    fun `loading state hides the submit button`() {
        setContent(uiState = LoginUiState(isLoading = true))
        composeTestRule.onNodeWithText("Sign in").assertDoesNotExist()
    }

    // M26: the login screen links the register and forgot-password flows.
    @Test
    fun `register and forgot-password links are present and invoke their callbacks`() {
        var registered = false
        var forgot = false
        composeTestRule.setContent {
            MycorrhizalTheme {
                LoginScreenContent(
                    uiState = LoginUiState(),
                    onServerUrlChange = {},
                    onModeChange = {},
                    onSubmit = { _, _, _, _ -> },
                    onRegisterClick = { registered = true },
                    onForgotPasswordClick = { forgot = true },
                )
            }
        }

        composeTestRule.onNodeWithText("No account? Create one").performScrollTo().performClick()
        composeTestRule.onNodeWithText("Forgot password?").performScrollTo().performClick()

        assertEquals(true, registered)
        assertEquals(true, forgot)
    }

    // #203: the OIDC native-return failure (MainActivity) used to be a Toast
    // -- it's now injected as `oidcError` and shown through this screen's own
    // SnackbarHostState, same as the screen's own submit errors.
    @Test
    fun `an injected oidc error is shown via the snackbar`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                LoginScreenContent(
                    uiState = LoginUiState(),
                    onServerUrlChange = {},
                    onModeChange = {},
                    onSubmit = { _, _, _, _ -> },
                    oidcError = "Single sign-on failed",
                    onOidcErrorShown = {},
                )
            }
        }

        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithText("Single sign-on failed").assertIsDisplayed()
    }

    @Test
    fun `the snackbar host is an assertive live region`() {
        setContent()

        // Not exercising assertIsDisplayed() -- an empty SnackbarHost with no
        // active message has zero visible size, which Compose UI testing
        // treats as "not displayed" even though the node (and its live
        // region) genuinely exists in the tree.
        composeTestRule.onNode(SemanticsMatcher.expectValue(SemanticsProperties.LiveRegion, LiveRegionMode.Assertive))
            .assert(SemanticsMatcher.expectValue(SemanticsProperties.LiveRegion, LiveRegionMode.Assertive))
    }

    @Test
    fun `the loading spinner announces itself while signing in`() {
        setContent(uiState = LoginUiState(isLoading = true))

        composeTestRule.onNode(SemanticsMatcher.expectValue(SemanticsProperties.ContentDescription, listOf("Saving")))
            .assert(SemanticsMatcher.expectValue(SemanticsProperties.ContentDescription, listOf("Saving")))
    }
}
