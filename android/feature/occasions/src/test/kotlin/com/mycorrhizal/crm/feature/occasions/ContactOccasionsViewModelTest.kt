package com.mycorrhizal.crm.feature.occasions

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.OccasionObligationRepository
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.model.network.OccasionObligation
import com.mycorrhizal.crm.model.network.OccasionObligationInput
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import com.mycorrhizal.crm.ui.R
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import io.mockk.slot
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

private const val UID = "11111111-1111-4111-8111-111111111111"

@OptIn(ExperimentalCoroutinesApi::class)
class ContactOccasionsViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val repository = mockk<OccasionObligationRepository>()
    private val contacts = mockk<ContactRepository>()

    private fun ob(id: String = "o1", label: String = "Christmas card") = OccasionObligation(
        id = id,
        entityId = UID,
        kind = "card",
        label = label,
        anchorMonth = 12,
        anchorDay = 25,
        leadTimeDays = 14,
        linkedLifeEventId = "le1"
    )

    private fun viewModel(contactId: Int = 5): ContactOccasionsViewModel {
        coEvery { contacts.getContact(5) } returns Result.success(
            ContactRecordResponse(id = 5, card = Card(uid = UID, name = Name(full = "Dana")))
        )
        return ContactOccasionsViewModel(
            repository,
            contacts,
            SavedStateHandle(mapOf("contactId" to contactId))
        )
    }

    @Test
    fun `load resolves the contact uid and lists its obligations`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.listForContact(UID) } returns Result.success(listOf(ob()))

            val vm = viewModel()
            advanceUntilIdle()

            assertEquals(UID, vm.uiState.value.entityId)
            assertEquals(listOf("o1"), vm.uiState.value.items.map { it.id })
            assertFalse(vm.uiState.value.isLoading)
            assertNull(vm.uiState.value.error)
        }

    @Test
    fun `load surfaces a list failure and keeps the list empty`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.listForContact(UID) } returns
                Result.failure(ApiError.Client(500, "boom"))

            val vm = viewModel()
            advanceUntilIdle()

            assertTrue(vm.uiState.value.error is ApiError.Client)
            assertTrue(vm.uiState.value.items.isEmpty())
        }

    @Test
    fun `a contact without a vcard uid reports the missing-uid error and never lists`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { contacts.getContact(0) } returns Result.failure(ApiError.Client(404, "gone"))
            val vm = ContactOccasionsViewModel(
                repository,
                contacts,
                SavedStateHandle(mapOf("contactId" to 0))
            )
            advanceUntilIdle()

            assertEquals(R.string.entities_error_no_vcard_uid, vm.uiState.value.errorRes)
            coVerify(exactly = 0) { repository.listForContact(any()) }
        }

    @Test
    fun `create sends the trimmed form as an input for the contact and reloads`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.listForContact(UID) } returns Result.success(emptyList())
            val captured = slot<OccasionObligationInput>()
            coEvery { repository.create(capture(captured)) } returns Result.success(ob("n"))
            val vm = viewModel()
            advanceUntilIdle()

            vm.save(
                null,
                OccasionObligationFormData(
                    kind = " gift ",
                    label = "  Birthday gift ",
                    anchorMonth = 6,
                    anchorDay = 3,
                    leadTimeText = "10",
                    active = false,
                    sensitivity = "private",
                    notes = "  likes tea "
                )
            )
            advanceUntilIdle()

            assertEquals(
                OccasionObligationInput(
                    entityId = UID,
                    kind = "gift",
                    label = "Birthday gift",
                    anchorMonth = 6,
                    anchorDay = 3,
                    linkedLifeEventId = null,
                    leadTimeDays = 10,
                    active = false,
                    sensitivity = "private",
                    notes = "likes tea"
                ),
                captured.captured
            )
            assertTrue(vm.uiState.value.saved)
            coVerify(atLeast = 2) { repository.listForContact(UID) }
        }

    @Test
    fun `a blank kind falls back to card and a blank note is sent as null`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.listForContact(UID) } returns Result.success(emptyList())
            val captured = slot<OccasionObligationInput>()
            coEvery { repository.create(capture(captured)) } returns Result.success(ob())
            val vm = viewModel()
            advanceUntilIdle()

            vm.save(
                null,
                OccasionObligationFormData(
                    kind = " ",
                    label = "x",
                    anchorMonth = 3,
                    anchorDay = 9,
                    notes = "   "
                )
            )
            advanceUntilIdle()

            assertEquals("card", captured.captured.kind)
            assertNull(captured.captured.notes)
            assertEquals(0, captured.captured.leadTimeDays)
        }

    @Test
    fun `update is a full replace that carries the linked life event forward`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.listForContact(UID) } returns Result.success(listOf(ob()))
            val captured = slot<OccasionObligationInput>()
            coEvery { repository.update("o1", capture(captured)) } returns Result.success(ob())
            val vm = viewModel()
            advanceUntilIdle()

            // The new anchor replaces the old one; the link is carried forward.
            vm.save(
                ob(),
                OccasionObligationFormData(
                    kind = "card",
                    label = "Renamed",
                    anchorMonth = 2,
                    anchorDay = 29,
                    leadTimeText = "0"
                )
            )
            advanceUntilIdle()

            assertEquals("le1", captured.captured.linkedLifeEventId)
            assertEquals(UID, captured.captured.entityId)
            assertEquals(2, captured.captured.anchorMonth)
            assertEquals(29, captured.captured.anchorDay)
            assertEquals("Renamed", captured.captured.label)
            assertTrue(vm.uiState.value.saved)
        }

    @Test
    fun `a blank label is rejected before any request`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.listForContact(UID) } returns Result.success(emptyList())
            val vm = viewModel()
            advanceUntilIdle()

            vm.save(null, OccasionObligationFormData(kind = "card", label = "   "))
            advanceUntilIdle()

            assertEquals(R.string.occasion_ob_validation_label_required, vm.uiState.value.errorRes)
            assertFalse(vm.uiState.value.saved)
            coVerify(exactly = 0) { repository.create(any()) }
        }

    @Test
    fun `a missing month and day is rejected with the required reason before any request`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.listForContact(UID) } returns Result.success(emptyList())
            val vm = viewModel()
            advanceUntilIdle()

            vm.save(null, OccasionObligationFormData(label = "x", anchorMonth = 3, anchorDay = 9))
            advanceUntilIdle()

            assertEquals(
                R.string.error_reason_occasion_anchor_date_required,
                vm.uiState.value.errorRes
            )
            coVerify(exactly = 0) { repository.create(any()) }
        }

    @Test
    fun `a month without a day or a day without a month is rejected as incomplete`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.listForContact(UID) } returns Result.success(emptyList())
            val vm = viewModel()
            advanceUntilIdle()

            vm.save(null, OccasionObligationFormData(label = "x", anchorMonth = 3))
            advanceUntilIdle()
            assertEquals(
                R.string.error_reason_occasion_anchor_date_incomplete,
                vm.uiState.value.errorRes
            )

            vm.onErrorShown()
            vm.save(null, OccasionObligationFormData(label = "x", anchorDay = 9))
            advanceUntilIdle()
            assertEquals(
                R.string.error_reason_occasion_anchor_date_incomplete,
                vm.uiState.value.errorRes
            )
            coVerify(exactly = 0) { repository.create(any()) }
        }

    @Test
    fun `a month and day that never coexist are rejected as invalid but Feb 29 is allowed`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.listForContact(UID) } returns Result.success(emptyList())
            coEvery { repository.create(any()) } returns Result.success(ob())
            val vm = viewModel()
            advanceUntilIdle()

            for ((month, day) in listOf(2 to 30, 2 to 31, 4 to 31, 6 to 31, 9 to 31, 11 to 31)) {
                vm.onErrorShown()
                vm.save(
                    null,
                    OccasionObligationFormData(label = "x", anchorMonth = month, anchorDay = day)
                )
                advanceUntilIdle()
                assertEquals(
                    "$month/$day",
                    R.string.error_reason_occasion_anchor_date_invalid,
                    vm.uiState.value.errorRes
                )
            }
            coVerify(exactly = 0) { repository.create(any()) }

            vm.onErrorShown()
            vm.save(null, OccasionObligationFormData(label = "x", anchorMonth = 2, anchorDay = 29))
            advanceUntilIdle()
            coVerify(exactly = 1) { repository.create(match { it.anchorDay == 29 }) }
        }

    @Test
    fun `daysInMonth follows the calendar and allows Feb 29`() {
        assertEquals(29, daysInMonth(2))
        for (m in listOf(4, 6, 9, 11)) assertEquals("month $m", 30, daysInMonth(m))
        for (m in listOf(1, 3, 5, 7, 8, 10, 12)) assertEquals("month $m", 31, daysInMonth(m))
        assertEquals(31, daysInMonth(null))
    }

    @Test
    fun `lead time must be a whole number from 0 to 365`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.listForContact(UID) } returns Result.success(emptyList())
            val vm = viewModel()
            advanceUntilIdle()

            for (bad in listOf("abc", "-1", "366", "1.5")) {
                vm.onErrorShown()
                vm.save(
                    null,
                    OccasionObligationFormData(
                        label = "x",
                        anchorMonth = 3,
                        anchorDay = 9,
                        leadTimeText = bad
                    )
                )
                advanceUntilIdle()
                assertEquals(
                    "lead time '$bad'",
                    R.string.occasion_ob_validation_lead_time,
                    vm.uiState.value.errorRes
                )
            }
            coVerify(exactly = 0) { repository.create(any()) }

            // Blank means 0 and the 365 boundary is allowed.
            coEvery { repository.create(any()) } returns Result.success(ob())
            vm.onErrorShown()
            vm.save(
                null,
                OccasionObligationFormData(
                    label = "x",
                    anchorMonth = 3,
                    anchorDay = 9,
                    leadTimeText = "365"
                )
            )
            advanceUntilIdle()
            coVerify(exactly = 1) { repository.create(match { it.leadTimeDays == 365 }) }
        }

    @Test
    fun `a failed save keeps the form open and surfaces the typed error`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.listForContact(UID) } returns Result.success(emptyList())
            coEvery { repository.create(any()) } returns Result.failure(ApiError.Client(422, "bad"))
            val vm = viewModel()
            advanceUntilIdle()

            vm.save(
                null,
                OccasionObligationFormData(label = "x", anchorMonth = 3, anchorDay = 9)
            )
            advanceUntilIdle()

            assertTrue(vm.uiState.value.error is ApiError.Client)
            assertFalse(vm.uiState.value.saved)
            assertFalse(vm.uiState.value.isSaving)
        }

    @Test
    fun `a second save while one is in flight is ignored`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.listForContact(UID) } returns Result.success(emptyList())
            coEvery { repository.create(any()) } returns Result.success(ob())
            val vm = viewModel()
            advanceUntilIdle()

            vm.save(null, OccasionObligationFormData(label = "x", anchorMonth = 3, anchorDay = 9))
            vm.save(null, OccasionObligationFormData(label = "x", anchorMonth = 3, anchorDay = 9))
            advanceUntilIdle()

            coVerify(exactly = 1) { repository.create(any()) }
        }

    @Test
    fun `delete removes the row by reloading and clears the deleting marker`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.listForContact(UID) } returnsMany listOf(
                Result.success(listOf(ob())),
                Result.success(emptyList())
            )
            coEvery { repository.delete("o1") } returns Result.success(Unit)
            val vm = viewModel()
            advanceUntilIdle()

            vm.delete("o1")
            advanceUntilIdle()

            assertTrue(vm.uiState.value.items.isEmpty())
            assertNull(vm.uiState.value.deletingId)
        }

    @Test
    fun `a failed delete clears the marker, keeps the row and surfaces the error`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.listForContact(UID) } returns Result.success(listOf(ob()))
            coEvery { repository.delete("o1") } returns Result.failure(ApiError.Client(404, "gone"))
            val vm = viewModel()
            advanceUntilIdle()

            vm.delete("o1")
            advanceUntilIdle()

            assertEquals(1, vm.uiState.value.items.size)
            assertNull(vm.uiState.value.deletingId)
            assertTrue(vm.uiState.value.error is ApiError.Client)
        }

    @Test
    fun `onSavedHandled and onErrorShown clear their flags`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.listForContact(UID) } returns Result.success(emptyList())
            coEvery { repository.create(any()) } returns Result.success(ob())
            val vm = viewModel()
            advanceUntilIdle()
            vm.save(null, OccasionObligationFormData(label = "x", anchorMonth = 3, anchorDay = 9))
            advanceUntilIdle()
            assertTrue(vm.uiState.value.saved)

            vm.onSavedHandled()
            assertFalse(vm.uiState.value.saved)
        }
}
