package com.mycorrhizal.crm.feature.settings

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import com.mycorrhizal.crm.model.network.ApiToken
import com.mycorrhizal.crm.model.network.ApiTokenCreateResponse
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
class ApiTokensScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun token(
        id: Int = 1,
        name: String = "CI token",
        scope: String = "full",
        revokedAt: String? = null,
        expiresAt: String? = null,
    ) = ApiToken(id = id, name = name, createdAt = "2026-01-01T00:00:00Z", scope = scope, revokedAt = revokedAt, expiresAt = expiresAt)

    @Test
    fun `an active token row shows its scope and status with rotate and revoke actions`() {
        var rotated = false
        var revoked = false
        composeTestRule.setContent {
            MycorrhizalTheme {
                ApiTokenRow(
                    token = token(name = "Sync script", scope = "carddav"),
                    rotating = false,
                    revoking = false,
                    onRotate = { rotated = true },
                    onRevoke = { revoked = true },
                )
            }
        }

        composeTestRule.onNodeWithText("Sync script").assertIsDisplayed()
        composeTestRule.onNodeWithText("CardDAV only").assertIsDisplayed()
        composeTestRule.onNodeWithText("Active").assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("Rotate Sync script").performClick()
        composeTestRule.onNodeWithContentDescription("Revoke Sync script").performClick()
        assert(rotated && revoked)
    }

    @Test
    fun `a revoked token is labeled and has no rotate or revoke action`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ApiTokenRow(
                    token = token(name = "Old token", revokedAt = "2026-01-02T00:00:00Z"),
                    rotating = false,
                    revoking = false,
                    onRotate = {},
                    onRevoke = {},
                )
            }
        }

        composeTestRule.onNodeWithText("Old token").assertIsDisplayed()
        composeTestRule.onNodeWithText("Revoked").assertIsDisplayed()
    }

    @Test
    fun `an expired token is labeled Expired even though revokedAt is null`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ApiTokenRow(
                    token = token(name = "Stale token", expiresAt = "2020-01-01T00:00:00Z"),
                    rotating = false,
                    revoking = false,
                    onRotate = {},
                    onRevoke = {},
                )
            }
        }

        composeTestRule.onNodeWithText("Stale token").assertIsDisplayed()
        composeTestRule.onNodeWithText("Expired").assertIsDisplayed()
    }

    @Test
    fun `create dialog blocks confirm until a name is entered`() {
        var confirmedName: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                CreateApiTokenDialog(
                    isSaving = false,
                    onConfirm = { name, _, _ -> confirmedName = name },
                    onDismiss = {},
                )
            }
        }

        composeTestRule.onNodeWithText("Create Token").assertIsNotEnabled()
        composeTestRule.onNodeWithText("Token Name").performTextInput("New token")
        composeTestRule.onNodeWithText("Create Token").performClick()

        assertEquals("New token", confirmedName)
    }

    @Test
    fun `create dialog defaults to 90 days and full access`() {
        var days: Int? = null
        var scope: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                CreateApiTokenDialog(
                    isSaving = false,
                    onConfirm = { _, expiresInDays, s -> days = expiresInDays; scope = s },
                    onDismiss = {},
                )
            }
        }

        composeTestRule.onNodeWithText("Token Name").performTextInput("New token")
        composeTestRule.onNodeWithText("Create Token").performClick()

        assertEquals(90, days)
        assertEquals("full", scope)
    }

    @Test
    fun `create dialog scope selection follows the chosen chip`() {
        var scope: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                CreateApiTokenDialog(
                    isSaving = false,
                    onConfirm = { _, _, s -> scope = s },
                    onDismiss = {},
                )
            }
        }

        composeTestRule.onNodeWithText("Token Name").performTextInput("New token")
        composeTestRule.onNodeWithText("CardDAV only").performClick()
        composeTestRule.onNodeWithText("Create Token").performClick()

        assertEquals("carddav", scope)
    }

    @Test
    fun `revealed token dialog shows create copy for a fresh token`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                RevealedTokenDialog(
                    revealed = ApiTokenCreateResponse(id = 1, name = "New", scope = "full", token = "mcrh_live_abc123"),
                    isRotation = false,
                    onDismiss = {},
                )
            }
        }

        composeTestRule.onNodeWithText("Token Created").assertIsDisplayed()
        composeTestRule.onNodeWithText("mcrh_live_abc123").assertIsDisplayed()
    }

    @Test
    fun `revealed token dialog shows rotation copy after a rotate`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                RevealedTokenDialog(
                    revealed = ApiTokenCreateResponse(id = 1, name = "New", scope = "full", token = "mcrh_live_rotated"),
                    isRotation = true,
                    onDismiss = {},
                )
            }
        }

        composeTestRule.onNodeWithText("Token Rotated").assertIsDisplayed()
        composeTestRule.onNodeWithText("mcrh_live_rotated").assertIsDisplayed()
    }

    @Test
    fun `revealed token dialog dismiss button clears it`() {
        var dismissed = false
        composeTestRule.setContent {
            MycorrhizalTheme {
                RevealedTokenDialog(
                    revealed = ApiTokenCreateResponse(id = 1, name = "New", scope = "full", token = "s3cret"),
                    isRotation = false,
                    onDismiss = { dismissed = true },
                )
            }
        }

        composeTestRule.onNodeWithText("Done, I saved it").performClick()
        assert(dismissed)
    }
}
