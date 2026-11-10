package com.mycorrhizal.crm.feature.contacts.entities

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.EntityRepository
import com.mycorrhizal.crm.domain.repository.FieldDefinitionRepository
import com.mycorrhizal.crm.model.network.ContactEntityLinkAdoptResponse
import com.mycorrhizal.crm.model.network.ContactFieldValuesInput
import com.mycorrhizal.crm.model.network.Entity
import com.mycorrhizal.crm.model.network.EntityContactLink
import com.mycorrhizal.crm.model.network.EntityDeleteResponse
import com.mycorrhizal.crm.model.network.EntityInput
import com.mycorrhizal.crm.model.network.EntityKind
import com.mycorrhizal.crm.model.network.EntityListResponse
import com.mycorrhizal.crm.model.network.EntityMergeInput
import com.mycorrhizal.crm.model.network.EntityMergeResponse
import com.mycorrhizal.crm.model.network.EntityRestoreResponse
import com.mycorrhizal.crm.model.network.FieldDefinition
import com.mycorrhizal.crm.model.network.FieldValue
import com.mycorrhizal.crm.model.network.PartialDate
import com.mycorrhizal.crm.model.network.TemporalRange
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.coVerifyOrder
import io.mockk.mockk
import java.time.LocalDate
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test

class EntityDetailViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val repository = mockk<EntityRepository>()
    private val definitions = mockk<FieldDefinitionRepository>()

    private val acme = Entity(
        id = "e1",
        kind = EntityKind.ORGANISATION,
        name = "Acme",
        revision = 3,
        linkCount = 2,
        projectionCount = 1
    )

    private fun person(contactId: Int, linkId: String, primary: Boolean, period: TemporalRange? = null) =
        EntityContactLink(linkId = linkId, contactId = contactId, isPrimary = primary, period = period)

    @Before
    fun setUp() {
        coEvery { repository.get("e1") } returns Result.success(acme)
        coEvery { repository.contacts("e1") } returns Result.success(emptyList())
        coEvery { repository.fieldValues("e1") } returns Result.success(emptyList())
        coEvery { definitions.list(any()) } returns Result.success(emptyList())
    }

    private fun vm(): EntityDetailViewModel {
        val vm = EntityDetailViewModel(SavedStateHandle(mapOf("entityId" to "e1")), repository, definitions)
        vm.todayProvider = { LocalDate.of(2026, 11, 10) }
        return vm
    }

    @Test
    fun `loads the entity, its people and only the custom fields of its kind`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.contacts("e1") } returns Result.success(listOf(person(7, "l1", true)))
            coEvery { definitions.list(any()) } returns Result.success(
                listOf(
                    FieldDefinition(id = "d1", label = "Industry", target = "organisation"),
                    FieldDefinition(id = "d2", label = "Breed", target = "pet"),
                    FieldDefinition(id = "d3", label = "Hobby", target = "contact"),
                    FieldDefinition(id = "d4", label = "Legacy", target = null)
                )
            )
            coEvery { repository.fieldValues("e1") } returns
                Result.success(listOf(FieldValue(fieldDefinitionId = "d1", value = "Tech")))
            val vm = vm()
            advanceUntilIdle()
            val s = vm.uiState.value
            assertEquals("Acme", s.entity?.name)
            assertEquals(1, s.people.size)
            assertEquals(listOf("d1"), s.fieldDefinitions.map { it.id })
            assertEquals(mapOf("d1" to "Tech"), s.fieldValues)
        }

    @Test
    fun `a missing entity is reported as not found`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.get("e1") } returns
            Result.failure(ApiError.Client(404, "gone"))
        val vm = vm()
        advanceUntilIdle()
        assertTrue(vm.uiState.value.notFound)
        assertNull(vm.uiState.value.entity)
    }

    @Test
    fun `saving sends the revision and replaces the entity`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.update("e1", any(), 3) } returns
            Result.success(acme.copy(name = "Acme 2", revision = 4))
        val vm = vm()
        advanceUntilIdle()
        var done = false
        vm.save(EntityInput(kind = EntityKind.ORGANISATION, name = "Acme 2")) { done = true }
        advanceUntilIdle()
        assertTrue(done)
        assertEquals("Acme 2", vm.uiState.value.entity?.name)
        assertNull(vm.uiState.value.formError)
    }

    @Test
    fun `a failed save keeps the form open with the error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.update(any(), any(), any()) } returns
            Result.failure(ApiError.Unknown(RuntimeException("x")))
        val vm = vm()
        advanceUntilIdle()
        var done = false
        vm.save(EntityInput(name = "x")) { done = true }
        advanceUntilIdle()
        assertFalse(done)
        assertNotNull(vm.uiState.value.formError)
        assertEquals("Acme", vm.uiState.value.entity?.name)
    }

    @Test
    fun `delete without adopt-all only deletes and leaves a restore state`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.delete("e1", 3) } returns Result.success(EntityDeleteResponse(linkCount = 2))
            val vm = vm()
            advanceUntilIdle()
            vm.delete(adoptAll = false)
            advanceUntilIdle()
            assertTrue(vm.uiState.value.deleted)
            coVerify(exactly = 0) { repository.adopt(any(), any()) }
        }

    @Test
    fun `adopt-all copies the value into each current primary contact before deleting`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.contacts("e1") } returns Result.success(
                listOf(
                    person(7, "l7", true),
                    person(8, "l8", false),
                    person(9, "l9", true, TemporalRange(end = PartialDate(year = 2020)))
                )
            )
            coEvery { repository.adopt(any(), any()) } returns
                Result.success(ContactEntityLinkAdoptResponse(adopted = true))
            coEvery { repository.delete("e1", 3) } returns Result.success(EntityDeleteResponse())
            val vm = vm()
            advanceUntilIdle()
            vm.delete(adoptAll = true)
            advanceUntilIdle()
            coVerifyOrder {
                repository.adopt(7, "l7")
                repository.delete("e1", 3)
            }
            coVerify(exactly = 1) { repository.adopt(any(), any()) }
            assertTrue(vm.uiState.value.deleted)
        }

    @Test
    fun `if any adopt fails nothing is deleted`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.contacts("e1") } returns
            Result.success(listOf(person(7, "l7", true), person(8, "l8", true)))
        coEvery { repository.adopt(7, "l7") } returns Result.success(ContactEntityLinkAdoptResponse())
        coEvery { repository.adopt(8, "l8") } returns
            Result.failure(ApiError.Unknown(RuntimeException("x")))
        val vm = vm()
        advanceUntilIdle()
        vm.delete(adoptAll = true)
        advanceUntilIdle()
        coVerify(exactly = 0) { repository.delete(any(), any()) }
        assertFalse(vm.uiState.value.deleted)
        assertEquals(1, vm.uiState.value.adoptFailedCount)
    }

    @Test
    fun `a pet never adopts because it never projects`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.get("e1") } returns Result.success(acme.copy(kind = EntityKind.PET))
        coEvery { repository.contacts("e1") } returns Result.success(listOf(person(7, "l7", true)))
        coEvery { repository.delete("e1", 3) } returns Result.success(EntityDeleteResponse())
        val vm = vm()
        advanceUntilIdle()
        vm.delete(adoptAll = true)
        advanceUntilIdle()
        coVerify(exactly = 0) { repository.adopt(any(), any()) }
        assertTrue(vm.uiState.value.deleted)
    }

    @Test
    fun `restore brings the entity back and surfaces a same-name warning`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.delete("e1", 3) } returns Result.success(EntityDeleteResponse())
            coEvery { repository.restore("e1") } returns Result.success(
                EntityRestoreResponse(entity = acme.copy(revision = 5), sameNameLive = true, linksRevived = 2)
            )
            val vm = vm()
            advanceUntilIdle()
            vm.delete(adoptAll = false)
            advanceUntilIdle()
            coEvery { repository.get("e1") } returns Result.success(acme.copy(revision = 5))
            vm.restore()
            advanceUntilIdle()
            assertFalse(vm.uiState.value.deleted)
            assertTrue(vm.uiState.value.sameNameLive)
            assertEquals(5, vm.uiState.value.entity?.revision)
        }

    @Test
    fun `merging this entity into another leaves a merged state that can be undone`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val other = Entity(id = "e2", kind = EntityKind.ORGANISATION, name = "Acme Inc")
            coEvery { repository.merge("e1", EntityMergeInput("e2", mapOf("notes" to "loser"))) } returns
                Result.success(EntityMergeResponse(survivor = other, loserId = "e1", linksRepointed = 2))
            coEvery { repository.restore("e1") } returns
                Result.success(EntityRestoreResponse(entity = acme, mergedInto = "e2"))
            val vm = vm()
            advanceUntilIdle()
            vm.merge(other, keepThis = false, fill = setOf("notes"))
            advanceUntilIdle()
            val outcome = vm.uiState.value.mergeOutcome
            assertEquals("e2", outcome?.survivor?.id)
            assertTrue(outcome?.thisWasMergedAway == true)
            assertEquals(2, outcome?.response?.linksRepointed)

            vm.restoreMergedLoser()
            advanceUntilIdle()
            assertNull(vm.uiState.value.mergeOutcome)
            assertEquals("Acme", vm.uiState.value.entity?.name)
        }

    @Test
    fun `swapping keeps this entity and merges the other one into it`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val other = Entity(id = "e2", kind = EntityKind.ORGANISATION, name = "Acme Inc")
            coEvery { repository.merge("e2", EntityMergeInput("e1", null)) } returns
                Result.success(EntityMergeResponse(survivor = acme, loserId = "e2", linksRepointed = 1))
            val vm = vm()
            advanceUntilIdle()
            vm.merge(other, keepThis = true, fill = emptySet())
            advanceUntilIdle()
            assertFalse(vm.uiState.value.mergeOutcome?.thisWasMergedAway == true)
            assertEquals("e2", vm.uiState.value.mergeOutcome?.loser?.id)
            coVerify(atLeast = 2) { repository.get("e1") }
        }

    @Test
    fun `merge candidates are the same kind, searched by name, minus this entity`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.list(EntityKind.ORGANISATION, "ac", null, any()) } returns Result.success(
                EntityListResponse(entities = listOf(acme, Entity(id = "e2", name = "Acme Inc")))
            )
            val vm = vm()
            advanceUntilIdle()
            vm.searchMergeCandidates("ac")
            advanceUntilIdle()
            assertEquals(listOf("e2"), vm.uiState.value.mergeCandidates.map { it.id })
        }

    @Test
    fun `saving one custom field resends the complete value set`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { definitions.list(any()) } returns Result.success(
                listOf(
                    FieldDefinition(id = "d1", target = "organisation"),
                    FieldDefinition(id = "d2", target = "organisation")
                )
            )
            coEvery { repository.fieldValues("e1") } returns
                Result.success(listOf(FieldValue(fieldDefinitionId = "d1", value = "A")))
            val sent = mutableListOf<ContactFieldValuesInput>()
            coEvery { repository.replaceFieldValues("e1", capture(sent)) } returns Result.success(
                listOf(
                    FieldValue(fieldDefinitionId = "d1", value = "A"),
                    FieldValue(fieldDefinitionId = "d2", value = "B")
                )
            )
            val vm = vm()
            advanceUntilIdle()
            vm.saveFieldValue("d2", "B")
            advanceUntilIdle()
            assertEquals(setOf("d1", "d2"), sent.single().fieldValues.map { it.fieldDefinitionId }.toSet())
            assertEquals(mapOf("d1" to "A", "d2" to "B"), vm.uiState.value.fieldValues)
        }
}
