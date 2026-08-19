package com.mycorrhizal.crm.feature.tracking

import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.domain.repository.ActivityRepository
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issue #207: a failed save used to report its reason as a loose Text below
 * the note field -- unassociated with the title field it was actually about,
 * silent (no live region), and never moved focus.
 */
@OptIn(kotlinx.coroutines.ExperimentalCoroutinesApi::class)
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class QuickCaptureSheetTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun `a blank-title error is linked to the title field, not a loose Text`() = runTest {
        val repo = mockk<ActivityRepository>(relaxed = true)
        val scope = CoroutineScope(StandardTestDispatcher(testScheduler))
        val state = QuickCaptureFormState(
            activityRepository = repo,
            scope = scope,
            prefill = QuickCapturePrefillFactory.forCall(null, "2026-08-18T12:00:00Z"),
            blankTitleMessage = "Title is required",
        )
        // forCall() defaults the title to "Call" for an unresolved caller --
        // clear it so save() hits the blank-title validation this test needs.
        state.onTitleChange("   ")
        composeTestRule.setContent {
            MycorrhizalTheme {
                QuickCaptureSheet(state = state, onDismiss = {})
            }
        }

        composeTestRule.onNodeWithText("Save").performClick()
        advanceUntilIdle()
        composeTestRule.waitForIdle()

        // The title field itself carries the error -- semantics { error(...) }
        // is what makes TalkBack say "invalid entry" with the reason.
        composeTestRule.onNodeWithText("Title")
            .assert(SemanticsMatcher.keyIsDefined(SemanticsProperties.Error))
        // The message is shown as the field's own supportingText, reachable
        // by its text, not as a disconnected Text elsewhere in the sheet.
        composeTestRule.onNodeWithText("Title is required").assertIsDisplayed()
    }

    @Test
    fun `save button announces saving via stateDescription`() = runTest {
        val repo = mockk<ActivityRepository>()
        coEvery { repo.create(any()) } coAnswers {
            kotlinx.coroutines.delay(1_000)
            Result.success(Activity(id = 1, title = "Call"))
        }
        val scope = CoroutineScope(StandardTestDispatcher(testScheduler))
        val state = QuickCaptureFormState(
            activityRepository = repo,
            scope = scope,
            prefill = QuickCapturePrefillFactory.forCall(null, "2026-08-18T12:00:00Z"),
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                QuickCaptureSheet(state = state, onDismiss = {})
            }
        }

        composeTestRule.onNodeWithText("Save").performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Save")
            .assert(SemanticsMatcher.expectValue(SemanticsProperties.StateDescription, "Saving"))
    }

    @Test
    fun `focusing a field cancels the display timeout`() = runTest {
        // #201 (WCAG 2.2.1): a TalkBack or switch-access user navigates the
        // sheet by focusing fields, which may not involve typing for a long
        // time — the very first focus into the form must cancel the overlay's
        // auto-dismiss timer. The form state here carries no onFirstInteraction
        // so only the sheet's focus path can have fired it.
        var interactions = 0
        val repo = mockk<ActivityRepository>(relaxed = true)
        val scope = CoroutineScope(StandardTestDispatcher(testScheduler))
        val state = QuickCaptureFormState(
            activityRepository = repo,
            scope = scope,
            prefill = QuickCapturePrefillFactory.forCall(null, "2026-08-18T12:00:00Z"),
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                QuickCaptureSheet(
                    state = state,
                    onDismiss = {},
                    onFirstInteraction = { interactions++ },
                )
            }
        }

        composeTestRule.onNodeWithText("Title").performClick()
        composeTestRule.waitForIdle()

        assertEquals(1, interactions)
    }
}
