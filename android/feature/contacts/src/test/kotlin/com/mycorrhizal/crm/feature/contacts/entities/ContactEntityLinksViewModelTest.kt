package com.mycorrhizal.crm.feature.contacts.entities

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.EntityRepository
import com.mycorrhizal.crm.model.network.Address
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactEntityLink
import com.mycorrhizal.crm.model.network.ContactEntityLinkAdoptResponse
import com.mycorrhizal.crm.model.network.ContactEntityLinkFromExistingResponse
import com.mycorrhizal.crm.model.network.ContactEntityLinkInput
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.Entity
import com.mycorrhizal.crm.model.network.EntityInput
import com.mycorrhizal.crm.model.network.EntityKind
import com.mycorrhizal.crm.model.network.EntityListResponse
import com.mycorrhizal.crm.model.network.Organization
import com.mycorrhizal.crm.model.network.PartialDate
import com.mycorrhizal.crm.model.network.TemporalRange
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
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

class ContactEntityLinksViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val repository = mockk<EntityRepository>()
    private val contacts = mockk<ContactRepository>()

    private val acmeLink = ContactEntityLink(
        id = "l1",
        entityId = "e1",
        entityName = "Acme",
        kind = EntityKind.ORGANISATION,
        role = "employee",
        isPrimary = true,
        projectsAs = "ORG",
        revision = 2
    )

    @Before
    fun setUp() {
        coEvery { repository.linksForContact(5) } returns Result.success(listOf(acmeLink))
        coEvery { contacts.getContact(5) } returns Result.success(
            ContactRecordResponse(
                id = 5,
                card = Card(
                    organizations = listOf(
                        Organization(id = "o1", name = "Globex"),
                        Organization(id = "ent-l1", name = "Acme")
                    ),
                    addresses = listOf(Address(id = "a1"))
                )
            )
        )
    }

    private fun vm() = ContactEntityLinksViewModel(
        SavedStateHandle(mapOf("contactId" to 5)),
        repository,
        contacts
    )

    @Test
    fun `loads the links and the contact's own organizations and addresses, never the projected one`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = vm()
            advanceUntilIdle()
            val s = vm.uiState.value
            assertEquals(listOf("l1"), s.links.map { it.id })
            assertEquals(listOf("o1"), s.ownOrganizations.map { it.id })
            assertEquals(listOf("a1"), s.ownAddresses.map { it.id })
        }

    @Test
    fun `linking an existing entity is never made primary on its own`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val sent = mutableListOf<ContactEntityLinkInput>()
            coEvery { repository.createLink(5, capture(sent)) } returns
                Result.success(acmeLink.copy(id = "l2", isPrimary = false))
            val vm = vm()
            advanceUntilIdle()
            var done = false
            vm.link(
                LinkForm(entityId = "e2", kind = EntityKind.ORGANISATION, role = "owner")
            ) { done = true }
            advanceUntilIdle()
            assertTrue(done)
            assertEquals("e2", sent.single().entityId)
            assertEquals("owner", sent.single().role)
            assertEquals(false, sent.single().isPrimary)
        }

    @Test
    fun `typing a new name creates the entity first and links it`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.create(EntityInput(kind = "pet", name = "Rex")) } returns
                Result.success(Entity(id = "e9", kind = "pet", name = "Rex"))
            val sent = mutableListOf<ContactEntityLinkInput>()
            coEvery { repository.createLink(5, capture(sent)) } returns Result.success(acmeLink)
            val vm = vm()
            advanceUntilIdle()
            vm.link(LinkForm(newEntityName = " Rex ", kind = "pet", role = "owner")) {}
            advanceUntilIdle()
            assertEquals("e9", sent.single().entityId)
        }

    @Test
    fun `a failed entity creation never creates a link`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery {
            repository.create(
                any()
            )
        } returns Result.failure(ApiError.Unknown(RuntimeException("x")))
        val vm = vm()
        advanceUntilIdle()
        var done = false
        vm.link(LinkForm(newEntityName = "Rex", kind = "pet", role = "owner")) { done = true }
        advanceUntilIdle()
        assertFalse(done)
        assertNotNull(vm.uiState.value.formError)
        coVerify(exactly = 0) { repository.createLink(any(), any()) }
    }

    @Test
    fun `the period years become a link period and a bad range is rejected before any call`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val sent = mutableListOf<ContactEntityLinkInput>()
            coEvery { repository.createLink(5, capture(sent)) } returns Result.success(acmeLink)
            val vm = vm()
            advanceUntilIdle()
            vm.link(
                LinkForm(
                    entityId = "e2",
                    kind = EntityKind.ORGANISATION,
                    role = "employee",
                    startYear = "2019",
                    endYear = "2024"
                )
            ) {}
            advanceUntilIdle()
            assertEquals(
                TemporalRange(PartialDate(year = 2019), PartialDate(year = 2024)),
                sent.single().period
            )
            var done = false
            vm.link(
                LinkForm(
                    entityId = "e2",
                    kind = EntityKind.ORGANISATION,
                    role = "employee",
                    startYear = "2024",
                    endYear = "2019"
                )
            ) { done = true }
            advanceUntilIdle()
            assertFalse(done)
            assertTrue(vm.uiState.value.periodInvalid)
            coVerify(exactly = 1) { repository.createLink(any(), any()) }
        }

    @Test
    fun `toggling primary updates the link with its revision and keeps its role`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val sent = mutableListOf<ContactEntityLinkInput>()
            coEvery { repository.updateLink(5, "l1", capture(sent), 2) } returns
                Result.success(acmeLink.copy(isPrimary = false, revision = 3))
            val vm = vm()
            advanceUntilIdle()
            // The list is re-read from the server afterwards (a new primary demotes another).
            coEvery { repository.linksForContact(5) } returns
                Result.success(listOf(acmeLink.copy(isPrimary = false, revision = 3)))
            vm.setPrimary(acmeLink, false)
            advanceUntilIdle()
            assertEquals("employee", sent.single().role)
            assertEquals(false, sent.single().isPrimary)
            assertEquals(false, vm.uiState.value.links.single().isPrimary)
        }

    @Test
    fun `toggling primary resends the entity, role label, period and sensitivity unchanged`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val period = TemporalRange(
                start = PartialDate(year = 2019),
                end = PartialDate(year = 2023, month = 6)
            )
            val richLink = acmeLink.copy(
                roleLabel = "Head chef",
                period = period,
                sensitivity = "private"
            )
            coEvery { repository.linksForContact(5) } returns Result.success(listOf(richLink))
            val sent = mutableListOf<ContactEntityLinkInput>()
            coEvery { repository.updateLink(5, "l1", capture(sent), 2) } returns
                Result.success(richLink.copy(isPrimary = false, revision = 3))
            val vm = vm()
            advanceUntilIdle()
            vm.setPrimary(richLink, false)
            advanceUntilIdle()
            val input = sent.single()
            assertEquals("e1", input.entityId)
            assertEquals("employee", input.role)
            assertEquals("Head chef", input.roleLabel)
            assertEquals(period, input.period)
            assertEquals("private", input.sensitivity)
            assertEquals(false, input.isPrimary)
        }

    @Test
    fun `a stale revision asks the user to reload instead of overwriting`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.updateLink(any(), any(), any(), any()) } returns
                Result.failure(ApiError.PreconditionFailed("stale"))
            val vm = vm()
            advanceUntilIdle()
            vm.setPrimary(acmeLink, false)
            advanceUntilIdle()
            assertTrue(vm.uiState.value.conflict)
            vm.onConflictLoadLatest()
            advanceUntilIdle()
            assertFalse(vm.uiState.value.conflict)
            coVerify(atLeast = 2) { repository.linksForContact(5) }
        }

    @Test
    fun `removing a link drops it from the list`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.deleteLink(5, "l1") } returns Result.success(Unit)
        val vm = vm()
        advanceUntilIdle()
        vm.remove(acmeLink)
        advanceUntilIdle()
        assertTrue(vm.uiState.value.links.isEmpty())
    }

    @Test
    fun `adopting reports what was copied and reloads the contact's own values`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.adopt(5, "l1") } returns
                Result.success(ContactEntityLinkAdoptResponse(link = acmeLink, adopted = true))
            val vm = vm()
            advanceUntilIdle()
            vm.adopt(acmeLink)
            advanceUntilIdle()
            assertEquals("Acme", vm.uiState.value.adoptedName)
            coVerify(atLeast = 2) { contacts.getContact(5) }
        }

    @Test
    fun `linking an own value defaults to creating a new entity and sends the element id`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val sent =
                mutableListOf<com.mycorrhizal.crm.model.network.ContactEntityLinkFromExistingInput>()
            coEvery { repository.linkFromExisting(5, capture(sent)) } returns Result.success(
                ContactEntityLinkFromExistingResponse(
                    link = acmeLink,
                    entity = Entity(id = "e5", name = "Globex"),
                    entityCreated = true
                )
            )
            val vm = vm()
            advanceUntilIdle()
            vm.linkOwnValue(
                OwnValue(kind = "organization", id = "o1", index = 0, label = "Globex"),
                existingEntityId = null,
                role = "employee"
            ) {}
            advanceUntilIdle()
            val input = sent.single()
            assertEquals("organization", input.source.kind)
            assertEquals("o1", input.source.id)
            assertNull(input.source.index)
            assertNull(input.entityId)
            assertEquals(false, input.isPrimary)
            assertEquals("Globex", vm.uiState.value.linkedOwnName)
        }

    @Test
    fun `an own value without an element id is addressed by its index`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val sent =
                mutableListOf<com.mycorrhizal.crm.model.network.ContactEntityLinkFromExistingInput>()
            coEvery { repository.linkFromExisting(5, capture(sent)) } returns Result.success(
                ContactEntityLinkFromExistingResponse(link = acmeLink, entity = Entity(name = "X"))
            )
            val vm = vm()
            advanceUntilIdle()
            vm.linkOwnValue(OwnValue("address", null, 2, "1 High St"), null, "resident") {}
            advanceUntilIdle()
            assertNull(sent.single().source.id)
            assertEquals(2, sent.single().source.index)
        }

    @Test
    fun `a matching organisation is offered for an own organization but never for an address`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.list(EntityKind.ORGANISATION, "Globex", null, any()) } returns
                Result.success(
                    EntityListResponse(
                        entities = listOf(
                            Entity(id = "e7", name = "Globex Corp"),
                            Entity(id = "e8", name = "globex")
                        )
                    )
                )
            val vm = vm()
            advanceUntilIdle()
            vm.findMatch(OwnValue("organization", "o1", 0, "Globex"))
            advanceUntilIdle()
            assertEquals("e8", vm.uiState.value.ownMatch?.id)
            vm.findMatch(OwnValue("address", "a1", 0, "1 High St"))
            advanceUntilIdle()
            assertNull(vm.uiState.value.ownMatch)
            coVerify(exactly = 1) { repository.list(any(), any(), any(), any()) }
        }

    @Test
    fun `entity search for the link picker is scoped to the chosen kind`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.list(EntityKind.PLACE, "ca", null, any()) } returns Result.success(
                EntityListResponse(
                    entities = listOf(Entity(id = "p1", name = "Cabin", kind = EntityKind.PLACE))
                )
            )
            val vm = vm()
            advanceUntilIdle()
            vm.searchEntities(EntityKind.PLACE, "ca")
            advanceUntilIdle()
            assertEquals(listOf("p1"), vm.uiState.value.searchResults.map { it.id })
        }
}
