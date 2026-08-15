package com.mycorrhizal.crm.feature.circles

import com.mycorrhizal.crm.domain.repository.CircleRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.ContactsPage
import com.mycorrhizal.crm.domain.repository.TagRepository
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.Tag
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class TriageViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val contactRepository = mockk<ContactRepository>()
    private val circleRepository = mockk<CircleRepository>()
    private val tagRepository = mockk<TagRepository>()

    private fun viewModel(): TriageViewModel =
        TriageViewModel(contactRepository, circleRepository, tagRepository)

    private fun contactsPage(vararg uids: String): ContactsPage = ContactsPage(
        contacts = uids.mapIndexed { i, uid -> ContactSummary(id = i + 1, uid = uid, fn = "Contact $i") },
        nextCursor = null,
        limit = 500,
        sync = null,
    )

    @Test
    fun `load collects legacy strings with per-string counts sorted descending`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { contactRepository.listLegacyCircles() } returns Result.success(listOf("Friends", "Ski club"))
            coEvery { contactRepository.listContacts(circleLegacy = "Friends", limit = 500) } returns
                Result.success(contactsPage("u1", "u2", "u3"))
            coEvery { contactRepository.listContacts(circleLegacy = "Ski club", limit = 500) } returns
                Result.success(contactsPage("u4", "u5"))

            val vm = viewModel()
            advanceUntilIdle()

            val items = vm.uiState.value.items
            // Friends has 3 contacts, Ski club 2 — sorted count-descending.
            assertEquals(listOf("Friends", "Ski club"), items.map { it.original })
            assertEquals(3, items[0].contactCount)
            assertEquals(2, items[1].contactCount)
            // Default classification is Circle (web's default).
            assertEquals(TriageClassification.CIRCLE, items[0].classification)
        }

    @Test
    fun `a failed legacy fetch surfaces the error`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { contactRepository.listLegacyCircles() } returns
                Result.failure(com.mycorrhizal.crm.network.ApiError.Client(500, "boom"))

            val vm = viewModel()
            advanceUntilIdle()

            assertFalse(vm.uiState.value.isLoading)
            assertEquals("boom", vm.uiState.value.error)
        }

    @Test
    fun `classify and rename update the items`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { contactRepository.listLegacyCircles() } returns Result.success(listOf("Friends"))
            coEvery { contactRepository.listContacts(circleLegacy = "Friends", limit = 500) } returns
                Result.success(contactsPage("u1"))
            val vm = viewModel()
            advanceUntilIdle()

            vm.setClassification(0, TriageClassification.TAG)
            vm.setName(0, "Old friends")

            val item = vm.uiState.value.items[0]
            assertEquals(TriageClassification.TAG, item.classification)
            assertEquals("Old friends", item.name)
        }

    @Test
    fun `apply creates circles and tags then adds their members`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { contactRepository.listLegacyCircles() } returns Result.success(listOf("Friends", "Ski club"))
            coEvery { contactRepository.listContacts(circleLegacy = "Friends", limit = 500) } returns
                Result.success(contactsPage("u1"))
            coEvery { contactRepository.listContacts(circleLegacy = "Ski club", limit = 500) } returns
                Result.success(contactsPage("u2"))
            coEvery { circleRepository.list(limit = 200) } returns Result.success(emptyList())
            coEvery { tagRepository.list(limit = 200) } returns Result.success(emptyList())
            coEvery { circleRepository.create("Friends") } returns Result.success(Circle(id = "c1", name = "Friends"))
            coEvery { tagRepository.create("Ski club") } returns Result.success(Tag(id = "t1", name = "Ski club"))
            coEvery { circleRepository.addMember("c1", "u1") } returns Result.success(
                com.mycorrhizal.crm.model.network.CircleMember(circleId = "c1", memberVCardUid = "u1"),
            )
            coEvery { tagRepository.addContact("t1", "u2") } returns Result.success(
                com.mycorrhizal.crm.model.network.ContactTag(tagId = "t1", contactVCardUid = "u2"),
            )

            val vm = viewModel()
            advanceUntilIdle()
            // Ski club -> tag, Friends stays a circle (index 1 = Ski club;
            // equal counts keep the original order).
            vm.setClassification(1, TriageClassification.TAG)
            vm.apply()
            advanceUntilIdle()

            val state = vm.uiState.value
            assertTrue(state.done)
            assertEquals(1, state.appliedCircles)
            assertEquals(1, state.appliedTags)
            coVerify { circleRepository.create("Friends") }
            coVerify { tagRepository.create("Ski club") }
            coVerify { circleRepository.addMember("c1", "u1") }
            coVerify { tagRepository.addContact("t1", "u2") }
        }

    @Test
    fun `apply reuses an existing entity of the same name so a re-run cannot duplicate`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { contactRepository.listLegacyCircles() } returns Result.success(listOf("Friends"))
            coEvery { contactRepository.listContacts(circleLegacy = "Friends", limit = 500) } returns
                Result.success(contactsPage("u1"))
            coEvery { circleRepository.list(limit = 200) } returns
                Result.success(listOf(Circle(id = "c9", name = "Friends")))
            coEvery { tagRepository.list(limit = 200) } returns Result.success(emptyList())
            coEvery { circleRepository.addMember("c9", "u1") } returns Result.success(
                com.mycorrhizal.crm.model.network.CircleMember(circleId = "c9", memberVCardUid = "u1"),
            )

            val vm = viewModel()
            advanceUntilIdle()
            vm.apply()
            advanceUntilIdle()

            coVerify(exactly = 0) { circleRepository.create("Friends") }
            coVerify { circleRepository.addMember("c9", "u1") }
        }

    @Test
    fun `skipped items are never created`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { contactRepository.listLegacyCircles() } returns Result.success(listOf("Obsolete"))
            coEvery { contactRepository.listContacts(circleLegacy = "Obsolete", limit = 500) } returns
                Result.success(contactsPage("u1"))
            coEvery { circleRepository.list(limit = 200) } returns Result.success(emptyList())
            coEvery { tagRepository.list(limit = 200) } returns Result.success(emptyList())

            val vm = viewModel()
            advanceUntilIdle()
            vm.setClassification(0, TriageClassification.SKIP)

            assertFalse(vm.uiState.value.hasWork)
            vm.apply()
            advanceUntilIdle()

            // Nothing to do: apply returns early without creating or marking done.
            assertFalse(vm.uiState.value.done)
            coVerify(exactly = 0) { circleRepository.create(any()) }
            coVerify(exactly = 0) { tagRepository.create(any()) }
        }
}
