package com.mycorrhizal.crm.feature.settings

import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import com.mycorrhizal.crm.model.network.TwoFactorSetupResponse
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
class TwoFactorScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(
        state: TwoFactorUiState = TwoFactorUiState(),
        onEnable: () -> Unit = {},
        onRegenerate: () -> Unit = {},
        onDisable: () -> Unit = {},
        onConfirmSetup: (String) -> Unit = {},
        onCloseSetup: () -> Unit = {},
        onSubmitSetupProofCode: (String) -> Unit = {},
        onSetupProofWithPasskey: () -> Unit = {},
        onDismissSetupProof: () -> Unit = {},
        onSubmitPromptCode: (String) -> Unit = {},
        onPromptWithPasskey: () -> Unit = {},
        onDismissPrompt: () -> Unit = {},
        onDismissRecoveryCodes: () -> Unit = {},
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                TwoFactorContent(
                    state = state,
                    onEnable = onEnable,
                    onRegenerate = onRegenerate,
                    onDisable = onDisable,
                    onConfirmSetup = onConfirmSetup,
                    onCloseSetup = onCloseSetup,
                    onSubmitSetupProofCode = onSubmitSetupProofCode,
                    onSetupProofWithPasskey = onSetupProofWithPasskey,
                    onDismissSetupProof = onDismissSetupProof,
                    onSubmitPromptCode = onSubmitPromptCode,
                    onPromptWithPasskey = onPromptWithPasskey,
                    onDismissPrompt = onDismissPrompt,
                    onDismissRecoveryCodes = onDismissRecoveryCodes,
                )
            }
        }
    }

    @Test
    fun `disabled state offers the enable action`() {
        var enabled = false
        setContent(
            state = TwoFactorUiState(loading = false, enabled = false),
            onEnable = { enabled = true },
        )
        composeTestRule.onNodeWithText("Enable two-factor authentication").performScrollTo().performClick()
        assertTrue(enabled)
    }

    @Test
    fun `enabled state shows the badge and the regenerate and disable actions`() {
        var regenerated = false
        var disabled = false
        setContent(
            state = TwoFactorUiState(loading = false, enabled = true),
            onRegenerate = { regenerated = true },
            onDisable = { disabled = true },
        )
        composeTestRule.onNodeWithText("Two-factor authentication is enabled").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Regenerate recovery codes").performScrollTo().performClick()
        composeTestRule.onNodeWithText("Disable").performScrollTo().performClick()
        assertTrue(regenerated && disabled)
    }

    @Test
    fun `loading shows a spinner without actions`() {
        setContent(state = TwoFactorUiState(loading = true))
        composeTestRule.onNodeWithText("Enable two-factor authentication").assertDoesNotExist()
    }

    @Test
    fun `an error is surfaced inline`() {
        setContent(
            state = TwoFactorUiState(
                loading = false,
                enabled = false,
                error = "Two-factor authentication is unavailable for accounts that sign in with SSO",
            ),
        )
        composeTestRule.onNodeWithText("Two-factor authentication is unavailable for accounts that sign in with SSO")
            .assertIsDisplayed()
    }

    @Test
    fun `a pending setup opens the wizard inside the screen`() {
        var confirmed: String? = null
        setContent(
            state = TwoFactorUiState(
                loading = false,
                enabled = false,
                setup = TwoFactorSetupResponse(secret = "JBSWY3DPEHPK3PXP", otpauthUrl = "otpauth://totp/x"),
            ),
            onConfirmSetup = { confirmed = it },
        )
        composeTestRule.onNodeWithText("Verification code").performTextInput("123456")
        composeTestRule.onNodeWithText("Enable and continue").performClick()

        assertEquals("123456", confirmed)
    }

    @Test
    fun `a prompt opens the code dialog inside the screen`() {
        var submitted: String? = null
        setContent(
            state = TwoFactorUiState(loading = false, enabled = true, prompt = TwoFactorPrompt.DISABLE),
            onSubmitPromptCode = { submitted = it },
        )
        composeTestRule.onNodeWithText("Disable two-factor authentication").assertIsDisplayed()
        composeTestRule.onNodeWithText("Verification code").performTextInput("654321")
        composeTestRule.onNodeWithText("Confirm").performClick()

        assertEquals("654321", submitted)
    }

    @Test
    fun `a passkey-only account is offered regenerate recovery codes`() {
        var regenerate = false
        setContent(
            state = TwoFactorUiState(loading = false, enabled = false, hasPasskey = true),
            onRegenerate = { regenerate = true },
        )
        composeTestRule.onNodeWithText("Regenerate recovery codes").performScrollTo().performClick()
        assertTrue(regenerate)
    }

    @Test
    fun `an account with no second factor is not offered regenerate`() {
        setContent(state = TwoFactorUiState(loading = false, enabled = false, hasPasskey = false))
        composeTestRule.onAllNodesWithText("Regenerate recovery codes").assertCountEquals(0)
    }

    @Test
    fun `the regenerate prompt offers a passkey proof when one can answer`() {
        var passkey = false
        setContent(
            state = TwoFactorUiState(
                loading = false,
                enabled = false,
                hasPasskey = true,
                passkeysAvailable = true,
                prompt = TwoFactorPrompt.REGENERATE,
            ),
            onPromptWithPasskey = { passkey = true },
        )
        composeTestRule.onNodeWithText("Verify with a passkey instead").performClick()
        assertTrue(passkey)
    }

    @Test
    fun `the disable prompt never offers a passkey proof`() {
        setContent(
            state = TwoFactorUiState(
                loading = false,
                enabled = true,
                hasPasskey = true,
                passkeysAvailable = true,
                prompt = TwoFactorPrompt.DISABLE,
            ),
        )
        composeTestRule.onAllNodesWithText("Verify with a passkey instead").assertCountEquals(0)
    }

    @Test
    fun `the recovery codes dialog renders inside the screen and can be dismissed`() {
        var dismissed = false
        setContent(
            state = TwoFactorUiState(
                loading = false,
                enabled = true,
                recoveryCodes = listOf("AAAAA-BBBBB-CCCCC"),
            ),
            onDismissRecoveryCodes = { dismissed = true },
        )
        composeTestRule.onAllNodesWithText("AAAAA-BBBBB-CCCCC").assertCountEquals(1)
        composeTestRule.onNodeWithText("Done").performClick()
        assertTrue(dismissed)
    }

    // --- dialogs ---

    @Test
    fun `setup dialog shows QR manual key and forwards the entered code`() {
        var confirmed: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                EnrollmentSetupDialog(
                    setup = TwoFactorSetupResponse(
                        secret = "JBSWY3DPEHPK3PXP",
                        otpauthUrl = "otpauth://totp/Example:alice?secret=JBSWY3DPEHPK3PXP&issuer=Example",
                    ),
                    busy = false,
                    error = null,
                    errorRes = null,
                    onConfirm = { confirmed = it },
                    onDismiss = {},
                )
            }
        }

        composeTestRule.onNodeWithText("Manual setup key").assertIsDisplayed()
        composeTestRule.onNodeWithText("JBSWY3DPEHPK3PXP").assertIsDisplayed()
        composeTestRule.onNodeWithText("Enable and continue").assertIsNotEnabled()
        composeTestRule.onNodeWithText("Verification code").performTextInput("123456")
        composeTestRule.onNodeWithText("Enable and continue").performClick()

        assertEquals("123456", confirmed)
    }

    @Test
    fun `recovery codes dialog lists the codes and the done button dismisses`() {
        var done = false
        composeTestRule.setContent {
            MycorrhizalTheme {
                RecoveryCodesDialog(
                    codes = listOf("AAAAA-BBBBB-CCCCC", "DDDDD-EEEEE-FFFFF"),
                    onDone = { done = true },
                )
            }
        }

        composeTestRule.onAllNodesWithText("AAAAA-BBBBB-CCCCC").assertCountEquals(1)
        composeTestRule.onAllNodesWithText("DDDDD-EEEEE-FFFFF").assertCountEquals(1)
        composeTestRule.onNodeWithText("Copy all codes").assertIsDisplayed()
        composeTestRule.onNodeWithText("Done").performClick()
        assertTrue(done)
    }

    @Test
    fun `disable prompt confirms with a live code`() {
        var confirmed: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                CodePromptDialog(
                    prompt = TwoFactorPrompt.DISABLE,
                    busy = false,
                    error = null,
                    errorRes = null,
                    onConfirm = { confirmed = it },
                    onDismiss = {},
                )
            }
        }

        composeTestRule.onNodeWithText("Disable two-factor authentication").assertIsDisplayed()
        composeTestRule.onNodeWithText("Verification code").performTextInput("654321")
        composeTestRule.onNodeWithText("Confirm").performClick()

        assertEquals("654321", confirmed)
    }

    @Test
    fun `regenerate prompt confirms with a live code`() {
        var confirmed: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                CodePromptDialog(
                    prompt = TwoFactorPrompt.REGENERATE,
                    busy = false,
                    error = null,
                    errorRes = null,
                    onConfirm = { confirmed = it },
                    onDismiss = {},
                )
            }
        }

        composeTestRule.onNodeWithText("Regenerate recovery codes").assertIsDisplayed()
        composeTestRule.onNodeWithText("Verification code").performTextInput("123456")
        composeTestRule.onNodeWithText("Confirm").performClick()

        assertEquals("123456", confirmed)
    }

    // --- issue #1337: proof before enabling TOTP on a passkey account ---

    @Test
    fun `the proof dialog submits a typed code and needs one`() {
        var submitted: String? = null
        setContent(
            state = TwoFactorUiState(loading = false, enabled = false, hasPasskey = true, proofPrompt = true),
            onSubmitSetupProofCode = { submitted = it },
        )
        composeTestRule.onNodeWithText("Verify it's you").assertIsDisplayed()
        composeTestRule.onNodeWithText("Verify and continue").assertIsNotEnabled()
        composeTestRule.onNodeWithText("Verification code").performTextInput("AAAAA-BBBBB-CCCCC")
        composeTestRule.onNodeWithText("Verify and continue").assertIsEnabled().performClick()
        assertEquals("AAAAA-BBBBB-CCCCC", submitted)
    }

    @Test
    fun `the passkey route is offered only when the gate is open`() {
        var used = false
        setContent(
            state = TwoFactorUiState(
                loading = false,
                enabled = false,
                hasPasskey = true,
                passkeysAvailable = true,
                proofPrompt = true,
            ),
            onSetupProofWithPasskey = { used = true },
        )
        composeTestRule.onNodeWithText("Verify with a passkey instead").performClick()
        assertTrue(used)
    }

    @Test
    fun `no passkey route without the gate and the proof dialog can be dismissed`() {
        var dismissed = false
        setContent(
            state = TwoFactorUiState(loading = false, enabled = false, hasPasskey = true, proofPrompt = true),
            onDismissSetupProof = { dismissed = true },
        )
        composeTestRule.onNodeWithText("Verify with a passkey instead").assertDoesNotExist()
        composeTestRule.onNodeWithText("Cancel").performClick()
        assertTrue(dismissed)
    }

    @Test
    fun `a rejected proof is shown inside the dialog and the busy label blocks it`() {
        setContent(
            state = TwoFactorUiState(
                loading = false,
                enabled = false,
                hasPasskey = true,
                proofPrompt = true,
                busy = true,
                error = "Account temporarily locked",
            ),
        )
        composeTestRule.onNodeWithText("Account temporarily locked").assertIsDisplayed()
        composeTestRule.onNodeWithText("Please wait...").assertIsNotEnabled()
    }
}
