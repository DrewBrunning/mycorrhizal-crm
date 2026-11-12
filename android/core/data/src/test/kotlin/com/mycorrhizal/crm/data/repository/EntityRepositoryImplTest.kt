package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.model.network.ContactEntityLink
import com.mycorrhizal.crm.model.network.ContactEntityLinkAdoptResponse
import com.mycorrhizal.crm.model.network.ContactEntityLinkInput
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.Entity
import com.mycorrhizal.crm.model.network.EntityContactLink
import com.mycorrhizal.crm.model.network.EntityDeleteResponse
import com.mycorrhizal.crm.model.network.EntityDismissResponse
import com.mycorrhizal.crm.model.network.EntityFromContactsCreated
import com.mycorrhizal.crm.model.network.EntityFromContactsGroup
import com.mycorrhizal.crm.model.network.EntityFromContactsInput
import com.mycorrhizal.crm.model.network.EntityFromContactsResponse
import com.mycorrhizal.crm.model.network.EntityInput
import com.mycorrhizal.crm.model.network.EntityMergeInput
import com.mycorrhizal.crm.model.network.EntityMergeResponse
import com.mycorrhizal.crm.model.network.EntityRestoreResponse
import com.mycorrhizal.crm.model.network.EntitySuggestionsResponse
import com.mycorrhizal.crm.model.network.EntityValueGroup
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * Issue #2302: a link or entity change moves what the server projects into a contact, so every
 * affected cached contact is re-fetched (the cache must not keep a stale projected ORG/ADR).
 */
class EntityRepositoryImplTest {

    private lateinit var api: ApiClient
    private lateinit var contacts: ContactRepository
    private lateinit var repository: EntityRepositoryImpl

    @Before
    fun setup() {
        api = mockk()
        contacts = mockk()
        coEvery { contacts.getContact(any()) } returns Result.success(ContactRecordResponse())
        repository = EntityRepositoryImpl(api, contacts)
    }

    private fun person(contactId: Int) = EntityContactLink(
        linkId = "l$contactId",
        contactId = contactId
    )

    @Test
    fun `creating a link refreshes that contact`() = runTest {
        coEvery {
            api.createContactEntityLink(
                5,
                any()
            )
        } returns Result.success(ContactEntityLink(id = "l1"))
        repository.createLink(5, ContactEntityLinkInput(entityId = "e1", role = "employee"))
        coVerify(exactly = 1) { contacts.getContact(5) }
    }

    @Test
    fun `editing, removing and adopting a link refresh the contact too`() = runTest {
        coEvery {
            api.updateContactEntityLink(
                5,
                "l1",
                any(),
                2
            )
        } returns Result.success(ContactEntityLink())
        coEvery { api.deleteContactEntityLink(5, "l1") } returns Result.success(Unit)
        coEvery { api.adoptContactEntityLink(5, "l1") } returns
            Result.success(ContactEntityLinkAdoptResponse(adopted = true))
        repository.updateLink(5, "l1", ContactEntityLinkInput(role = "owner"), 2)
        repository.deleteLink(5, "l1")
        repository.adopt(5, "l1")
        coVerify(exactly = 3) { contacts.getContact(5) }
    }

    @Test
    fun `a failed link write does not refresh anything`() = runTest {
        coEvery {
            api.deleteContactEntityLink(
                5,
                "l1"
            )
        } returns Result.failure(ApiError.Unknown(RuntimeException("x")))
        assertTrue(repository.deleteLink(5, "l1").isFailure)
        coVerify(exactly = 0) { contacts.getContact(any()) }
    }

    @Test
    fun `a failed refresh never fails the write`() = runTest {
        coEvery {
            api.createContactEntityLink(
                5,
                any()
            )
        } returns Result.success(ContactEntityLink(id = "l1"))
        coEvery {
            contacts.getContact(
                5
            )
        } returns Result.failure(ApiError.Network(java.io.IOException()))
        val result = repository.createLink(
            5,
            ContactEntityLinkInput(entityId = "e1", role = "owner")
        )
        assertEquals("l1", result.getOrThrow().id)
    }

    @Test
    fun `renaming an entity refreshes the contacts linked to it`() = runTest {
        coEvery {
            api.listEntityContacts(
                "e1"
            )
        } returns Result.success(listOf(person(7), person(8)))
        coEvery { api.updateEntity("e1", any(), 3) } returns Result.success(Entity(id = "e1"))
        repository.update("e1", EntityInput(name = "Acme 2"), 3)
        coVerify { contacts.getContact(7) }
        coVerify { contacts.getContact(8) }
    }

