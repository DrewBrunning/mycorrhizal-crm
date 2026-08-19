package com.mycorrhizal.crm.feature.timelineentities

import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.runtime.Composable
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertIsToggleable
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextReplacement
import com.mycorrhizal.crm.model.network.ConversationAgenda
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.Gift
import com.mycorrhizal.crm.model.network.GiftStatuses
import com.mycorrhizal.crm.model.network.LifeEvent
import com.mycorrhizal.crm.model.network.Preference
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

// M18 (100-M18-android-entity-field-richness.md). Two layers, matching the
// module's existing test split:
//
//   1. EntityListScaffoldTest -- the shared delete-confirmation + tap-to-edit
//      mechanics, now also covering the M18 section headers.
//   2. One test class per entity dialog -- each form's fields round-trip into
//      onConfirm, and edit mode pre-fills from the loaded entity.

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@OptIn(ExperimentalMaterial3Api::class)
class EntityListScaffoldTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(
        items: List<EntityItem>,
        sectionLabel: (@Composable (String) -> String)? = null,
        onItemClick: (String) -> Unit = {},
        onDelete: (String) -> Unit = {},
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                EntityListScaffold(
                    title = "Life events",
                    addLabel = "New life event",
                    uiState = EntityListUiState(items = items),
                    onAdd = {},
                    onItemClick = onItemClick,
                    onDelete = onDelete,
                    onErrorShown = {},
                    onBack = {},
                    sectionLabel = sectionLabel,
                    dialog = {},
                )
            }
        }
    }

    @Test
    fun `tapping a row invokes onItemClick`() {
        var clickedId: String? = null
        setContent(
            items = listOf(EntityItem(id = "e1", label = "Moved to Madison")),
            onItemClick = { clickedId = it },
        )
        composeTestRule.onNodeWithText("Moved to Madison").performClick()
        assertEquals("e1", clickedId)
    }

    @Test
    fun `delete asks first -- tapping delete shows a confirmation and does not call onDelete`() {
        var deletedId: String? = null
        setContent(
            items = listOf(EntityItem(id = "e1", label = "Moved to Madison")),
            onDelete = { deletedId = it },
        )
        composeTestRule.onNodeWithContentDescription("Delete").performClick()

        composeTestRule.onNodeWithText("Delete item?").assertIsDisplayed()
        composeTestRule.onNodeWithText("Delete “Moved to Madison”? This cannot be undone.").assertIsDisplayed()
        assertNull(deletedId)
    }

    @Test
    fun `confirming the dialog calls onDelete with the right id`() {
        var deletedId: String? = null
        setContent(
            items = listOf(EntityItem(id = "e1", label = "Moved to Madison")),
            onDelete = { deletedId = it },
        )
        composeTestRule.onNodeWithContentDescription("Delete").performClick()
        composeTestRule.onNodeWithText("Delete").performClick()

        assertEquals("e1", deletedId)
    }

    @Test
    fun `section headers render between groups and the extra action slot is invoked`() {
        var extraActionCount = 0
        composeTestRule.setContent {
            MycorrhizalTheme {
                EntityListScaffold(
                    title = "Agenda",
                    addLabel = "New item",
                    uiState = EntityListUiState(
                        items = listOf(
                            EntityItem(id = "a1", label = "Open item", sectionKey = "open"),
                            EntityItem(id = "a2", label = "Done item", sectionKey = "discussed"),
                        ),
                    ),
                    onAdd = {},
                    onItemClick = {},
                    onDelete = {},
                    onErrorShown = {},
                    onBack = {},
                    sectionLabel = { section -> if (section == "discussed") "Discussed" else "Open" },
                    extraAction = {
                        extraActionCount++
                    },
                    dialog = {},
                )
            }
        }

        composeTestRule.onNodeWithText("Open").assertIsDisplayed()
        composeTestRule.onNodeWithText("Discussed").assertIsDisplayed()
        composeTestRule.onNodeWithText("Open item").assertIsDisplayed()
        composeTestRule.onNodeWithText("Done item").assertIsDisplayed()
        assertEquals(2, extraActionCount)
    }
}

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class LifeEventDialogTest {
    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(
        initial: LifeEvent? = null,
        onConfirm: (LifeEventFormData) -> Unit = {},
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                LifeEventDialog(
                    initial = initial,
                    relatedContacts = emptyList(),
                    contactSearchQuery = "",
                    contactSearchResults = emptyList(),
                    contactSearchLoading = false,
                    onSearchContacts = {},
                    onAddRelated = {},
                    onRemoveRelated = {},
                    onConfirm = onConfirm,
                    onDismiss = {},
                )
            }
        }
    }

    @Test
    fun `a new event requires a category before the type select is enabled`() {
        var confirmed: LifeEventFormData? = null
        setContent(onConfirm = { confirmed = it })

        // No category chosen yet: the type control is disabled, so Create is blocked.
        composeTestRule.onNodeWithTag("life-event-type").assertIsNotEnabled()

        // Pick the family category, then a category-scoped type.
        composeTestRule.onNodeWithTag("life-event-category").performClick()
        composeTestRule.onNodeWithText("Family & Relationships").performClick()
        composeTestRule.onNodeWithTag("life-event-type").performClick()
        composeTestRule.onNodeWithText("married").performClick()

        composeTestRule.onNodeWithText("Description").performTextReplacement("We got married")
        composeTestRule.onNodeWithText("Create").performClick()

        assertEquals("married", confirmed?.type)
        assertEquals("family_relationships", confirmed?.category)
        assertEquals("We got married", confirmed?.description)
    }

    @Test
    fun `partial date with month and day enables the yearly reminder`() {
        var confirmed: LifeEventFormData? = null
        setContent(onConfirm = { confirmed = it })

        composeTestRule.onNodeWithTag("life-event-category").performClick()
        composeTestRule.onNodeWithText("Family & Relationships").performClick()
        composeTestRule.onNodeWithTag("life-event-type").performClick()
        composeTestRule.onNodeWithText("anniversary").performClick()

        // Month+day present enables the reminder checkbox; then confirm.
        composeTestRule.onNodeWithText("Month").performScrollTo().performTextReplacement("6")
        composeTestRule.onNodeWithText("Day").performScrollTo().performTextReplacement("15")
        // #199: Modifier.toggleable on the row merges "Remind me yearly" into
        // the checkbox's own accessible name -- previously an unnamed
        // Checkbox sat next to a plain, unassociated Text.
        composeTestRule.onNodeWithText("Remind me yearly").performScrollTo().assertIsToggleable()
        composeTestRule.onNodeWithTag("life-event-remind").performScrollTo().performClick()
        composeTestRule.onNodeWithText("Create").performClick()

        assertEquals(6, confirmed?.date?.month)
        assertEquals(15, confirmed?.date?.day)
        assertEquals(true, confirmed?.remind)
    }

    @Test
    fun `a year-only date cannot enable the reminder`() {
        var confirmed: LifeEventFormData? = null
        setContent(onConfirm = { confirmed = it })

        composeTestRule.onNodeWithTag("life-event-category").performClick()
        composeTestRule.onNodeWithText("Family & Relationships").performClick()
        composeTestRule.onNodeWithTag("life-event-type").performClick()
        composeTestRule.onNodeWithText("anniversary").performClick()

        composeTestRule.onNodeWithText("Year").performScrollTo().performTextReplacement("2019")
        composeTestRule.onNodeWithTag("life-event-remind").performScrollTo().performClick()
        composeTestRule.onNodeWithText("Create").performClick()

        // Remind is forced false when month/day are absent.
        assertEquals(2019, confirmed?.date?.year)
        assertEquals(false, confirmed?.remind)
    }

    @Test
    fun `edit mode pre-fills every modeled field`() {
        var confirmed: LifeEventFormData? = null
        setContent(
            initial = LifeEvent(
                id = "e1",
                entityId = "uid",
                type = "job_change",
                category = "work_education",
                description = "Started at Acme",
                date = com.mycorrhizal.crm.model.network.PartialDate(year = 2024, month = 3),
            ),
            onConfirm = { confirmed = it },
        )

        composeTestRule.onNodeWithText("Edit life event").assertIsDisplayed()
        composeTestRule.onNodeWithText("job change").assertIsDisplayed()
        composeTestRule.onNodeWithText("2024").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("3").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Started at Acme").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `a legacy event with no category falls back to the uncategorized sentinel and keeps its type editable`() {
        var confirmed: LifeEventFormData? = null
        setContent(
            // Pre-T36 rows carry category NULL, which Go serializes as an
            // absent JSON key — the dialog must land on the sentinel, not the
            // empty "select a category" state (review-pass fix).
            initial = LifeEvent(
                id = "e1",
                entityId = "uid",
                type = "went to the lake",
                description = "Summer trip",
            ),
            onConfirm = { confirmed = it },
        )

        composeTestRule.onNodeWithText("Other / Uncategorized").assertIsDisplayed()
        // The type is free text under the sentinel, and editable.
        composeTestRule.onNodeWithText("went to the lake").assertIsDisplayed()
        composeTestRule.onNodeWithText("went to the lake").performTextReplacement("went to the sea")
        composeTestRule.onNodeWithText("Save").performClick()

        // Saving sends category null (never the sentinel string).
        assertEquals(null, confirmed?.category)
        assertEquals("went to the sea", confirmed?.type)
    }

    @Test
    fun `removing a related contact invokes onRemoveRelated`() {
        var removed: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                LifeEventDialog(
                    initial = null,
                    relatedContacts = listOf(ContactSummary(id = 1, uid = "u1", fn = "Alice")),
                    contactSearchQuery = "",
                    contactSearchResults = emptyList(),
                    contactSearchLoading = false,
                    onSearchContacts = {},
                    onAddRelated = {},
                    onRemoveRelated = { removed = it },
                    onConfirm = {},
                    onDismiss = {},
                )
            }
        }

        composeTestRule.onNodeWithContentDescription("Remove Alice").performClick()

        assertEquals("u1", removed)
    }
}

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class GiftDialogTest {
    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(
        initial: Gift? = null,
        onConfirm: (GiftFormData) -> Unit = {},
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                GiftDialog(
                    initial = initial,
                    lifeEvents = emptyList(),
                    activities = emptyList(),
                    onConfirm = onConfirm,
                    onDismiss = {},
                )
            }
        }
    }

    @Test
    fun `status defaults to idea and can be changed`() {
        var confirmed: GiftFormData? = null
        setContent(onConfirm = { confirmed = it })

        composeTestRule.onNodeWithTag("gift-status").performClick()
        composeTestRule.onNodeWithText("Given").performClick()
        composeTestRule.onNodeWithText("Description").performTextReplacement("Socks")
        composeTestRule.onNodeWithText("Create").performClick()

        assertEquals(GiftStatuses.GIVEN, confirmed?.status)
    }

    @Test
    fun `amount and currency are enforced together`() {
        setContent()

        // Amount without currency blocks save (backend pair rule).
        composeTestRule.onNodeWithText("Description").performTextReplacement("Socks")
        composeTestRule.onNodeWithText("Amount").performScrollTo().performTextReplacement("25")
        composeTestRule.onNodeWithText("Amount and currency must be set together").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Create").assertIsNotEnabled()

        composeTestRule.onNodeWithText("Currency").performScrollTo().performTextReplacement("EUR")
        composeTestRule.onNodeWithText("Create").assertIsEnabled()
    }

    @Test
    fun `a scheme-less url is normalized to https on save`() {
        var confirmed: GiftFormData? = null
        setContent(onConfirm = { confirmed = it })

        composeTestRule.onNodeWithText("Description").performTextReplacement("Socks")
        composeTestRule.onNodeWithText("URL").performScrollTo().performTextReplacement("example.com/socks")
        composeTestRule.onNodeWithText("Create").performClick()

        assertEquals("https://example.com/socks", confirmed?.url)
    }

    @Test
    fun `an invalid url blocks save`() {
        setContent()

        composeTestRule.onNodeWithText("Description").performTextReplacement("Socks")
        composeTestRule.onNodeWithText("URL").performScrollTo().performTextReplacement("javascript:alert(1)")
        composeTestRule.onNodeWithText("Enter a valid http(s) URL").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Create").assertIsNotEnabled()
    }

    @Test
    fun `edit mode pre-fills the amount from value cents`() {
        setContent(
            initial = Gift(
                id = "g1",
                entityId = "uid",
                status = "given",
                description = "Socks",
                valueCents = 2500,
                currency = "EUR",
            ),
        )
        composeTestRule.onNodeWithText("Edit gift").assertIsDisplayed()
        composeTestRule.onNodeWithText("25").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("EUR").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Given").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `centsToAmountText renders whole and fractional values`() {
        assertEquals("25", centsToAmountText(2500))
        assertEquals("2500", centsToAmountText(250000))
        assertEquals("25.5", centsToAmountText(2550))
        assertEquals("0.05", centsToAmountText(5))
    }
}

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class PreferenceDialogTest {
    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(
        initial: Preference? = null,
        onConfirm: (PreferenceFormData) -> Unit = {},
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                PreferenceDialog(
                    initial = initial,
                    onConfirm = onConfirm,
                    onDismiss = {},
                )
            }
        }
    }

    @Test
    fun `category defaults to food and can be changed with category-scoped key suggestions`() {
        var confirmed: PreferenceFormData? = null
        setContent(onConfirm = { confirmed = it })

        // media suggests show/movie/music.
        composeTestRule.onNodeWithTag("preference-category").performClick()
        composeTestRule.onNodeWithText("Media").performClick()
        composeTestRule.onNodeWithText("Show").performClick()
        composeTestRule.onNodeWithText("Value").performTextReplacement("Severance")
        composeTestRule.onNodeWithText("Create").performClick()

        assertEquals("media", confirmed?.category)
        assertEquals("show", confirmed?.key)
        assertEquals("Severance", confirmed?.value)
    }

    @Test
    fun `sensitivity is selectable and defaults to normal`() {
        var confirmed: PreferenceFormData? = null
        setContent(onConfirm = { confirmed = it })

        composeTestRule.onNodeWithTag("preference-sensitivity").performScrollTo().performClick()
        composeTestRule.onNodeWithText("Secret").performClick()
        composeTestRule.onNodeWithText("Value").performScrollTo().performTextReplacement("peanuts")
        composeTestRule.onNodeWithText("Create").performClick()

        assertEquals("secret", confirmed?.sensitivity)
    }

    @Test
    fun `edit mode pre-fills category value and sensitivity`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                PreferenceDialog(
                    initial = Preference(
                        id = "p1",
                        entityId = "uid",
                        category = "drink",
                        key = "favorite",
                        value = "matcha",
                        sensitivity = "private",
                    ),
                    onConfirm = {},
                    onDismiss = {},
                )
            }
        }
        composeTestRule.onNodeWithText("Edit preference").assertIsDisplayed()
        composeTestRule.onNodeWithText("Drink").assertIsDisplayed()
        composeTestRule.onNodeWithText("favorite").assertIsDisplayed()
        composeTestRule.onNodeWithText("matcha").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Private").performScrollTo().assertIsDisplayed()
    }
}

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class AgendaDialogTest {
    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun `edit mode pre-fills content and reference url and reports both`() {
        var confirmedContent: String? = null
        var confirmedUrl: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                AgendaDialog(
                    initial = ConversationAgenda(
                        id = "a1",
                        entityId = "uid",
                        content = "Ask about the move",
                        referenceUrl = "https://example.com/listing",
                    ),
                    onConfirm = { content, url -> confirmedContent = content; confirmedUrl = url },
                    onDismiss = {},
                )
            }
        }
        composeTestRule.onNodeWithText("Edit item").assertIsDisplayed()
        composeTestRule.onNodeWithText("Ask about the move").assertIsDisplayed()
        composeTestRule.onNodeWithText("https://example.com/listing").assertIsDisplayed()

        composeTestRule.onNodeWithText("Ask about the move").performTextReplacement("Ask about the new place")
        composeTestRule.onNodeWithText("Save").performClick()

        assertEquals("Ask about the new place", confirmedContent)
        assertEquals("https://example.com/listing", confirmedUrl)
    }

    @Test
    fun `a scheme-less reference url is normalized on save`() {
        var confirmedUrl: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                AgendaDialog(
                    initial = null,
                    onConfirm = { _, url -> confirmedUrl = url },
                    onDismiss = {},
                )
            }
        }
        composeTestRule.onNodeWithText("Content").performTextReplacement("Read the article")
        composeTestRule.onNodeWithText("Reference URL").performTextReplacement("example.com/article")
        composeTestRule.onNodeWithText("Create").performClick()

        assertEquals("https://example.com/article", confirmedUrl)
    }
}

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class MarkDiscussedDialogTest {
    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun `marking discussed without a link confirms with no activity`() {
        var confirmedCalled = false
        var confirmed: Int? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                MarkDiscussedDialog(
                    item = ConversationAgenda(id = "a1", entityId = "uid", content = "Ask about the move"),
                    activities = emptyList(),
                    confirming = false,
                    onConfirm = { confirmed = it; confirmedCalled = true },
                    onDismiss = {},
                )
            }
        }

        composeTestRule.onNodeWithText("Ask about the move").assertIsDisplayed()
        composeTestRule.onNodeWithText("Discuss").performClick()

        assertTrue(confirmedCalled)
        assertEquals(null, confirmed)
    }

    @Test
    fun `the activity selector links a chosen activity`() {
        var confirmed: Int? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                MarkDiscussedDialog(
                    item = ConversationAgenda(id = "a1", entityId = "uid", content = "Ask about the move"),
                    activities = listOf(
                        com.mycorrhizal.crm.model.network.Activity(id = 7, title = "Coffee chat"),
                        com.mycorrhizal.crm.model.network.Activity(id = 8, title = "Dinner"),
                    ),
                    confirming = false,
                    onConfirm = { confirmed = it },
                    onDismiss = {},
                )
            }
        }

        // The selector defaults to "None"; pick the Dinner activity.
        composeTestRule.onNodeWithText("None").performClick()
        composeTestRule.onNodeWithText("Dinner").performClick()
        composeTestRule.onNodeWithText("Discuss").performClick()

        assertEquals(8, confirmed)
    }
}
