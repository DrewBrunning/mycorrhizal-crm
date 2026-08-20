package com.mycorrhizal.crm.feature.contacts

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.autofill.ContentType
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithContentDescription
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextReplacement
import com.mycorrhizal.crm.model.network.Email
import com.mycorrhizal.crm.model.network.Phone
import com.mycorrhizal.crm.ui.components.AutofillOutlinedTextField
import com.mycorrhizal.crm.ui.components.EmailSpec
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
        var items by mutableStateOf(listOf(Email(id = "e1", address = "a@b.c", contexts = listOf("work"), pref = 1, label = "work")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                MultiValueEditor(items = items, spec = EmailSpec, onChange = { items = it }, label = "Email")
            }
        }
        // Replace (not append) so the assertion reads the typed value, not "a@b.cx@y.z".
        composeTestRule.onNodeWithText("Value 1").performTextReplacement("x@y.z")
        assertEquals(
            listOf(Email(id = "e1", address = "x@y.z", contexts = listOf("work"), pref = 1, label = "work")),
            items,
        )
    }

    @Test
    fun `clicking a star enforces pref exclusivity through the ui`() {
        var items by mutableStateOf(
            listOf(
                Phone(id = "p1", number = "+1", pref = 1),
                Phone(id = "p2", number = "+2"),
                Phone(id = "p3", number = "+3"),
            ),
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                MultiValueEditor(items = items, spec = PhoneSpec, onChange = { items = it }, label = "Phone")
            }
        }
        // Rows 2 and 3 are not preferred -> their stars say "Set as preferred"; the first
        // one belongs to row 2 (index 1 in the list). Toggling it must clear row 1.
        composeTestRule.onAllNodesWithContentDescription("Set as preferred")[0].performClick()
        assertNull(items[0].pref)
        assertEquals(1, items[1].pref)
        assertNull(items[2].pref)
    }

    @Test
    fun `selecting a type from the dropdown updates the context`() {
        var items by mutableStateOf(listOf(Email(id = "e1", address = "a@b.c", contexts = listOf("work"))))
        composeTestRule.setContent {
            MycorrhizalTheme {
                MultiValueEditor(items = items, spec = EmailSpec, onChange = { items = it }, label = "Email")
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
        var items by mutableStateOf(listOf(Phone(id = "p1", number = "+1", contexts = listOf("work"))))
        composeTestRule.setContent {
            MycorrhizalTheme {
                MultiValueEditor(items = items, spec = PhoneSpec, onChange = { items = it }, label = "Phone")
            }
        }
        composeTestRule.onNodeWithContentDescription("Add").performClick()
        assertEquals(2, items.size)
        // Loaded row untouched, new row has the cell default and no id.
        assertEquals(listOf("work"), items[0].contexts)
        assertEquals(listOf("cell"), items[1].contexts)
        assertNull(items[1].id)

        // Typing into the new row keeps its cell default (T81: default only for new rows).
        composeTestRule.onNodeWithText("Value 2").performTextReplacement("+2")
        assertEquals(listOf("cell"), items[1].contexts)
        assertEquals("+2", items[1].number)
    }

    @Test
    fun `the service name field edits the online service name`() {
        var items by mutableStateOf(listOf(com.mycorrhizal.crm.model.network.OnlineService(id = "s1", service = "Signal", uri = "@dana")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                MultiValueEditor(items = items, spec = com.mycorrhizal.crm.ui.components.OnlineServiceSpec, onChange = { items = it }, label = "Messaging")
            }
        }
        composeTestRule.onNodeWithText("Signal").performTextReplacement("Telegram")
        assertEquals("Telegram", items.firstOrNull()?.service)
        // The handle and id ride along untouched.
        assertEquals("@dana", items.firstOrNull()?.uri)
        assertEquals("s1", items.firstOrNull()?.id)
    }

    @Test
    fun `renders the editor caption and rows`() {
        var items by mutableStateOf(listOf(Email(address = "a@b.c")))
        composeTestRule.setContent {
            MycorrhizalTheme {
                MultiValueEditor(items = items, spec = EmailSpec, onChange = { items = it }, label = "Email")
            }
        }
        composeTestRule.onNodeWithText("Email").assertIsDisplayed()
        composeTestRule.onNodeWithText("Value 1").assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("Add").assertIsDisplayed()
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
                    contentType = ContentType.PersonFirstName,
                )
            }
        }

        composeTestRule.onNodeWithText("Given name")
            .assert(SemanticsMatcher.expectValue(SemanticsProperties.ContentType, ContentType.PersonFirstName))
    }

    @Test
    fun `autofill field without a type carries no content type semantics`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                AutofillOutlinedTextField(
                    value = "",
                    onValueChange = {},
                    label = "No hint",
                    contentType = null,
                )
            }
        }

        composeTestRule.onNodeWithText("No hint")
            .assert(SemanticsMatcher.keyNotDefined(SemanticsProperties.ContentType))
    }
}
