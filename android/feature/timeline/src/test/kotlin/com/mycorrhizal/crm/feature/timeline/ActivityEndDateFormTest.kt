package com.mycorrhizal.crm.feature.timeline

import androidx.compose.foundation.layout.padding
import androidx.compose.material3.SnackbarHostState
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performScrollTo
import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.ActivityRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.ActivityInput
import com.mycorrhizal.crm.model.network.ErrorReason
import com.mycorrhizal.crm.model.network.ErrorReasonCode
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import com.mycorrhizal.crm.ui.LocalDateFormat
import com.mycorrhizal.crm.ui.components.FormScaffold
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import io.mockk.slot
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/** Issue #2006: the activity form's optional end date (view model rules). */
class ActivityEndDateFormViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val activityRepository = mockk<ActivityRepository>()
    private val contactRepository = mockk<ContactRepository>()

    private fun createViewModel(activityId: Int? = null): ActivityFormViewModel {
        coEvery { contactRepository.getContact(any()) } returns
            Result.failure(RuntimeException("offline"))
        val handle = SavedStateHandle(
            mapOf<String, Any?>("contactId" to 5).toMutableMap().apply {
                if (activityId != null) put("activityId", activityId)
            }
        )
        return ActivityFormViewModel(activityRepository, contactRepository, handle)
    }

    private fun clientRefusal() = ApiError.Client(
        400,
        "Invalid value for field end_date",
        ErrorReason(ErrorReasonCode.ACTIVITY_END_BEFORE_START, emptyMap())
    )

    @Test
    fun `create with an end date sends it as the UTC-midnight instant`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val captured = slot<ActivityInput>()
            coEvery { activityRepository.create(capture(captured)) } returns
                Result.success(Activity(id = 1))
            val vm = createViewModel()
            vm.onTitleChange("Trip")
            vm.onDateChange("2026-08-10T00:00:00Z")
            vm.onEndDateChange("2026-08-14T00:00:00Z")
            vm.save()
            advanceUntilIdle()

            assertEquals("2026-08-14T00:00:00Z", captured.captured.endDate)
        }

    @Test
    fun `create without an end date sends null`() = runTest(mainDispatcherRule.testDispatcher) {
        val captured = slot<ActivityInput>()
        coEvery { activityRepository.create(capture(captured)) } returns
            Result.success(Activity(id = 1))
        val vm = createViewModel()
        vm.onTitleChange("Lunch")
        vm.onDateChange("2026-08-10T00:00:00Z")
        vm.save()
        advanceUntilIdle()

        assertNull(captured.captured.endDate)
    }

    @Test
    fun `edit hydrates the stored end and clearing it sends null on the update`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { activityRepository.get(7) } returns Result.success(
                Activity(
                    id = 7,
                    title = "Trip",
                    date = "2026-08-10T00:00:00Z",
                    endDate = "2026-08-14T00:00:00Z",
                    revision = 2
                )
            )
            val captured = slot<ActivityInput>()
            coEvery { activityRepository.update(7, capture(captured), 2) } returns
                Result.success(Activity(id = 7))
            val vm = createViewModel(activityId = 7)
            advanceUntilIdle()
            assertEquals("2026-08-14T00:00:00Z", vm.uiState.value.endDate)
            assertFalse(vm.uiState.value.isDirty)

            vm.onEndDateChange("")
            assertTrue(vm.uiState.value.isDirty)
            vm.save()
            advanceUntilIdle()

            assertNull(captured.captured.endDate)
            assertEquals(ActivityFormEvent.Saved, vm.events.value)
        }

    @Test
    fun `an unchanged stored end is sent again on the update, never omitted`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { activityRepository.get(7) } returns Result.success(
                Activity(
                    id = 7,
                    title = "Trip",
                    date = "2026-08-10T00:00:00Z",
                    endDate = "2026-08-14T00:00:00Z",
                    revision = 2
                )
            )
            val captured = slot<ActivityInput>()
            coEvery { activityRepository.update(7, capture(captured), 2) } returns
                Result.success(Activity(id = 7))
            val vm = createViewModel(activityId = 7)
            advanceUntilIdle()
            vm.save()
            advanceUntilIdle()

            assertEquals("2026-08-14T00:00:00Z", captured.captured.endDate)
        }

    @Test
    fun `an end before the start blocks the save and flags the end field, not a snackbar`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = createViewModel()
            vm.onTitleChange("Trip")
            vm.onDateChange("2026-08-10T00:00:00Z")
            vm.onEndDateChange("2026-08-09T00:00:00Z")
            assertTrue(vm.uiState.value.endBeforeStart)
            vm.save()
            advanceUntilIdle()

            coVerify(exactly = 0) { activityRepository.create(any()) }
            assertTrue(vm.uiState.value.endDateError)
            assertNull(vm.uiState.value.errorRes)
            assertNull(vm.uiState.value.error)
        }

    @Test
    fun `an end on the start day is allowed`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { activityRepository.create(any()) } returns Result.success(Activity(id = 1))
        val vm = createViewModel()
        vm.onTitleChange("Day trip")
        vm.onDateChange("2026-08-10T00:00:00Z")
        vm.onEndDateChange("2026-08-10T00:00:00Z")
        assertFalse(vm.uiState.value.endBeforeStart)
        vm.save()
        advanceUntilIdle()

        coVerify(exactly = 1) { activityRepository.create(any()) }
    }

    @Test
    fun `the server end-before-start refusal lands on the end field`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { activityRepository.create(any()) } returns Result.failure(clientRefusal())
            val vm = createViewModel()
            vm.onTitleChange("Trip")
            // A start with a time-of-day: the client passes it, the server disagrees.
            vm.onDateChange("2026-08-10T00:00:00Z")
            vm.onEndDateChange("2026-08-12T00:00:00Z")
            vm.save()
            advanceUntilIdle()

            val state = vm.uiState.value
            assertTrue(state.endDateRejected)
            assertTrue(state.endDateError)
            assertNull(state.error)
            assertNull(state.errorRes)
            assertFalse(state.isSaving)

            vm.onEndDateChange("2026-08-13T00:00:00Z")
            assertFalse(vm.uiState.value.endDateRejected)
        }

    @Test
    fun `any other server refusal still surfaces as the generic error`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { activityRepository.create(any()) } returns
                Result.failure(ApiError.Client(400, "Bad title"))
            val vm = createViewModel()
            vm.onTitleChange("Trip")
            vm.onDateChange("2026-08-10T00:00:00Z")
            vm.save()
            advanceUntilIdle()

            assertFalse(vm.uiState.value.endDateRejected)
            assertEquals("Bad title", vm.uiState.value.error?.message)
        }
}

