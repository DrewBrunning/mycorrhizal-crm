package com.mycorrhizal.crm.feature.timeline

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.ActivityRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.GeoPulseRepository
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.ActivityInput
import com.mycorrhizal.crm.model.network.GeoPulsePhotoSuggestion
import com.mycorrhizal.crm.model.network.GeoPulseStaySuggestion
import com.mycorrhizal.crm.model.network.GeoPulseSuggestionsResponse
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import io.mockk.slot
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import java.time.ZoneId

private fun stay(
    id: Long = 42,
    location: String = "Cafe Roma",
    timestamp: String = "2026-03-10T14:30:00Z",
    existing: Int? = null,
    photos: List<GeoPulsePhotoSuggestion> = emptyList(),
    unavailable: Boolean = false,
) = GeoPulseStaySuggestion(
    stayId = id,
    externalRef = "geopulse:stay:$id",
    location = location,
    city = "Rome",
    country = "Italy",
    timestamp = timestamp,
    durationSeconds = 5400,
    photos = photos,
    photosUnavailable = unavailable,
    existingActivityId = existing,
)

class GeoPulseSuggestionsViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val repository = mockk<GeoPulseRepository>()

    @Test
    fun `lookup sends the picked date and the device timezone and stores the stays`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.getSuggestions(any(), any()) } returns
                Result.success(GeoPulseSuggestionsResponse("2026-03-10", listOf(stay())))
            val vm = GeoPulseSuggestionsViewModel(repository)

            vm.onDateChange("2026-03-10")
            vm.lookup()
            advanceUntilIdle()

            coVerify { repository.getSuggestions("2026-03-10", ZoneId.systemDefault().id) }
            val state = vm.uiState.value
            assertEquals(1, state.suggestions!!.size)
            assertEquals("2026-03-10", state.lookedUpDate)
            assertNull(state.error)
        }

    @Test
    fun `lookup failure shows the server message and clears stale results`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.getSuggestions(any(), any()) } returns
                Result.failure(ApiError.Client(400, "No GeoPulse connection configured"))
            val vm = GeoPulseSuggestionsViewModel(repository)

            vm.lookup()
            advanceUntilIdle()

            assertEquals("No GeoPulse connection configured", vm.uiState.value.error)
            assertNull(vm.uiState.value.suggestions)
        }

    @Test
    fun `refreshIfLoaded is a no-op before a lookup and re-runs the looked-up date after`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.getSuggestions(any(), any()) } returns
                Result.success(GeoPulseSuggestionsResponse("2026-03-10", listOf(stay())))
            val vm = GeoPulseSuggestionsViewModel(repository)

            vm.refreshIfLoaded()
            advanceUntilIdle()
            coVerify(exactly = 0) { repository.getSuggestions(any(), any()) }

            vm.onDateChange("2026-03-10")
            vm.lookup()
            advanceUntilIdle()
            // The picker moved on without a new lookup; coming back re-queries the looked-up day.
            vm.onDateChange("2026-04-01")
            vm.refreshIfLoaded()
            advanceUntilIdle()

            coVerify(exactly = 2) { repository.getSuggestions("2026-03-10", any()) }
            assertEquals("2026-03-10", vm.uiState.value.date)
        }

    @Test
    fun `prefill date is the stay's own local day, not the picker date (issue 1500)`() {
        // 23:30 UTC is already the next calendar day in Rome (UTC+1 in March, before DST).
        val rome = ZoneId.of("Europe/Rome")
        assertEquals(
            "2026-03-11T00:00:00+01:00",
            stayPrefillDate("2026-03-10T23:30:00Z", rome, fallbackDate = "2026-03-10"),
        )
        assertEquals(
            "2026-03-10T00:00:00-07:00",
            stayPrefillDate("2026-03-10T14:30:00Z", ZoneId.of("America/Phoenix"), "1999-01-01"),
        )
        // Unparseable timestamps fall back to the lookup date, never crash.
        assertEquals("2026-03-10T00:00:00Z", stayPrefillDate("garbage", ZoneId.of("UTC"), "2026-03-10"))
    }

    @Test
    fun `stay time is rendered in the lookup zone`() {
        assertEquals("15:30", formatStayTime("2026-03-10T14:30:00Z", ZoneId.of("Europe/Rome")))
        assertEquals("nope", formatStayTime("nope", ZoneId.of("UTC")))
    }
}

class ActivityFormGeoPulsePrefillTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val activityRepository = mockk<ActivityRepository>()
    private val contactRepository = mockk<ContactRepository>()

    @Test
    fun `create form is pre-filled from nav args and saves the external_ref`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val input = slot<ActivityInput>()
            coEvery { activityRepository.create(capture(input)) } returns Result.success(Activity(id = 1))
            val handle = SavedStateHandle(
                mapOf(
                    "contactId" to 0,
                    "location" to "Cafe Roma",
                    "date" to "2026-03-10T00:00:00+01:00",
                    "externalRef" to "geopulse:stay:42",
                ),
            )
            val vm = ActivityFormViewModel(activityRepository, contactRepository, handle)

            val state = vm.uiState.value
            assertEquals("Cafe Roma", state.location)
            assertEquals("2026-03-10T00:00:00+01:00", state.date)
            assertEquals("geopulse:stay:42", state.externalRef)

            vm.onTitleChange("Coffee")
            vm.save()
            advanceUntilIdle()

            assertEquals("geopulse:stay:42", input.captured.externalRef)
            assertEquals("Cafe Roma", input.captured.location)
            assertEquals("2026-03-10T00:00:00+01:00", input.captured.date)
        }

    @Test
    fun `a plain create form has no prefill and edit mode ignores the args`() {
        val plain = ActivityFormViewModel(
            activityRepository,
            contactRepository,
            SavedStateHandle(mapOf("contactId" to 0)),
        )
        assertEquals("", plain.uiState.value.location)
        assertNull(plain.uiState.value.externalRef)

        coEvery { activityRepository.get(7) } returns Result.success(Activity(id = 7, title = "x"))
        val edit = ActivityFormViewModel(
            activityRepository,
            contactRepository,
            SavedStateHandle(mapOf("contactId" to 0, "activityId" to 7, "externalRef" to "geopulse:stay:1")),
        )
        assertNull(edit.uiState.value.externalRef)
    }
}

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class GeoPulseSuggestionsScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun state(
        suggestions: List<GeoPulseStaySuggestion>? = null,
        error: String? = null,
    ) = GeoPulseSuggestionsUiState(
        date = "2026-03-10",
        suggestions = suggestions,
        error = error,
        timezone = "UTC",
    )

    private fun show(
        s: GeoPulseSuggestionsUiState,
        onLogStay: (GeoPulseStaySuggestion, String) -> Unit = { _, _ -> },
        onOpenSettings: () -> Unit = {},
    ) = composeTestRule.setContent {
        MycorrhizalTheme { GeoPulseSuggestionsContent(state = s, onLogStay = onLogStay, onOpenSettings = onOpenSettings) }
    }

    @Test
    fun `each stay shows place time duration area and a place-and-time labelled log action`() {
        var logged: Pair<GeoPulseStaySuggestion, String>? = null
        show(state(listOf(stay())), onLogStay = { s, d -> logged = s to d })

        composeTestRule.onNodeWithText("Cafe Roma").assertIsDisplayed()
        composeTestRule.onNodeWithText("14:30 · 1 h 30 min · Rome, Italy").assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("Log activity at Cafe Roma, 14:30").performClick()

        assertEquals("geopulse:stay:42", logged!!.first.externalRef)
        assertEquals("2026-03-10T00:00:00Z", logged!!.second)
    }

    @Test
    fun `result count is announced through a live region`() {
        show(state(listOf(stay(1), stay(2, location = "Park"))))
        composeTestRule.onNodeWithText("Places found: 2").assertIsDisplayed()
    }

    @Test
    fun `an empty result says so`() {
        show(state(emptyList()))
        composeTestRule.onNodeWithText("No places found").assertIsDisplayed()
        composeTestRule.onNodeWithText("GeoPulse has no recorded stays for this date.").assertIsDisplayed()
    }

    @Test
    fun `already logged stays have no log action`() {
        show(state(listOf(stay(existing = 9))))
        composeTestRule.onNodeWithText("Already logged").assertIsDisplayed()
        composeTestRule.onNodeWithText("Log activity").assertDoesNotExist()
    }

    @Test
    fun `photo file names show as chips and unavailable photos are noted`() {
        show(
            state(
                listOf(
                    stay(1, photos = listOf(GeoPulsePhotoSuggestion("p1", "IMG_1.jpg"))),
                    stay(2, location = "Park", unavailable = true),
                ),
            ),
        )
        composeTestRule.onNodeWithText("Photos nearby (1)").assertIsDisplayed()
        composeTestRule.onNodeWithText("IMG_1.jpg").assertIsDisplayed()
        composeTestRule.onNodeWithText("Photos could not be checked.").assertIsDisplayed()
    }

    @Test
    fun `an error offers the settings shortcut`() {
        var opened = false
        show(state(error = "No GeoPulse connection configured"), onOpenSettings = { opened = true })
        composeTestRule.onNodeWithText("No GeoPulse connection configured").assertIsDisplayed()
        composeTestRule.onNodeWithText("Open GeoPulse settings").performClick()
        assertTrue(opened)
    }
}
