package com.mycorrhizal.crm.feature.circles

import android.content.Context
import androidx.annotation.StringRes
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertIsSelected
import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextClearance
import androidx.compose.ui.test.performTextInput
import androidx.lifecycle.SavedStateHandle
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.domain.repository.CircleDetail
import com.mycorrhizal.crm.domain.repository.CircleRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.CircleMember
import com.mycorrhizal.crm.model.network.CircleTimelineItem
import com.mycorrhizal.crm.model.network.CircleTimelineMember
import com.mycorrhizal.crm.model.network.CircleTimelinePage
import com.mycorrhizal.crm.model.network.ErrorReason
import com.mycorrhizal.crm.model.network.ErrorReasonCode
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.a11y.assertAccessibleSemantics
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/** Issue #2522: description, colour and the member timeline on the circle detail screen. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class CircleDetailMetadataScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val circleRepository = mockk<CircleRepository>()
    private val contactRepository = mockk<ContactRepository>()

    private fun str(@StringRes res: Int, vararg args: Any): String =
        ApplicationProvider.getApplicationContext<Context>().getString(res, *args)

    private val dana = CircleTimelineMember(id = 7, firstname = "Dana", lastname = "Lee")

    private fun setScreen(
        circle: Circle = Circle(
            id = "c1",
            name = "Family",
            description = "Close ones",
            color = "teal"
        ),
        members: List<CircleMember> = listOf(CircleMember(1, "c1", "uid-dana")),
        timeline: Result<CircleTimelinePage> = Result.success(CircleTimelinePage(memberCount = 1)),
        onNavigate: (Int) -> Unit = {}
    ) {
        coEvery { circleRepository.getWithMembers("c1") } returns
            Result.success(CircleDetail(circle, members))
        coEvery { circleRepository.getTimeline("c1", null) } returns timeline
        coEvery { contactRepository.resolveByUid(any()) } returns Result.success(emptyMap())
        val vm = CircleDetailViewModel(
            circleRepository,
            contactRepository,
            SavedStateHandle(mapOf("circleId" to "c1"))
        )
        composeTestRule.setContent {
            MycorrhizalTheme(darkTheme = false) {
                CircleDetailScreen(onBack = {}, onNavigateToContact = onNavigate, viewModel = vm)
            }
        }
    }

    @Test
    fun `the description is shown on the detail`() {
        setScreen()

        composeTestRule.onNodeWithText("Close ones").assertIsDisplayed()
    }

    @Test
    fun `an empty timeline shows the empty state and the member count`() {
        setScreen(timeline = Result.success(CircleTimelinePage(memberCount = 1)))

        composeTestRule.onNodeWithText(str(R.string.circles_timeline_title)).assertIsDisplayed()
        composeTestRule.onNodeWithText(str(R.string.circles_timeline_empty)).assertIsDisplayed()
        composeTestRule.onNodeWithText(
            ApplicationProvider.getApplicationContext<Context>().resources
                .getQuantityString(R.plurals.circles_member_count, 1, 1)
        ).assertIsDisplayed()
    }

    @Test
    fun `timeline entries show their summary, type, and member names that open the contact`() {
        var opened: Int? = null
        setScreen(
            timeline = Result.success(
                CircleTimelinePage(
                    items = listOf(
                        CircleTimelineItem(
                            type = "note",
                            id = "11",
                            date = "2026-09-01T10:00:00Z",
                            data = mapOf("content" to "Loves climbing"),
                            members = listOf(dana)
                        )
                    ),
                    memberCount = 1
                )
            ),
            onNavigate = { opened = it }
        )

        composeTestRule.onNodeWithText("Loves climbing").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText(str(R.string.circles_timeline_empty)).assertDoesNotExist()
        composeTestRule.onNodeWithText("Dana Lee").performScrollTo().performClick()
        assertEquals(7, opened)
    }

    @Test
    fun `load more appears with a cursor and requests the next page`() {
        coEvery { circleRepository.getTimeline("c1", "n2") } returns
            Result.success(CircleTimelinePage())
        setScreen(
            timeline = Result.success(
                CircleTimelinePage(
                    items = listOf(
                        CircleTimelineItem(
                            type = "note",
                            id = "1",
                            date = "2026-09-01T10:00:00Z",
                            data = mapOf("content" to "First")
                        )
                    ),
                    nextCursor = "n2",
                    memberCount = 1
                )
            )
        )

        composeTestRule.onNodeWithText(str(R.string.action_load_more))
            .performScrollTo().performClick()

        coVerify(exactly = 1) { circleRepository.getTimeline("c1", "n2") }
    }

    @Test
    fun `a timeline failure shows a localized inline error with retry`() {
        setScreen(timeline = Result.failure(ApiError.Server(500, "boom")))

        composeTestRule.onNodeWithText(str(R.string.circles_timeline_title)).assertIsDisplayed()
        composeTestRule.onNodeWithText(
            str(R.string.action_retry)
        ).performScrollTo().assertIsDisplayed()
        // Members still render: the timeline failure is scoped to its section.
        composeTestRule.onNodeWithText(str(R.string.contact_unknown)).assertIsDisplayed()
    }

    @Test
    fun `the edit dialog opens prefilled with name, description and colour`() {
        setScreen()

        composeTestRule.onNodeWithContentDescription(str(R.string.circles_edit)).performClick()

        composeTestRule.onNodeWithTag(CIRCLE_EDIT_NAME_TAG).assertTextContains("Family")
        composeTestRule.onNodeWithTag(CIRCLE_EDIT_DESCRIPTION_TAG).assertTextContains("Close ones")
        composeTestRule.onNodeWithContentDescription(str(R.string.circles_color_teal))
            .assertIsSelected()
        composeTestRule.onNodeWithText(
            "10/${com.mycorrhizal.crm.model.network.CircleColors.MAX_DESCRIPTION}"
        ).assertIsDisplayed()
        composeTestRule.onNodeWithText(str(R.string.action_save)).assertIsNotEnabled()
    }

    @Test
    fun `saving the edit dialog sends the new description and colour`() {
        setScreen()
        coEvery { circleRepository.update("c1", "Family", "Dear ones", "purple") } returns
            Result.success(
                Circle(id = "c1", name = "Family", description = "Dear ones", color = "purple")
            )

        composeTestRule.onNodeWithContentDescription(str(R.string.circles_edit)).performClick()
        composeTestRule.onNodeWithTag(CIRCLE_EDIT_DESCRIPTION_TAG).performTextClearance()
        composeTestRule.onNodeWithTag(CIRCLE_EDIT_DESCRIPTION_TAG).performTextInput("Dear ones")
        composeTestRule.onNodeWithContentDescription(str(R.string.circles_color_purple))
            .performClick()
        composeTestRule.onNodeWithText(str(R.string.action_save)).assertIsEnabled().performClick()

        coVerify(exactly = 1) { circleRepository.update("c1", "Family", "Dear ones", "purple") }
        composeTestRule.onNodeWithText("Dear ones").assertIsDisplayed()
    }

    @Test
    fun `choosing None clears the colour`() {
        setScreen()
        coEvery { circleRepository.update("c1", "Family", "Close ones", "") } returns
            Result.success(Circle(id = "c1", name = "Family", description = "Close ones"))

        composeTestRule.onNodeWithContentDescription(str(R.string.circles_edit)).performClick()
        composeTestRule.onNodeWithContentDescription(str(R.string.circles_color_none))
            .performClick()
        composeTestRule.onNodeWithText(str(R.string.action_save)).performClick()

        coVerify(exactly = 1) { circleRepository.update("c1", "Family", "Close ones", "") }
    }

    @Test
    fun `a duplicate name shows the inline error and keeps the dialog open`() {
        setScreen()
        coEvery { circleRepository.update(any(), any(), any(), any()) } returns Result.failure(
            ApiError.Client(
                409,
                "exists",
                reason = ErrorReason(ErrorReasonCode.CIRCLE_ALREADY_EXISTS, emptyMap())
            )
        )

        composeTestRule.onNodeWithContentDescription(str(R.string.circles_edit)).performClick()
        composeTestRule.onNodeWithTag(CIRCLE_EDIT_NAME_TAG).performTextInput(" 2")
        composeTestRule.onNodeWithText(str(R.string.action_save)).performClick()

        composeTestRule.onNodeWithText(str(R.string.error_reason_circle_already_exists))
            .assertIsDisplayed()
    }

    @Test
    fun `circle detail with the timeline has no accessibility violations`() {
        setScreen()

        composeTestRule.assertAccessibleSemantics()
    }
}