/** Issue #2006: the editor shows both date fields and the end-date error on the end field. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], qualifiers = "w480dp-h2000dp")
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ActivityEndDateFormScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(state: ActivityFormState) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                FormScaffold(
                    title = "Form",
                    onBack = {},
                    saveLabel = "Create activity",
                    isSaving = false,
                    onSave = {},
                    snackbarHostState = remember { SnackbarHostState() }
                ) { padding ->
                    ActivityFormContent(
                        state = state,
                        onTitleChange = {},
                        onTypeChange = {},
                        onDateChange = {},
                        onEndDateChange = {},
                        onDescriptionChange = {},
                        onLocationChange = {},
                        onContactSearchChange = {},
                        onAddParticipant = {},
                        onRemoveParticipant = {},
                        modifier = Modifier.padding(padding)
                    )
                }
            }
        }
    }

    @Test
    fun `shows both the start and the optional end date fields`() {
        setContent(ActivityFormState(date = "2026-08-10T00:00:00Z"))
        composeTestRule.onNodeWithText("Date").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("End date (optional)").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `a stored end date renders in the date-format preference`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                CompositionLocalProvider(LocalDateFormat provides "iso") {
                    ActivityFormContent(
                        state = ActivityFormState(
                            date = "2026-08-10T00:00:00Z",
                            endDate = "2026-08-14T00:00:00Z"
                        ),
                        onTitleChange = {},
                        onTypeChange = {},
                        onDateChange = {},
                        onEndDateChange = {},
                        onDescriptionChange = {},
                        onLocationChange = {},
                        onContactSearchChange = {},
                        onAddParticipant = {},
                        onRemoveParticipant = {}
                    )
                }
            }
        }
        composeTestRule.onNodeWithText("2026-08-14").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `an end before the start shows the message on the end field`() {
        setContent(
            ActivityFormState(date = "2026-08-12T00:00:00Z", endDate = "2026-08-10T00:00:00Z")
        )
        composeTestRule.onNodeWithText("The end date cannot be before the start date")
            .performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `a server-rejected end shows the same message on the end field`() {
        setContent(
            ActivityFormState(
                date = "2026-08-10T00:00:00Z",
                endDate = "2026-08-12T00:00:00Z",
                endDateRejected = true
            )
        )
        composeTestRule.onNodeWithText("The end date cannot be before the start date")
            .performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `a valid range shows no end-date message`() {
        setContent(
            ActivityFormState(date = "2026-08-10T00:00:00Z", endDate = "2026-08-12T00:00:00Z")
        )
        composeTestRule.onNodeWithText("The end date cannot be before the start date")
            .assertDoesNotExist()
    }
}
