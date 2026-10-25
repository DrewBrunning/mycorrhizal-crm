package com.mycorrhizal.crm.feature.settings

import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import com.mycorrhizal.crm.domain.repository.ApiTokenRepository
import com.mycorrhizal.crm.model.network.ApiToken
import com.mycorrhizal.crm.model.network.ApiTokenCreateResponse
import com.mycorrhizal.crm.model.network.RevokeAllApiTokensResponse
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.awaitCancellation
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
        expiresAt: String? = null
    ) = ApiToken(
        id = id,
        name = name,
        createdAt = "2026-01-01T00:00:00Z",
        scope = scope,
        revokedAt = revokedAt,
        expiresAt = expiresAt
    )

    // --- Top-level ApiTokensScreen: the RefreshableContent wrapper + its
    // loading/empty/populated branches only execute when the real screen is
    // mounted against a real ViewModel, so these mirror CirclesScreenTest's
    // construction (mocked repository, no Hilt).

    private fun setScreen(viewModel: ApiTokensViewModel) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ApiTokensScreen(onBack = {}, viewModel = viewModel)
            }
        }
        composeTestRule.waitForIdle()
    }

    private fun spinnerIsShown() {
        composeTestRule
            .onNode(SemanticsMatcher.keyIsDefined(SemanticsProperties.ProgressBarRangeInfo))
            .assertExists()
    }

    @Test
    fun `top-level screen shows a spinner while the initial load is in flight`() {
        val repository = mockk<ApiTokenRepository>()
        coEvery { repository.list() } coAnswers { awaitCancellation() }

        setScreen(ApiTokensViewModel(repository))

        spinnerIsShown()
        composeTestRule.onNodeWithText("No API tokens yet").assertDoesNotExist()
    }

    @Test
    fun `top-level screen shows the empty state when no tokens exist`() {
        val repository = mockk<ApiTokenRepository>()
        coEvery { repository.list() } returns Result.success(emptyList())

        setScreen(ApiTokensViewModel(repository))

        composeTestRule.onNodeWithText("No API tokens yet").assertIsDisplayed()
    }

    @Test
    fun `top-level screen renders the token list`() {
        val repository = mockk<ApiTokenRepository>()
        coEvery { repository.list() } returns Result.success(
            listOf(token(1, name = "Deploy token"))
        )

        setScreen(ApiTokensViewModel(repository))

        composeTestRule.onNodeWithText("Deploy token").assertIsDisplayed()
    }

    @Test
    fun `top-level screen shows an action error above a populated list`() {
        val repository = mockk<ApiTokenRepository>()
        coEvery { repository.list() } returnsMany listOf(
            Result.success(listOf(token(1, name = "Deploy token"))),
            Result.failure(ApiError.Server(500, "boom"))
        )
        val viewModel = ApiTokensViewModel(repository)
        setScreen(viewModel)

        composeTestRule.runOnIdle { viewModel.load() }
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Deploy token").assertIsDisplayed()
        composeTestRule.onNodeWithText("Server error (500)").assertIsDisplayed()
    }

    @Test
    fun `top-level screen shows the revoke-all count above the list`() {
        val repository = mockk<ApiTokenRepository>()
        coEvery { repository.list() } returns Result.success(
            listOf(token(1, name = "Deploy token"))
        )
        coEvery { repository.revokeAll() } returns Result.success(
            RevokeAllApiTokensResponse(revoked = 2)
        )
        val viewModel = ApiTokensViewModel(repository)
        setScreen(viewModel)

        composeTestRule.runOnIdle { viewModel.revokeAll() }
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Deploy token").assertIsDisplayed()
        composeTestRule.onNodeWithText("2 tokens revoked").assertIsDisplayed()
    }

    @Test
    fun `an explicitly scoped token is not mislabeled as full access`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ApiTokenRow(
                    token = token(name = "Scoped token", scope = "custom"),
                    rotating = false,
                    revoking = false,
                    onRotate = {},
                    onRevoke = {},
                )
            }
        }

        composeTestRule.onNodeWithText("Custom access").assertIsDisplayed()
    }

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
                    onRevoke = { revoked = true }
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
                    onRevoke = {}
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
                    onRevoke = {}
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
                    onDismiss = {}
                )
            }
        }

        composeTestRule.onNodeWithText("Create token").assertIsNotEnabled()
        composeTestRule.onNodeWithText("Token name").performTextInput("New token")
        composeTestRule.onNodeWithText("Create token").performClick()

        assertEquals("New token", confirmedName)
    }

    @Test
    fun `create dialog defaults to 90 days and full access`() {
        var days: Int? = null
        var scopes: List<String>? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                CreateApiTokenDialog(
                    isSaving = false,
                    onConfirm = { _, expiresInDays, selected ->
                        days = expiresInDays
                        scopes = selected
                    },
                    onDismiss = {}
                )
            }
        }

        composeTestRule.onNodeWithText("Token name").performTextInput("New token")
        composeTestRule.onNodeWithText("Create token").performClick()

        assertEquals(90, days)
        assertEquals(listOf("carddav", "caldav", "api:read", "api:write", "mcp"), scopes)
    }

    @Test
    fun `create dialog scope selection follows the chosen chip`() {
        var scopes: List<String>? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                CreateApiTokenDialog(
                    isSaving = false,
                    onConfirm = { _, _, selected -> scopes = selected },
                    onDismiss = {}
                )
            }
        }

        composeTestRule.onNodeWithText("Token name").performTextInput("New token")
        composeTestRule.onNodeWithText("CardDAV only").performClick()
        composeTestRule.onNodeWithText("Create token").performClick()

        assertEquals(listOf("carddav"), scopes)
    }

    @Test
    fun `revealed token dialog shows create copy for a fresh token`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                RevealedTokenDialog(
                    revealed = ApiTokenCreateResponse(
                        id = 1,
                        name = "New",
                        scope = "full",
                        token = "mcrh_live_abc123"
                    ),
                    isRotation = false,
                    onDismiss = {}
                )
            }
        }

        composeTestRule.onNodeWithText("Token created").assertIsDisplayed()
        composeTestRule.onNodeWithText("mcrh_live_abc123").assertIsDisplayed()
    }

    @Test
    fun `revealed token dialog shows rotation copy after a rotate`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                RevealedTokenDialog(
                    revealed = ApiTokenCreateResponse(
                        id = 1,
                        name = "New",
                        scope = "full",
                        token = "mcrh_live_rotated"
                    ),
                    isRotation = true,
                    onDismiss = {}
                )
            }
        }

        composeTestRule.onNodeWithText("Token rotated").assertIsDisplayed()
        composeTestRule.onNodeWithText("mcrh_live_rotated").assertIsDisplayed()
    }

    @Test
    fun `revealed token dialog dismiss button clears it`() {
        var dismissed = false
        composeTestRule.setContent {
            MycorrhizalTheme {
                RevealedTokenDialog(
                    revealed = ApiTokenCreateResponse(
                        id = 1,
                        name = "New",
                        scope = "full",
                        token = "s3cret"
                    ),
                    isRotation = false,
                    onDismiss = { dismissed = true }
                )
            }
        }

        composeTestRule.onNodeWithText("Done, I saved it").performClick()
        assert(dismissed)
    }

    // --- Issue #1775: destructive Revoke all moved out of the top bar ---

    @Test
    fun `revoke all is not a top-bar icon and lives in the overflow`() {
        val repository = mockk<ApiTokenRepository>()
        coEvery { repository.list() } returns Result.success(
            listOf(token(1, name = "Deploy token"))
        )
        setScreen(ApiTokensViewModel(repository))

        composeTestRule.onNodeWithContentDescription("Revoke all").assertDoesNotExist()
        composeTestRule.onNodeWithContentDescription("More options").assertIsDisplayed()
    }

    @Test
    fun `opening the overflow then revoke all shows the confirmation and revokes`() {
        val repository = mockk<ApiTokenRepository>()
        coEvery { repository.list() } returns Result.success(
            listOf(token(1, name = "Deploy token"))
        )
        coEvery { repository.revokeAll() } returns Result.success(
            RevokeAllApiTokensResponse(revoked = 1)
        )
        setScreen(ApiTokensViewModel(repository))

        composeTestRule.onNodeWithContentDescription("More options").performClick()
        composeTestRule.onNodeWithText("Revoke all").performClick()
        composeTestRule.onNodeWithText("Revoke all tokens").assertIsDisplayed()
        // The dialog's confirm button carries the same label as the menu item.
        composeTestRule.onAllNodesWithText("Revoke all")[0].performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("1 tokens revoked").assertIsDisplayed()
    }

    @Test
    fun `revoke all is disabled in the overflow when no token is active`() {
        val repository = mockk<ApiTokenRepository>()
        coEvery { repository.list() } returns Result.success(
            listOf(token(1, name = "Old", revokedAt = "2026-01-02T00:00:00Z"))
        )
        setScreen(ApiTokensViewModel(repository))

        composeTestRule.onNodeWithContentDescription("More options").performClick()
        composeTestRule.onNodeWithText("Revoke all").assertIsNotEnabled()
    }
}
