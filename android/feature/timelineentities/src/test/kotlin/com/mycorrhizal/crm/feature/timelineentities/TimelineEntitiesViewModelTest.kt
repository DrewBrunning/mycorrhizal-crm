package com.mycorrhizal.crm.feature.timelineentities

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.ActivityRepository
import com.mycorrhizal.crm.domain.repository.ContactActivitiesPage
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.GiftRepository
import com.mycorrhizal.crm.domain.repository.LifeEventRepository
import com.mycorrhizal.crm.domain.repository.PreferenceRepository
import com.mycorrhizal.crm.domain.repository.ConversationAgendaRepository
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.Gift
import com.mycorrhizal.crm.model.network.GiftStatuses
import com.mycorrhizal.crm.model.network.LifeEvent
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.model.network.PartialDate
import com.mycorrhizal.crm.model.network.Preference
import com.mycorrhizal.crm.model.network.PreferenceSensitivities
import com.mycorrhizal.crm.model.network.ConversationAgenda
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TestWatcher
import org.junit.runner.Description
import java.util.TimeZone

/**
 * The gift-date helpers convert between local dates and UTC instants using the
 * device zone (matching web). Tests assert exact ISO strings, so they pin the
 * zone to UTC to be runnable anywhere.
 */
class UtcTimezoneRule : TestWatcher() {
    private val original = TimeZone.getDefault()
    override fun starting(description: Description) {
        TimeZone.setDefault(TimeZone.getTimeZone("UTC"))
    }

    override fun finished(description: Description) {
        TimeZone.setDefault(original)
    }
}

private const val UID = "11111111-1111-1111-1111-111111111111"

private fun stubContact(contactRepository: ContactRepository) {
    coEvery { contactRepository.getContact(5) } returns Result.success(
        ContactRecordResponse(id = 5, card = Card(uid = UID, name = Name(full = "Dana White"))),
    )
}

private fun giftForm(
    status: String = GiftStatuses.IDEA,
    description: String = "Socks",
    url: String? = null,
    notes: String? = null,
    occasion: String? = null,
    date: String? = null,
    valueCents: Long? = null,
    currency: String? = null,
    lifeEventId: String? = null,
    activityId: Int? = null,
) = GiftFormData(
    status = status, description = description, url = url, notes = notes, occasion = occasion,
    date = date, valueCents = valueCents, currency = currency, lifeEventId = lifeEventId, activityId = activityId,
)

class LifeEventsViewModelTest {
    @get:Rule val mainDispatcherRule = MainDispatcherRule()
    private val repo = mockk<LifeEventRepository>()
    private val contacts = mockk<ContactRepository>()
    private fun vm() = LifeEventsViewModel(repo, contacts, SavedStateHandle(mapOf("contactId" to 5)))

