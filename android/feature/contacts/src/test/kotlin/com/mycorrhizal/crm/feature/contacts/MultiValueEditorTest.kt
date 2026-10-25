package com.mycorrhizal.crm.feature.contacts

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.autofill.ContentType
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotSelected
import androidx.compose.ui.test.assertIsSelected
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextReplacement
import com.mycorrhizal.crm.model.network.Email
import com.mycorrhizal.crm.model.network.GrammaticalGender
import com.mycorrhizal.crm.model.network.Phone
import com.mycorrhizal.crm.ui.components.AutofillOutlinedTextField
import com.mycorrhizal.crm.ui.components.EmailSpec
import com.mycorrhizal.crm.ui.components.GrammaticalGenderSpec
import com.mycorrhizal.crm.ui.components.MultiValueEditor
import com.mycorrhizal.crm.ui.components.PhoneSpec
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class MultiValueEditorTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun `typing a value copies the entry keeping id contexts and pref`() {
        var items by mutableStateOf(
            listOf(
                Email(
                    id = "e1",
                    address = "a@b.c",
                    contexts = listOf("work"),
                    pref = 1,
                    label = "work"
                )
            )
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                MultiValueEditor(
                    items = items,
                    spec = EmailSpec,
                    onChange = { items = it },
                    label = "Email"
                )
            }
        }
        // Issue #1724: the field is labelled with its meaning, not "Value 1".
        composeTestRule.onNodeWithContentDescription("Email").performTextReplacement("x@y.z")
        composeTestRule.onAllNodesWithText("Value 1").assertCountEquals(0)
        assertEquals(
            listOf(
                Email(
                    id = "e1",
                    address = "x@y.z",
                    contexts = listOf("work"),
                    pref = 1,
                    label = "work"
                )
            ),
            items
        )
    }

    @Test
    fun `the preferred chip enforces pref exclusivity and announces its state`() {
        var items by mutableStateOf(
            listOf(
                Phone(id = "p1", number = "+1", pref = 1),
                Phone(id = "p2", number = "+2"),
                Phone(id = "p3", number = "+3")
            )
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                MultiValueEditor(
                    items = items,
                    spec = PhoneSpec,
                    onChange = { items = it },
                    label = "Phone"
                )
            }
        }
        // A selected FilterChip is distinct from the favourite-contact star and
        // exposes selected/not-selected to TalkBack. Toggling row 2 must clear row 1.
        composeTestRule.onAllNodesWithText("Preferred")[0].assertIsSelected()
        composeTestRule.onAllNodesWithText("Preferred")[1].assertIsNotSelected()

        composeTestRule.onAllNodesWithText("Preferred")[1].performClick()
        assertNull(items[0].pref)
        assertEquals(1, items[1].pref)
        assertNull(items[2].pref)
        composeTestRule.onAllNodesWithText("Preferred")[0].assertIsNotSelected()
        composeTestRule.onAllNodesWithText("Preferred")[1].assertIsSelected()
    }

    @Test
    fun `selecting a type from the dropdown updates the context`() {
        var items by mutableStateOf(
            listOf(Email(id = "e1", address = "a@b.c", contexts = listOf("work")))
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                MultiValueEditor(
                    items = items,
                    spec = EmailSpec,
                    onChange = { items = it },
                    label = "Email"
                )
            }
        }
        // The read-only field shows the localized current type ("Work"); clicking it opens
        // the dropdown, where a different option updates contexts[0].
        composeTestRule.onNodeWithText("Work").performClick()
        composeTestRule.onNodeWithText("Home").performClick()
        assertEquals(listOf(Email(id = "e1", address = "a@b.c", contexts = listOf("home"))), items)
    }

    @Test
    fun `adding a new phone mints it via blank so it defaults to cell`() {
        var items by mutableStateOf(
            listOf(Phone(id = "p1", number = "+1", contexts = listOf("work")))
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                MultiValueEditor(
                    items = items,
                    spec = PhoneSpec,
                    onChange = { items = it },
                    label = "Phone"
                )
            }
        }
        // Issue #1724: a visible, field-named add action (not a bare "Add" icon).
        composeTestRule.onNodeWithText("Add phone number").performClick()
        assertEquals(2, items.size)
        // Loaded row untouched, new row has the cell default and no id.
        assertEquals(listOf("work"), items[0].contexts)
        assertEquals(listOf("cell"), items[1].contexts)
        assertNull(items[1].id)

        // Typing into the new row keeps its cell default (T81: default only for new rows).
        // The row number is only in the TalkBack description; the visible label is "Phone number".
        composeTestRule.onNodeWithContentDescription("Phone number 2").performTextReplacement("+2")
        assertEquals(listOf("cell"), items[1].contexts)
        assertEquals("+2", items[1].number)
    }

    @Test
    fun `removing a row is announced with its field and row number`() {
        var items by mutableStateOf(
            listOf(
                Phone(id = "p1", number = "+1"),
                Phone(id = "p2", number = "+2")
            )
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                MultiValueEditor(
                    items = items,
                    spec = PhoneSpec,
                    onChange = { items = it },
                    label = "Phone"
                )
            }
        }
        composeTestRule.onNodeWithContentDescription("Remove phone number 2").performClick()
        assertEquals(listOf("p1"), items.map { it.id })
    }

    @Test
    fun `a label-only email pre-selects a known type in the dropdown`() {
        var items by mutableStateOf(listOf(Email(id = "e1", address = "a@b.c", label = "home")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                MultiValueEditor(
                    items = items,
                    spec = EmailSpec,
                    onChange = { items = it },
                    label = "Email"
                )
            }
        }
        // Legacy flat-Type data stores the type in `label`; the dropdown now
        // agrees with the detail screen instead of showing blank.
        composeTestRule.onNodeWithText("Home").assertIsDisplayed().performClick()
        composeTestRule.onNodeWithText("Work").performClick()
        assertEquals(listOf("work"), items.single().contexts)
        // The X-ABLabel fallback is preserve-only, not rewritten by a type edit.
        assertEquals("home", items.single().label)
    }

    @Test
    fun `the service name field edits the online service name`() {
        var items by mutableStateOf(
            listOf(
                com.mycorrhizal.crm.model.network.OnlineService(
                    id = "s1",
                    service = "Signal",
                    uri = "@dana"
                )
            )
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                MultiValueEditor(
                    items = items,
                    spec = com.mycorrhizal.crm.ui.components.OnlineServiceSpec,
                    onChange = { items = it },
                    label = "Messaging"
                )
            }
        }
        composeTestRule.onNodeWithText("Signal").performTextReplacement("Telegram")
        assertEquals("Telegram", items.firstOrNull()?.service)
        // The handle and id ride along untouched.
        assertEquals("@dana", items.firstOrNull()?.uri)
        assertEquals("s1", items.firstOrNull()?.id)
    }

    @Test
    fun `renders the field label, format example and named add action`() {
        var items by mutableStateOf(listOf(Email(address = "")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                MultiValueEditor(
                    items = items,
                    spec = EmailSpec,
                    onChange = { items = it },
                    label = "Email"
                )
            }
        }
        composeTestRule.onNodeWithContentDescription("Email").assertIsDisplayed().performClick()
        // A TextField placeholder appears while the field is focused and empty.
        composeTestRule.onNodeWithText("name@example.com").assertExists()
        composeTestRule.onAllNodesWithText("Value 1").assertCountEquals(0)
        composeTestRule.onNodeWithText("Add email").assertIsDisplayed()
    }

    @Test
    fun `the grammatical gender row labels its free text as Language with a BCP-47 example`() {
        var items by mutableStateOf(listOf(GrammaticalGender(value = "feminine", language = "")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                MultiValueEditor(
                    items = items,
                    spec = GrammaticalGenderSpec,
                    onChange = { items = it },
                    label = "Grammatical gender"
                )
            }
        }
        // The free-text box is the language tag, not the gender (the dropdown is).
        composeTestRule.onNodeWithText("Language").assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("Language").performClick()
        composeTestRule.onNodeWithText("de, fr, es").assertExists()
        composeTestRule.onNodeWithText(
            "BCP-47 language tag; the dropdown is the gender."
        ).assertExists()
        // The dropdown carries the gender value.
        composeTestRule.onNodeWithText("Feminine").assertIsDisplayed()
        composeTestRule.onAllNodesWithText("Value 1").assertCountEquals(0)
    }

    // --- T115: Android autofill hints --------------------------------------
    // The contact form's fields (via AutofillOutlinedTextField) advertise a
    // semantics ContentType so the platform Autofill framework can offer a
    // fill. This is Compose's stable (since UI 1.8.0) semantics-based
    // Autofill API -- it replaced the old experimental AutofillNode/
    // LocalAutofillTree API these tests used to drive directly (registering
    // a node, then manually invoking its onFill to simulate a framework
    // fill). The new API has no public equivalent hook: delivery happens
    // through the platform's real AutofillManager/View#autofill() plumbing,
    // which isn't something a Robolectric unit test can trigger. What's left
    // to pin here is the wiring this component controls: the right
    // ContentType lands in the field's semantics, or none does when the spec
    // has no standard hint. `onValueChange` itself is already covered by the
    // plain typing tests above (e.g. "typing a value copies the entry...").

    @Test
    fun `autofill field advertises its content type`() {
        var text by mutableStateOf("")
        composeTestRule.setContent {
            MycorrhizalTheme {
                AutofillOutlinedTextField(
                    value = text,
                    onValueChange = { text = it },
                    label = "Given name",
                    contentType = ContentType.PersonFirstName
                )
            }
        }

        composeTestRule.onNodeWithText("Given name")
            .assert(
                SemanticsMatcher.expectValue(
                    SemanticsProperties.ContentType,
                    ContentType.PersonFirstName
                )
            )
    }

    @Test
    fun `autofill field without a type carries no content type semantics`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                AutofillOutlinedTextField(
                    value = "",
                    onValueChange = {},
                    label = "No hint",
                    contentType = null
                )
            }
        }

        composeTestRule.onNodeWithText("No hint")
            .assert(SemanticsMatcher.keyNotDefined(SemanticsProperties.ContentType))
    }
}
