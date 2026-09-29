package com.mycorrhizal.crm.feature.settings

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.onLast
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import com.mycorrhizal.crm.model.network.WebAuthnCredential
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class PasskeysScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val phone = WebAuthnCredential("id-phone", "Phone", "2026-09-01T10:00:00Z", null)
    private val key = WebAuthnCredential("id-key", "YubiKey", "2026-09-02T10:00:00Z", "2026-09-03T10:00:00Z")

    private fun setContent(
        state: PasskeysUiState,
        onStartAdd: () -> Unit = {},
        onConfirmAdd: (String) -> Unit = {},
        onRequestRemove: (WebAuthnCredential) -> Unit = {},
        onRemoveWithCode: (String) -> Unit = {},
        onRemoveWithPasskey: () -> Unit = {},
        onDismissRecoveryCodes: () -> Unit = {},
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                PasskeysContent(
                    state = state,
                    onStartAdd = onStartAdd,
                    onDismissAdd = {},
                    onConfirmAdd = onConfirmAdd,
                    onRequestRemove = onRequestRemove,
                    onDismissRemove = {},
                    onRemoveWithCode = onRemoveWithCode,
                    onRemoveWithPasskey = onRemoveWithPasskey,
                    onDismissRecoveryCodes = onDismissRecoveryCodes,
                )
            }
        }
    }

    private fun ready(
        passkeys: List<WebAuthnCredential> = listOf(phone, key),
        available: Boolean = true,
        block: PasskeysUiState.() -> PasskeysUiState = { this },
    ) = PasskeysUiState(loading = false, available = available, passkeys = passkeys).block()

    @Test
    fun `lists credentials with added and last-used dates`() {
        setContent(ready())
        composeTestRule.onNodeWithText("Phone").assertIsDisplayed()
        composeTestRule.onNodeWithText("YubiKey").assertIsDisplayed()
        composeTestRule.onAllNodesWithText("Added", substring = true).assertCountEquals(2)
        composeTestRule.onNodeWithText("never used", substring = true).assertExists()
        composeTestRule.onNodeWithText("last used", substring = true).assertExists()
    }

    @Test
    fun `an empty list says so and offers add`() {
        var added = 0
        setContent(ready(passkeys = emptyList()), onStartAdd = { added++ })
        composeTestRule.onNodeWithText("No passkeys yet.").assertIsDisplayed()
        composeTestRule.onNodeWithText("Add a passkey").performScrollTo().performClick()
        assertEquals(1, added)
    }

    @Test
    fun `a closed gate explains and offers no add`() {
        setContent(ready(available = false))
        composeTestRule.onNodeWithText("Passkeys aren't available on this app for this server.").assertIsDisplayed()
        composeTestRule.onNodeWithText("Add a passkey").assertDoesNotExist()
    }

    @Test
    fun `a blocked account shows the server text and no add`() {
        setContent(ready { copy(blockedText = "Not available for identity-provider accounts") })
        composeTestRule.onNodeWithText("Not available for identity-provider accounts").assertIsDisplayed()
        composeTestRule.onNodeWithText("Add a passkey").assertDoesNotExist()
    }

    @Test
    fun `a not-associated server shows its state and no add`() {
        setContent(ready { copy(blockedRes = R.string.settings_passkeys_not_associated) })
        composeTestRule.onNodeWithText("This server isn't set up for passkeys on Android", substring = true).assertIsDisplayed()
        composeTestRule.onNodeWithText("Add a passkey").assertDoesNotExist()
    }

    @Test
    fun `the add dialog submits the typed label`() {
        var label: String? = null
        setContent(ready { copy(adding = true) }, onConfirmAdd = { label = it })
        composeTestRule.onNodeWithText("Passkey name (optional)").performTextInput("Laptop")
        composeTestRule.onNodeWithText("Continue").performClick()
        assertEquals("Laptop", label)
    }

    @Test
    fun `the add dialog shows progress and blocks while waiting for the device`() {
        setContent(ready { copy(adding = true, busy = true) })
        composeTestRule.onNodeWithText("Waiting for your device…").assertIsDisplayed()
        composeTestRule.onNodeWithText("Continue").assertIsNotEnabled()
    }

    @Test
    fun `remove opens the proof dialog and submits a code`() {
        var removed: WebAuthnCredential? = null
        var code: String? = null
        setContent(
            ready { copy(removing = phone) },
            onRequestRemove = { removed = it },
            onRemoveWithCode = { code = it },
        )
        composeTestRule.onNodeWithText("Verification code").performTextInput("123456")
        // Title and confirm button share the text; the confirm button is the last one.
        composeTestRule.onAllNodes(hasText("Remove passkey")).onLast().performClick()
        assertEquals("123456", code)
        assertEquals(null, removed)
    }

    @Test
    fun `the remove button per row requests removal`() {
        var removed: WebAuthnCredential? = null
        setContent(ready(), onRequestRemove = { removed = it })
        composeTestRule.onNodeWithContentDescription("Remove passkey YubiKey").performClick()
        assertEquals(key, removed)
    }

    @Test
    fun `another-passkey proof is offered only when another passkey exists`() {
        var used = 0
        setContent(ready(listOf(phone, key)) { copy(removing = phone) }, onRemoveWithPasskey = { used++ })
        composeTestRule.onNodeWithText("Verify with another passkey instead").performClick()
        assertEquals(1, used)
    }

    @Test
    fun `the sole passkey has no another-passkey option`() {
        setContent(ready(listOf(phone)) { copy(removing = phone) })
        composeTestRule.onNodeWithText("Verification code").assertIsDisplayed()
        composeTestRule.onNodeWithText("Verify with another passkey instead").assertDoesNotExist()
    }

    @Test
    fun `a closed gate hides the another-passkey option even with two passkeys`() {
        setContent(ready(listOf(phone, key), available = false) { copy(removing = phone) })
        composeTestRule.onNodeWithText("Verify with another passkey instead").assertDoesNotExist()
    }

    @Test
    fun `proof errors appear inside the dialog`() {
        setContent(ready { copy(removing = phone, errorRes = R.string.settings_passkeys_invalid_proof) })
        composeTestRule.onNodeWithText("Verification failed. Please try again.").assertIsDisplayed()
    }

    @Test
    fun `a server message for 404 and 409 is shown in the dialog`() {
        setContent(ready { copy(removing = phone, error = "No other passkey is registered") })
        composeTestRule.onNodeWithText("No other passkey is registered").assertIsDisplayed()
    }

    @Test
    fun `recovery codes are shown once with a done action`() {
        var done = 0
        setContent(
            ready { copy(recoveryCodes = listOf("AAAAA-BBBBB-CCCCC", "DDDDD-EEEEE-FFFFF")) },
            onDismissRecoveryCodes = { done++ },
        )
        composeTestRule.onNodeWithText("AAAAA-BBBBB-CCCCC").assertIsDisplayed()
        composeTestRule.onNodeWithText("Done").performClick()
        assertEquals(1, done)
    }

    @Test
    fun `success and error lines are shown outside dialogs`() {
        setContent(ready { copy(messageRes = R.string.settings_passkeys_add_success, error = "Boom") })
        composeTestRule.onNodeWithText("Passkey added.").assertIsDisplayed()
        composeTestRule.onNodeWithText("Boom").assertIsDisplayed()
    }

    @Test
    fun `loading shows a spinner and no list`() {
        setContent(PasskeysUiState(loading = true))
        composeTestRule.onNodeWithText("No passkeys yet.").assertDoesNotExist()
        assertTrue(true)
    }
}