    @Test
    fun `deleting an entity refreshes the contacts it was linked to, only after it succeeds`() =
        runTest {
            coEvery { api.listEntityContacts("e1") } returns Result.success(listOf(person(7)))
            coEvery {
                api.deleteEntity(
                    "e1",
                    3
                )
            } returns Result.failure(ApiError.Unknown(RuntimeException("x")))
            assertTrue(repository.delete("e1", 3).isFailure)
            coVerify(exactly = 0) { contacts.getContact(any()) }

            coEvery {
                api.deleteEntity(
                    "e1",
                    3
                )
            } returns Result.success(EntityDeleteResponse(linkCount = 1))
            repository.delete("e1", 3)
            coVerify(exactly = 1) { contacts.getContact(7) }
        }

    @Test
    fun `merging refreshes the loser's contacts`() = runTest {
        coEvery { api.listEntityContacts("loser") } returns Result.success(listOf(person(9)))
        coEvery { api.mergeEntity("loser", any()) } returns Result.success(EntityMergeResponse())
        repository.merge("loser", EntityMergeInput(into = "keep"))
        coVerify(exactly = 1) { contacts.getContact(9) }
    }

    @Test
    fun `suggestions and dismissal go straight to the api and touch no contact`() = runTest {
        val response = EntitySuggestionsResponse(mode = "pairs")
        coEvery { api.listEntitySuggestions("place") } returns Result.success(response)
        coEvery { api.dismissEntitySuggestion("a", "b") } returns
            Result.success(EntityDismissResponse(idLow = "a", idHigh = "b"))

        assertEquals(response, repository.suggestions("place").getOrThrow())
        assertTrue(repository.dismissSuggestion("a", "b").isSuccess)
        coVerify(exactly = 0) { contacts.getContact(any()) }

        coEvery { api.dismissEntitySuggestion("a", "b") } returns
            Result.failure(ApiError.Network(java.io.IOException("offline")))
        assertTrue(repository.dismissSuggestion("a", "b").isFailure)
    }

    @Test
    fun `value groups go straight to the api and touch no contact`() = runTest {
        val response = EntitySuggestionsResponse(
            mode = "contact_values",
            groups = listOf(EntityValueGroup(key = "k"))
        )
        coEvery { api.listEntityValueGroups("place", "c1") } returns Result.success(response)
        assertEquals(response, repository.valueGroups("place", "c1").getOrThrow())
        coVerify(exactly = 0) { contacts.getContact(any()) }
    }

    @Test
    fun `creating from contacts refreshes only the contacts of groups that were created`() =
        runTest {
            val input = EntityFromContactsInput(
                kind = "organisation",
                groups = listOf(
                    EntityFromContactsGroup("k1", "Acme", "employee", true, listOf(7, 8)),
                    EntityFromContactsGroup("k2", null, "employee", false, listOf(9))
                )
            )
            coEvery { api.createEntitiesFromContacts(input) } returns Result.success(
                EntityFromContactsResponse(created = listOf(EntityFromContactsCreated(key = "k1")))
            )
            assertTrue(repository.createFromContacts(input).isSuccess)
            coVerify(exactly = 1) { contacts.getContact(7) }
            coVerify(exactly = 1) { contacts.getContact(8) }
            coVerify(exactly = 0) { contacts.getContact(9) }
        }

    @Test
    fun `a failed create from contacts refreshes nothing`() = runTest {
        val input = EntityFromContactsInput(
            "place",
            listOf(EntityFromContactsGroup("k1", null, "resident", false, listOf(7)))
        )
        coEvery { api.createEntitiesFromContacts(input) } returns
            Result.failure(ApiError.Network(java.io.IOException("offline")))
        assertTrue(repository.createFromContacts(input).isFailure)
        coVerify(exactly = 0) { contacts.getContact(any()) }
    }

    @Test
    fun `restoring refreshes the revived links' contacts`() = runTest {
        coEvery {
            api.restoreEntity(
                "e1"
            )
        } returns Result.success(EntityRestoreResponse(linksRevived = 1))
        coEvery { api.listEntityContacts("e1") } returns Result.success(listOf(person(7)))
        repository.restore("e1")
        coVerify(exactly = 1) { contacts.getContact(7) }
    }

    @Test
    fun `the refresh is bounded so a huge entity does not stampede the server`() = runTest {
        coEvery { api.listEntityContacts("e1") } returns
            Result.success((1..120).map { person(it) })
        coEvery { api.updateEntity("e1", any(), null) } returns Result.success(Entity(id = "e1"))
        repository.update("e1", EntityInput(name = "x"), null)
        coVerify(
            exactly = EntityRepositoryImpl.MAX_REFRESHED_CONTACTS
        ) { contacts.getContact(any()) }
    }
}