    @Test fun `loads life events after resolving the uid`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact(contacts)
        coEvery { repo.listForContact(UID) } returns Result.success(
            listOf(LifeEvent(id = "e1", entityId = UID, type = "moved", description = "Moved to Madison")),
        )
        val vm = vm(); advanceUntilIdle()
        assertFalse(vm.uiState.value.isLoading)
        assertEquals(UID, vm.uiState.value.entityId)
        assertEquals(1, vm.uiState.value.items.size)
        assertNull(vm.uiState.value.error)
        // LifeEvent has no url-like field — EntityItem.url must default to
        // null, not carry over stale data from another entity type.
        assertNull(vm.uiState.value.items[0].url)
    }

    @Test fun `create posts every modeled field`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact(contacts)
        coEvery { repo.listForContact(UID) } returns Result.success(emptyList())
        coEvery { repo.create(any()) } returns Result.success(LifeEvent(id = "e1", entityId = UID, type = "moved"))
        val vm = vm(); advanceUntilIdle()

        vm.create(
            LifeEventFormData(
                type = "job_change",
                category = "work_education",
                description = "Started at Acme",
                date = PartialDate(year = 2026, month = 8),
                relatedEntityIds = listOf("u1", "u2"),
                remind = false,
            ),
        )
        advanceUntilIdle()

        coVerify {
            repo.create(
                match {
                    it.entityId == UID && it.type == "job_change" && it.category == "work_education" &&
                        it.description == "Started at Acme" &&
                        it.date?.year == 2026 && it.date?.month == 8 &&
                        it.relatedEntityIds == listOf("u1", "u2") && it.remind == false
                },
            )
        }
    }

    @Test fun `create with a partial date and remind flags remind true only when month+day are set`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubContact(contacts)
            coEvery { repo.listForContact(UID) } returns Result.success(emptyList())
            coEvery { repo.create(any()) } returns Result.success(LifeEvent(id = "e1"))
            val vm = vm(); advanceUntilIdle()

            vm.create(
                LifeEventFormData(
                    type = "married",
                    category = "family_relationships",
                    date = PartialDate(year = 2020, month = 6, day = 15),
                    remind = true,
                ),
            )
            advanceUntilIdle()

            coVerify { repo.create(match { it.remind == true && it.date?.hasMonthDay == true }) }
        }

    @Test fun `update preserves the unmodeled source field while applying the form`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubContact(contacts)
            val original = LifeEvent(
                id = "e1",
                entityId = UID,
                type = "moved",
                category = "family_relationships",
                description = "Moved to Madison",
                source = "manual",
                remind = true,
            )
            coEvery { repo.listForContact(UID) } returns Result.success(listOf(original))
            coEvery { repo.update("e1", any()) } returns Result.success(original)
            val vm = vm(); advanceUntilIdle()

            vm.update(
                original,
                LifeEventFormData(
                    type = "relocated",
                    category = "family_relationships",
                    description = "Moved to Chicago",
                    date = null,
                    relatedEntityIds = emptyList(),
                    remind = false,
                ),
            )
            advanceUntilIdle()

            coVerify {
                repo.update(
                    "e1",
                    match {
                        it.source == "manual" && it.type == "relocated" && it.description == "Moved to Chicago" &&
                            // Cleared fields persist as cleared, not carried forward.
                            it.date == null && it.relatedEntityIds == null && it.remind == false
                    },
                )
            }
        }

    @Test fun `uncategorized category is sent as null`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact(contacts)
        coEvery { repo.listForContact(UID) } returns Result.success(emptyList())
        coEvery { repo.create(any()) } returns Result.success(LifeEvent(id = "e1"))
        val vm = vm(); advanceUntilIdle()

        vm.create(LifeEventFormData(type = "got a new pet", category = null, description = "Adopted a cat"))
        advanceUntilIdle()

        coVerify { repo.create(match { it.category == null && it.type == "got a new pet" }) }
    }

    // M18: related-contact resolution + search for the life-event form.
    @Test fun `onDialogOpened resolves the initial related contacts by uid`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubContact(contacts)
            coEvery { repo.listForContact(UID) } returns Result.success(emptyList())
            coEvery { contacts.resolveByUid(listOf("u1", "u2")) } returns Result.success(
                mapOf(
                    "u1" to com.mycorrhizal.crm.model.network.ContactSummary(id = 1, uid = "u1", fn = "Alice"),
                    "u2" to com.mycorrhizal.crm.model.network.ContactSummary(id = 2, uid = "u2", fn = "Bob"),
                ),
            )
            val vm = vm(); advanceUntilIdle()

            vm.onDialogOpened(
                LifeEvent(id = "e1", entityId = UID, relatedEntityIds = listOf("u1", "u2")),
            )
            advanceUntilIdle()

            assertEquals(listOf("Alice", "Bob"), vm.relatedContacts.value.map { it.displayName })
        }

    @Test fun `related search excludes already-selected contacts`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubContact(contacts)
            coEvery { repo.listForContact(UID) } returns Result.success(emptyList())
            coEvery {
                contacts.listContacts(search = "a", limit = 25)
            } returns Result.success(
                com.mycorrhizal.crm.domain.repository.ContactsPage(
                    contacts = listOf(
                        com.mycorrhizal.crm.model.network.ContactSummary(id = 1, uid = "u1", fn = "Alice"),
                        com.mycorrhizal.crm.model.network.ContactSummary(id = 3, uid = "u3", fn = "Ava"),
                    ),
                    nextCursor = null, limit = 25, sync = null,
                ),
            )
            val vm = vm(); advanceUntilIdle()
            vm.addRelated(com.mycorrhizal.crm.model.network.ContactSummary(id = 1, uid = "u1", fn = "Alice"))

            vm.searchRelated("a")
            advanceUntilIdle()

            assertEquals(listOf("Ava"), vm.relatedSearchResults.value.map { it.displayName })
        }

    @Test fun `findById returns the loaded entity by id`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact(contacts)
        coEvery { repo.listForContact(UID) } returns Result.success(
            listOf(LifeEvent(id = "e1", entityId = UID, type = "moved", description = "Moved to Madison")),
        )
        val vm = vm(); advanceUntilIdle()
        assertEquals("Moved to Madison", vm.findById("e1")?.description)
        assertNull(vm.findById("no-such-id"))
    }
}

