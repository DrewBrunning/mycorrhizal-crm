package com.mycorrhizal.crm.feature.shares

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.ShareGrantRepository
import com.mycorrhizal.crm.model.network.ErrorReason
import com.mycorrhizal.crm.model.network.ErrorReasonCode
import com.mycorrhizal.crm.model.network.ShareGrantContent
import com.mycorrhizal.crm.model.network.SharedActivity
import com.mycorrhizal.crm.model.network.SharedCircle
import com.mycorrhizal.crm.model.network.SharedCircleMember
import com.mycorrhizal.crm.model.network.SharedNote
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.a11y.assertAccessibleSemantics
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/** Each grant type renders only its allowlisted fields; a failed read never shows stale content. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ShareGrantContentScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val repository = mockk<ShareGrantRepository>()

    private fun screen(sections: String = "emails") {
        val vm = ShareGrantContentViewModel(
            repository,
            SavedStateHandle(mapOf("grantId" to "g1", "sections" to sections))
        )
        composeTestRule.setContent {
            MycorrhizalTheme { ShareGrantContentScreen(onBack = {}, viewModel = vm) }
        }
    }

    private fun content(
        type: String,
        contact: Map<String, Any?>? = null,
        note: SharedNote? = null,
        activity: SharedActivity? = null,
        circle: SharedCircle? = null
    ) = ShareGrantContent("g1", type, "dana", contact, note, activity, circle)

    @Test
    fun `a contact renders its identity and only the granted sections`() {
        coEvery { repository.content("g1") } returns Result.success(
            content(
                "contact",
                contact = mapOf(
                    "uid" to "SECRET-UID",
                    "name" to mapOf("full" to "Dana White"),
                    "emails" to mapOf("e" to mapOf("address" to "dana@example.com")),
                    "phones" to mapOf("p" to mapOf("number" to "555-0100"))
                )
            )
        )
        screen(sections = "emails")

        composeTestRule.onNodeWithText("Dana White").assertIsDisplayed()
        composeTestRule.onNodeWithText("Shared by dana").assertIsDisplayed()
        composeTestRule.onNodeWithText("Emails").assertIsDisplayed()
        composeTestRule.onNodeWithText("dana@example.com").assertIsDisplayed()
        // Not granted, not allowlisted: never on screen.
        composeTestRule.onNodeWithText("555-0100").assertDoesNotExist()
        composeTestRule.onNodeWithText("SECRET-UID").assertDoesNotExist()
    }

    @Test
    fun `a note renders its text and date only`() {
        coEvery { repository.content("g1") } returns Result.success(
            content("note", note = SharedNote("Met at the fair", "2026-11-01T00:00:00Z"))
        )
        screen()
        composeTestRule.onNodeWithText("Met at the fair").assertIsDisplayed()
        composeTestRule.onNodeWithText("01.11.2026").assertIsDisplayed()
    }

    @Test
    fun `an activity renders title description location and dates`() {
        coEvery { repository.content("g1") } returns Result.success(
            content(
                "activity",
                activity = SharedActivity(
                    title = "Picnic",
                    description = "Bring snacks",
                    location = "Park",
                    date = "2026-11-02T00:00:00Z",
                    endDate = "2026-11-03T00:00:00Z",
                    type = "meal"
                )
            )
        )
        screen()
        composeTestRule.onNodeWithText("Picnic").assertIsDisplayed()
        composeTestRule.onNodeWithText("Bring snacks").assertIsDisplayed()
        composeTestRule.onNodeWithText("Park").assertIsDisplayed()
        composeTestRule.onNodeWithText("02.11.2026").assertIsDisplayed()
        composeTestRule.onNodeWithText("03.11.2026").assertIsDisplayed()
    }

    @Test
    fun `a circle renders metadata and live member names with a truncation note`() {
        coEvery { repository.content("g1") } returns Result.success(
            content(
                "circle",
                circle = SharedCircle(
                    name = "Family",
                    description = "Close family",
                    color = "#ff0000",
                    memberCount = 120,
                    members = listOf(SharedCircleMember("Ann Lee"), SharedCircleMember("Bo Kim")),
                    membersTruncated = true
                )
            )
        )
        screen()
        composeTestRule.onNodeWithText("Family").assertIsDisplayed()
        composeTestRule.onNodeWithText("120 members").assertIsDisplayed()
        composeTestRule.onNodeWithText("Ann Lee").assertIsDisplayed()
        composeTestRule.onNodeWithText("Only some members are shown").assertIsDisplayed()
    }

    @Test
    fun `a failed read shows a retryable error with the offline hint and no content`() {
        coEvery { repository.content("g1") } returnsMany listOf(
            Result.failure(ApiError.Server(503, "down")),
            Result.success(content("note", note = SharedNote("Back again", null)))
        )
        screen()

        composeTestRule.onNodeWithText("Retry").assertIsDisplayed()
        composeTestRule.onNodeWithText(
            "Shared content is never stored on this device, so it cannot be shown offline."
        ).assertIsDisplayed()
        composeTestRule.onNodeWithText("Back again").assertDoesNotExist()

        composeTestRule.onNodeWithText("Retry").performClick()
        composeTestRule.onNodeWithText("Back again").assertIsDisplayed()
        coVerify(exactly = 2) { repository.content("g1") }
    }

    @Test
    fun `a revoked grant is simply unavailable`() {
        coEvery { repository.content("g1") } returns Result.failure(
            ApiError.Client(
                404,
                "Share grant not found",
                reason = ErrorReason(ErrorReasonCode.SHARE_GRANT_NOT_FOUND, emptyMap())
            )
        )
        screen()
        composeTestRule.onNodeWithText("This share is no longer available").assertIsDisplayed()
        composeTestRule.onNodeWithText("Retry").assertDoesNotExist()
    }

    @Test
    fun `the read view has no accessibility violations`() {
        coEvery { repository.content("g1") } returns Result.success(
            content(
                "contact",
                contact = mapOf(
                    "name" to mapOf("full" to "Dana White"),
                    "emails" to mapOf("e" to mapOf("address" to "dana@example.com"))
                )
            )
        )
        screen()
        composeTestRule.assertAccessibleSemantics()
    }
}
