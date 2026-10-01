package com.mycorrhizal.crm.feature.timeline

import androidx.compose.foundation.layout.padding
import androidx.compose.material3.SnackbarHostState
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import com.mycorrhizal.crm.ui.components.FormScaffold
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotDisplayed
import androidx.compose.ui.test.assertIsToggleable
import androidx.compose.ui.test.hasScrollAction
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTouchInput
import androidx.compose.ui.test.swipeDown
import androidx.compose.ui.test.swipeUp
import androidx.compose.ui.test.performScrollTo
import com.mycorrhizal.crm.model.network.ReminderRecurrence
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import java.time.LocalDate

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], qualifiers = "w480dp-h2000dp")
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ReminderFormScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(
        state: ReminderFormState,
        onRemindAtChange: (String) -> Unit = {},
        onRecurrenceChange: (String) -> Unit = {},
        onReoccurFromCompletionChange: (Boolean) -> Unit = {},
        onSave: () -> Unit = {},
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                FormScaffold(
                    title = "Form",
                    onBack = {},
                    saveLabel = if (state.isEdit) "Save changes" else "Create reminder",
                    isSaving = state.isSaving,
                    onSave = onSave,
                    snackbarHostState = remember { SnackbarHostState() },
                ) { padding ->
                    ReminderFormContent(
                        state = state,
                        onMessageChange = {},
                        onRemindAtChange = onRemindAtChange,
                        onRecurrenceChange = onRecurrenceChange,
                        onByMailChange = {},
                        onReoccurFromCompletionChange = onReoccurFromCompletionChange,
                        modifier = Modifier.padding(padding),
                    )
                }
            }
        }
    }

    @Test
    fun `shows the prefilled date`() {
        val today = LocalDate.now().toString()
        setContent(ReminderFormState(contactId = 5, remindAt = "${today}T00:00:00Z"))
        composeTestRule.onNodeWithText(today).assertIsDisplayed()
    }

    @Test
    fun `reoccur from completion switch is shown for recurring reminders`() {
        setContent(
            ReminderFormState(
                contactId = 5,
                recurrence = ReminderRecurrence.WEEKLY,
                remindAt = "${LocalDate.now()}T00:00:00Z",
            ),
        )
        composeTestRule.onNodeWithText("Reschedule from completion date").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `reoccur from completion switch is hidden for once reminders`() {
        setContent(
            ReminderFormState(
                contactId = 5,
                recurrence = ReminderRecurrence.ONCE,
                remindAt = "${LocalDate.now()}T00:00:00Z",
            ),
        )
        composeTestRule.onNodeWithText("Reschedule from completion date").assertIsNotDisplayed()
    }

    @Test
    fun `reoccur from completion switch toggles the callback`() {
        var value: Boolean? = null
        setContent(
            ReminderFormState(
                contactId = 5,
                recurrence = ReminderRecurrence.WEEKLY,
                remindAt = "${LocalDate.now()}T00:00:00Z",
            ),
            onReoccurFromCompletionChange = { value = it },
        )
        // #199: Modifier.toggleable on the ListItem merges the "Reschedule from
        // completion date" headline into the same node as the toggle state, so
        // it's reachable directly by its label — no more scoping through an
        // ancestor to disambiguate it from the by-mail switch.
        composeTestRule.onNodeWithText("Reschedule from completion date")
            .assertIsToggleable()
            .performScrollTo()
            .performClick()
        assertEquals(false, value)
    }

    @Test
    fun `save button invokes the save callback`() {
        var saved = false
        setContent(
            ReminderFormState(
                contactId = 5,
                remindAt = "${LocalDate.now()}T00:00:00Z",
            ),
            onSave = { saved = true },
        )
        composeTestRule.onNodeWithText("Create reminder").performClick()
        assertEquals(true, saved)
    }

    // Issue #1404: the primary action is pinned in the Scaffold's bottomBar, so it
    // is on screen without scrolling and stays there as the form scrolls.
    @Test
    @Config(qualifiers = "w360dp-h400dp")
    fun `save button stays displayed without scrolling and while the form scrolls`() {
        var saved = false
        setContent(ReminderFormState(contactId = 5, remindAt = "${LocalDate.now()}T00:00:00Z"), onSave = { saved = true })

        composeTestRule.onNodeWithText("Create reminder").assertIsDisplayed()
        composeTestRule.onNode(hasScrollAction()).performTouchInput { swipeUp() }
        composeTestRule.onNodeWithText("Create reminder").assertIsDisplayed()
        composeTestRule.onNode(hasScrollAction()).performTouchInput { swipeDown(); swipeDown() }
        composeTestRule.onNodeWithText("Create reminder").assertIsDisplayed()
        composeTestRule.onNodeWithText("Create reminder").performClick()
        assertEquals(true, saved)
    }
}
