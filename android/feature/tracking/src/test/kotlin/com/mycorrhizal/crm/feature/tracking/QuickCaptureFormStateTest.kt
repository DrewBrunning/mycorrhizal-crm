package com.mycorrhizal.crm.feature.tracking

import com.mycorrhizal.crm.domain.repository.ActivityRepository
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.ActivityInput
import com.mycorrhizal.crm.model.network.ContactSummary
import io.mockk.coEvery
import io.mockk.mockk
import io.mockk.slot
import io.mockk.coVerify
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class QuickCaptureFormStateTest {

    private val jane = ContactSummary(id = 7, firstname = "Jane", lastname = "Doe")

    @Test
    fun `save creates the activity with the prefilled participant and date`() = runTest {
        val repo = mockk<ActivityRepository>()
        val captured = slot<ActivityInput>()
        coEvery { repo.create(capture(captured)) } returns Result.success(Activity(id = 1, title = "Call"))
        val scope = CoroutineScope(StandardTestDispatcher(testScheduler))
        val state = QuickCaptureFormState(
            activityRepository = repo,
            scope = scope,
            prefill = QuickCapturePrefillFactory.forCall(jane, "2026-08-18T12:00:00Z"),
        )

        state.save()
        advanceUntilIdle()

        assertTrue(state.saved)
        val input = captured.captured
        assertEquals("Call", input.title)
        assertEquals("call", input.type)
        assertEquals("2026-08-18T12:00:00Z", input.date)
        assertEquals(listOf(7), input.contactIds)
    }

    @Test
    fun `an unknown caller saves a contact-less activity`() = runTest {
        val repo = mockk<ActivityRepository>()
        val captured = slot<ActivityInput>()
        coEvery { repo.create(capture(captured)) } returns Result.success(Activity(id = 2))
        val scope = CoroutineScope(StandardTestDispatcher(testScheduler))
        val state = QuickCaptureFormState(
            activityRepository = repo,
            scope = scope,
            prefill = QuickCapturePrefillFactory.forCall(null, "2026-08-18T12:00:00Z"),
        )

        state.save()
        advanceUntilIdle()

        assertTrue(state.saved)
        assertNull(captured.captured.contactIds)
    }

    @Test
    fun `editing the fields changes what is saved`() = runTest {
        val repo = mockk<ActivityRepository>()
        val captured = slot<ActivityInput>()
        coEvery { repo.create(capture(captured)) } returns Result.success(Activity(id = 3))
        val scope = CoroutineScope(StandardTestDispatcher(testScheduler))
        val state = QuickCaptureFormState(
            activityRepository = repo,
            scope = scope,
            prefill = QuickCapturePrefillFactory.forCall(jane, "2026-08-18T12:00:00Z"),
        )

        state.onTitleChange("Caught up over lunch")
        state.onDescriptionChange("Talked about the move")
        state.save()
        advanceUntilIdle()

        assertEquals("Caught up over lunch", captured.captured.title)
        assertEquals("Talked about the move", captured.captured.description)
    }

    @Test
    fun `a blank title reports the required error and never saves`() = runTest {
        val repo = mockk<ActivityRepository>(relaxed = true)
        val scope = CoroutineScope(StandardTestDispatcher(testScheduler))
        val state = QuickCaptureFormState(
            activityRepository = repo,
            scope = scope,
            prefill = QuickCapturePrefillFactory.forCall(jane, "2026-08-18T12:00:00Z"),
            blankTitleMessage = "Title is required",
        )

        state.onTitleChange("   ")
        state.save()
        advanceUntilIdle()

        assertFalse(state.saved)
        assertEquals("Title is required", state.error)
        coVerify(exactly = 0) { repo.create(any()) }
    }

    @Test
    fun `a failed save surfaces the error and allows a retry`() = runTest {
        val repo = mockk<ActivityRepository>()
        coEvery { repo.create(any()) } returns Result.failure(IllegalStateException("offline"))
        val scope = CoroutineScope(StandardTestDispatcher(testScheduler))
        val state = QuickCaptureFormState(
            activityRepository = repo,
            scope = scope,
            prefill = QuickCapturePrefillFactory.forCall(jane, "2026-08-18T12:00:00Z"),
        )

        state.save()
        advanceUntilIdle()

        assertFalse(state.saved)
        assertEquals("offline", state.error)
        assertFalse(state.isSaving)
    }

    @Test
    fun `the first edit invokes onFirstInteraction exactly once`() {
        // #201 (WCAG 2.2.1): the overlay's auto-dismiss timer must be cancelled
        // on the first engagement with the form — but only once, and not for a
        // sheet nobody ever touched (which is exactly when the timer should
        // still fire).
        var calls = 0
        val state = QuickCaptureFormState(
            activityRepository = mockk<ActivityRepository>(relaxed = true),
            scope = CoroutineScope(StandardTestDispatcher()),
            prefill = QuickCapturePrefillFactory.forCall(jane, "2026-08-18T12:00:00Z"),
            onFirstInteraction = { calls++ },
        )

        state.onTitleChange("Caught up")
        state.onTypeChange("lunch")
        state.onDescriptionChange("Talked about the move")
        state.onTitleChange("Caught up again")

        assertEquals(1, calls)
    }
}