class GiftsViewModelTest {
    @get:Rule val mainDispatcherRule = MainDispatcherRule()
    @get:Rule val utcTimezone = UtcTimezoneRule()
    private val repo = mockk<GiftRepository>()
    private val lifeEventRepo = mockk<LifeEventRepository>()
    private val activityRepo = mockk<ActivityRepository>()
    private val contacts = mockk<ContactRepository>()
    private fun vm(): GiftsViewModel {
        // load() always fetches the picker lists; stub them empty by default so
        // tests that don't care about pickers don't trip on the missing answer.
        coEvery { lifeEventRepo.listForContact(UID) } returns Result.success(emptyList())
        coEvery { activityRepo.listForContact(5, any(), any(), any(), any(), any()) } returns
            Result.success(ContactActivitiesPage(activities = emptyList(), nextCursor = null))
        return GiftsViewModel(repo, lifeEventRepo, activityRepo, contacts, SavedStateHandle(mapOf("contactId" to 5)))
    }

    @Test fun `loads gifts, picker lists, and exposes url`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact(contacts)
        coEvery { repo.listForContact(UID) } returns Result.success(
            listOf(Gift(id = "g1", entityId = UID, description = "Socks", url = "https://example.com/socks")),
        )
        val vm = vm()
        // Stub the pickers AFTER vm() so its defaults don't clobber them.
        coEvery { lifeEventRepo.listForContact(UID) } returns Result.success(
            listOf(LifeEvent(id = "le1", entityId = UID, type = "married")),
        )
        coEvery { activityRepo.listForContact(5, any(), any(), any(), any(), any()) } returns
            Result.success(ContactActivitiesPage(activities = listOf(Activity(id = 7, title = "Coffee")), nextCursor = null))
        advanceUntilIdle()
        assertEquals(1, vm.uiState.value.items.size)
        assertEquals("Socks", vm.uiState.value.items[0].label)
        assertEquals("https://example.com/socks", vm.uiState.value.items[0].url)
        // The picker lists feed the gift form's life-event/activity selects.
        assertEquals("le1", vm.lifeEvents.value[0].id)
        assertEquals(7, vm.activities.value[0].id)
    }

    @Test fun `create posts the full field set`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact(contacts)
        coEvery { repo.listForContact(UID) } returns Result.success(emptyList())
        coEvery { repo.create(any()) } returns Result.success(Gift(id = "g1"))
        val vm = vm(); advanceUntilIdle()

        vm.create(
            giftForm(
                status = GiftStatuses.PURCHASED,
                description = "Wool socks",
                url = "https://example.com/socks",
                notes = "Size L",
                occasion = "Birthday",
                date = "2026-08-10",
                valueCents = 2500,
                currency = "eur",
                lifeEventId = "le1",
                activityId = 7,
            ),
        )
        advanceUntilIdle()

        coVerify {
            repo.create(
                match {
                    it.entityId == UID && it.status == "purchased" && it.description == "Wool socks" &&
                        it.url == "https://example.com/socks" && it.notes == "Size L" &&
                        it.occasion == "Birthday" && it.date == "2026-08-10T00:00:00Z" &&
                        it.valueCents == 2500L && it.currency == "EUR" &&
                        it.lifeEventId == "le1" && it.activityId == 7
                },
            )
        }
    }

    @Test fun `update applies the form and clears fields the user emptied`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubContact(contacts)
            val original = Gift(
                id = "g1",
                entityId = UID,
                status = "given",
                description = "Socks",
                url = "https://example.com/socks",
                notes = "Wool, size L",
                valueCents = 2500,
                currency = "USD",
            )
            coEvery { repo.listForContact(UID) } returns Result.success(listOf(original))
            coEvery { repo.update("g1", any()) } returns Result.success(original)
            val vm = vm(); advanceUntilIdle()

            // The user cleared url/notes/value and changed the date.
            vm.update(
                original,
                giftForm(status = "given", description = "Wool socks", date = "2026-12-01"),
            )
            advanceUntilIdle()

            coVerify {
                repo.update(
                    "g1",
                    match {
                        it.description == "Wool socks" && it.status == "given" &&
                            it.url == null && it.notes == null && it.valueCents == null && it.currency == null &&
                            it.date == "2026-12-01T00:00:00Z"
                    },
                )
            }
        }

    @Test fun `markGiven flips an idea gift to given and defaults the date to now`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubContact(contacts)
            val idea = Gift(id = "g1", entityId = UID, status = GiftStatuses.IDEA, description = "Socks")
            coEvery { repo.listForContact(UID) } returns Result.success(listOf(idea))
            coEvery { repo.update("g1", any()) } returns Result.success(idea.copy(status = GiftStatuses.GIVEN))
            val vm = vm(); advanceUntilIdle()

            vm.markGiven("g1")
            advanceUntilIdle()

            coVerify {
                repo.update(
                    "g1",
                    match { it.status == GiftStatuses.GIVEN && it.date != null },
                )
            }
        }

    @Test fun `markGiven keeps an existing date`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact(contacts)
        val gift = Gift(
            id = "g1", entityId = UID, status = GiftStatuses.IDEA, description = "Socks",
            date = "2026-01-01T00:00:00Z",
        )
        coEvery { repo.listForContact(UID) } returns Result.success(listOf(gift))
        coEvery { repo.update("g1", any()) } returns Result.success(gift.copy(status = GiftStatuses.GIVEN))
        val vm = vm(); advanceUntilIdle()

        vm.markGiven("g1")
        advanceUntilIdle()

        coVerify { repo.update("g1", match { it.date == "2026-01-01T00:00:00Z" }) }
    }

    @Test fun `markGiven is a no-op for an already-given or received gift`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubContact(contacts)
            val given = Gift(id = "g1", entityId = UID, status = GiftStatuses.GIVEN, description = "Socks")
            val received = Gift(id = "g2", entityId = UID, status = GiftStatuses.RECEIVED, description = "Mug")
            coEvery { repo.listForContact(UID) } returns Result.success(listOf(given, received))
            val vm = vm(); advanceUntilIdle()

            vm.markGiven("g1")
            vm.markGiven("g2")
            advanceUntilIdle()

            coVerify(exactly = 0) { repo.update(any(), any()) }
        }

    @Test fun `clearing the gift date persists as cleared`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubContact(contacts)
            val original = Gift(
                id = "g1", entityId = UID, status = "given", description = "Socks",
                date = "2026-01-01T00:00:00Z",
            )
            coEvery { repo.listForContact(UID) } returns Result.success(listOf(original))
            coEvery { repo.update("g1", any()) } returns Result.success(original)
            val vm = vm(); advanceUntilIdle()

            // The user emptied the date field.
            vm.update(original, giftForm(status = "given", description = "Socks", date = null))
            advanceUntilIdle()

            coVerify { repo.update("g1", match { it.date == null }) }
        }

    @Test fun `findById returns the loaded entity by id`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact(contacts)
        coEvery { repo.listForContact(UID) } returns Result.success(
            listOf(Gift(id = "g1", entityId = UID, description = "Socks")),
        )
        val vm = vm(); advanceUntilIdle()
        assertEquals("Socks", vm.findById("g1")?.description)
        assertNull(vm.findById("no-such-id"))
    }

    @Test fun `missing contact id surfaces an error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { contacts.getContact(0) } returns Result.failure(ApiError.Client(404, "gone"))
        val vm = GiftsViewModel(repo, lifeEventRepo, activityRepo, contacts, SavedStateHandle(mapOf("contactId" to 0)))
        advanceUntilIdle()
        assertFalse(vm.uiState.value.isLoading)
    }
}

class PreferencesViewModelTest {
    @get:Rule val mainDispatcherRule = MainDispatcherRule()
    private val repo = mockk<PreferenceRepository>()
    private val contacts = mockk<ContactRepository>()
    private fun vm() = PreferencesViewModel(repo, contacts, SavedStateHandle(mapOf("contactId" to 5)))

    @Test fun `loads preferences grouped by section`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact(contacts)
        coEvery { repo.listForContact(UID) } returns Result.success(
            listOf(
                Preference(id = "p1", entityId = UID, category = "food", key = "allergy", value = "peanuts"),
                Preference(id = "p2", entityId = UID, category = "media", value = "Neon Genesis"),
                Preference(id = "p3", entityId = UID, category = "hobby", value = "pottery"),
            ),
        )
        val vm = vm(); advanceUntilIdle()
        assertEquals(3, vm.uiState.value.items.size)
        // M18: section grouping mirrors web's PreferenceList (food+drink / media / other).
        assertEquals(listOf("food_drink", "media", "other"), vm.uiState.value.items.map { it.sectionKey })
        assertEquals("food: allergy = peanuts", vm.uiState.value.items[0].label)
    }

    @Test fun `interleaved preferences are sorted into contiguous sections`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubContact(contacts)
            // Server order is updated_at desc, which interleaves sections; the
            // ViewModel must group them contiguously so the scaffold's
            // change-detecting headers can't repeat (review-pass fix).
            coEvery { repo.listForContact(UID) } returns Result.success(
                listOf(
                    Preference(id = "p1", entityId = UID, category = "media", value = "Movie"),
                    Preference(id = "p2", entityId = UID, category = "food", value = "spicy"),
                    Preference(id = "p3", entityId = UID, category = "drink", value = "tea"),
                    Preference(id = "p4", entityId = UID, category = "hobby", value = "pottery"),
                    Preference(id = "p5", entityId = UID, category = "media", value = "Album"),
                ),
            )
            val vm = vm(); advanceUntilIdle()
            // Each section appears exactly once, in order.
            assertEquals(
                listOf("food_drink", "food_drink", "media", "media", "other"),
                vm.uiState.value.items.map { it.sectionKey },
            )
        }

    @Test fun `clothing-size preferences are surfaced for the dedicated panel`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubContact(contacts)
            coEvery { repo.listForContact(UID) } returns Result.success(
                listOf(
                    Preference(id = "p1", entityId = UID, category = "food", value = "spicy"),
                    Preference(id = "c1", entityId = UID, category = "clothing_size", value = "M"),
                ),
            )
            val vm = vm(); advanceUntilIdle()
            // The panel's items are NOT part of the grouped list.
            assertEquals(1, vm.uiState.value.items.size)
            assertEquals(listOf("M"), vm.clothingSizes.map { it.value })
        }

    @Test fun `create sends category, key, value and sensitivity`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact(contacts)
        coEvery { repo.listForContact(UID) } returns Result.success(emptyList())
        coEvery { repo.create(any()) } returns Result.success(Preference(id = "p1"))
        val vm = vm(); advanceUntilIdle()

        vm.create(
            PreferenceFormData(
                category = "drink",
                key = "favorite",
                value = "matcha",
                sensitivity = PreferenceSensitivities.PRIVATE,
            ),
        )
        advanceUntilIdle()

        coVerify {
            repo.create(
                match {
                    it.entityId == UID && it.category == "drink" && it.key == "favorite" &&
                        it.value == "matcha" && it.sensitivity == "private"
                },
            )
        }
    }

    @Test fun `update preserves unmodeled fields while the form is authoritative`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubContact(contacts)
            val original = Preference(
                id = "p1", entityId = UID, category = "food", key = "allergy", value = "peanuts",
                source = "manual", confidence = 0.5, sensitivity = "private",
            )
            coEvery { repo.listForContact(UID) } returns Result.success(listOf(original))
            coEvery { repo.update("p1", any()) } returns Result.success(original)
            val vm = vm(); advanceUntilIdle()

            // The user cleared the key and changed sensitivity back to normal.
            vm.update(
                original,
                PreferenceFormData(
                    category = "food",
                    key = null,
                    value = "tree nuts",
                    sensitivity = PreferenceSensitivities.NORMAL,
                ),
            )
            advanceUntilIdle()

            coVerify {
                repo.update(
                    "p1",
                    match {
                        it.value == "tree nuts" && it.key == null && it.sensitivity == "normal" &&
                            it.source == "manual" && it.confidence == 0.5
                    },
                )
            }
        }

    @Test fun `createClothingSize posts a clothing_size preference`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact(contacts)
        coEvery { repo.listForContact(UID) } returns Result.success(emptyList())
        coEvery { repo.create(any()) } returns Result.success(Preference(id = "c1"))
        val vm = vm(); advanceUntilIdle()

        vm.createClothingSize("42")
        advanceUntilIdle()

        coVerify { repo.create(match { it.category == "clothing_size" && it.value == "42" }) }
    }

    @Test fun `findById returns the loaded entity by id`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact(contacts)
        coEvery { repo.listForContact(UID) } returns Result.success(
            listOf(Preference(id = "p1", entityId = UID, category = "food", value = "peanuts")),
        )
        val vm = vm(); advanceUntilIdle()
        assertEquals("food", vm.findById("p1")?.category)
        assertNull(vm.findById("no-such-id"))
    }
}

class ConversationAgendaViewModelTest {
    @get:Rule val mainDispatcherRule = MainDispatcherRule()
    private val repo = mockk<ConversationAgendaRepository>()
    private val activityRepo = mockk<ActivityRepository>()
    private val contacts = mockk<ContactRepository>()
    private fun vm(): ConversationAgendaViewModel {
        coEvery { activityRepo.listForContact(5, any(), any(), any(), any(), any()) } returns
            Result.success(ContactActivitiesPage(activities = emptyList(), nextCursor = null))
        return ConversationAgendaViewModel(repo, activityRepo, contacts, SavedStateHandle(mapOf("contactId" to 5)))
    }

    @Test fun `loads agenda items split into open and discussed sections`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubContact(contacts)
            coEvery { repo.listForContact(UID) } returns Result.success(
                listOf(
                    ConversationAgenda(
                        id = "a1", entityId = UID, content = "Ask about the move",
                        referenceUrl = "https://example.com/listing",
                    ),
                    ConversationAgenda(
                        id = "a2", entityId = UID, content = "Discuss the wedding",
                        discussedAt = "2026-08-01T10:00:00Z",
                    ),
                ),
            )
            val vm = vm(); advanceUntilIdle()
            assertEquals(2, vm.uiState.value.items.size)
            assertEquals(listOf("open", "discussed"), vm.uiState.value.items.map { it.sectionKey })
            assertEquals("https://example.com/listing", vm.uiState.value.items[0].url)
        }

    @Test fun `interleaved open and discussed items are sorted into contiguous sections`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubContact(contacts)
            // Server order is updated_at desc — discussing bumps updated_at, so
            // a discussed item can sort FIRST between open items; the
            // ViewModel must group open-then-discussed contiguously.
            coEvery { repo.listForContact(UID) } returns Result.success(
                listOf(
                    ConversationAgenda(id = "a1", entityId = UID, content = "Done", discussedAt = "2026-08-01T10:00:00Z"),
                    ConversationAgenda(id = "a2", entityId = UID, content = "Open 1"),
                    ConversationAgenda(id = "a3", entityId = UID, content = "Done 2", discussedAt = "2026-08-02T10:00:00Z"),
                ),
            )
            val vm = vm(); advanceUntilIdle()
            assertEquals(
                listOf("open", "discussed", "discussed"),
                vm.uiState.value.items.map { it.sectionKey },
            )
        }

    @Test fun `create sends the reference url`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact(contacts)
        coEvery { repo.listForContact(UID) } returns Result.success(emptyList())
        coEvery { repo.create(any()) } returns Result.success(ConversationAgenda(id = "a1"))
        val vm = vm(); advanceUntilIdle()

        vm.create("Ask about the move", "https://example.com/listing")
        advanceUntilIdle()

        coVerify {
            repo.create(
                match {
                    it.entityId == UID && it.content == "Ask about the move" &&
                        it.referenceUrl == "https://example.com/listing"
                },
            )
        }
    }

    @Test fun `update applies a cleared reference url as cleared`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact(contacts)
        val original = ConversationAgenda(
            id = "a1", entityId = UID, content = "Ask about the move",
            referenceUrl = "https://example.com/listing",
        )
        coEvery { repo.listForContact(UID) } returns Result.success(listOf(original))
        coEvery { repo.update("a1", any()) } returns Result.success(original)
        val vm = vm(); advanceUntilIdle()

        vm.update(original, "Ask about the new place", null)
        advanceUntilIdle()

        coVerify { repo.update("a1", match { it.referenceUrl == null && it.content == "Ask about the new place" }) }
    }

    @Test fun `markDiscussed links the chosen activity`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact(contacts)
        coEvery { repo.listForContact(UID) } returns Result.success(emptyList())
        coEvery { repo.discuss("a1", 7) } returns Result.success(
            ConversationAgenda(id = "a1", entityId = UID, content = "Ask", activityId = 7),
        )
        val vm = vm(); advanceUntilIdle()

        vm.markDiscussed("a1", 7)
        advanceUntilIdle()

        coVerify { repo.discuss("a1", 7) }
    }

    @Test fun `markDiscussed without an activity links none`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact(contacts)
        coEvery { repo.listForContact(UID) } returns Result.success(emptyList())
        coEvery { repo.discuss("a1", null) } returns Result.success(
            ConversationAgenda(id = "a1", entityId = UID, content = "Ask"),
        )
        val vm = vm(); advanceUntilIdle()

        vm.markDiscussed("a1", null)
        advanceUntilIdle()

        coVerify { repo.discuss("a1", null) }
    }

    @Test fun `findById returns the loaded entity by id`() = runTest(mainDispatcherRule.testDispatcher) {
        stubContact(contacts)
        coEvery { repo.listForContact(UID) } returns Result.success(
            listOf(ConversationAgenda(id = "a1", entityId = UID, content = "Ask about the move")),
        )
        val vm = vm(); advanceUntilIdle()
        assertEquals("Ask about the move", vm.findById("a1")?.content)
        assertNull(vm.findById("no-such-id"))
    }
}

// --- M18 pure helpers ---

class GiftDateHelpersTest {
    @get:Rule val utcTimezone = UtcTimezoneRule()

    @Test fun `giftDateToIso converts a local date to a UTC midnight instant`() {
        assertEquals("2026-08-10T00:00:00Z", giftDateToIso("2026-08-10"))
    }

    @Test fun `giftDateToIso rejects malformed or blank input`() {
        assertEquals(null, giftDateToIso("not-a-date"))
        assertEquals(null, giftDateToIso("  "))
    }

    @Test fun `giftDateFromIso reads the local date part`() {
        assertEquals("2026-08-10", giftDateFromIso("2026-08-10T00:00:00Z"))
        assertEquals("2026-08-10", giftDateFromIso("2026-08-10T22:00:00.000Z"))
    }

    @Test fun `giftDateFromIso tolerates blank and garbage`() {
        assertEquals("", giftDateFromIso(null))
        assertEquals("", giftDateFromIso(""))
        assertEquals("", giftDateFromIso("garbage"))
    }

    @Test fun `preferenceSection groups food and drink together`() {
        assertEquals("food_drink", preferenceSection("food"))
        assertEquals("food_drink", preferenceSection("drink"))
        assertEquals("media", preferenceSection("media"))
        assertEquals("other", preferenceSection("hobby"))
        assertEquals("other", preferenceSection("clothing_size"))
    }
}
