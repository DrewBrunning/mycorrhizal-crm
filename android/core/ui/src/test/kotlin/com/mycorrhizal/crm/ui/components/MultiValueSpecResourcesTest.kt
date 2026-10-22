package com.mycorrhizal.crm.ui.components

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.ui.R
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/**
 * Issue #1724: every multi-value spec names its value field and, where the
 * format is easy to get wrong, shows a literal example and/or translated hint.
 * This pins the per-spec resources (so no spec can quietly regress to the
 * generic "Value N") and the English strings the UI renders.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
class MultiValueSpecResourcesTest {

    private val context: Context = ApplicationProvider.getApplicationContext()

    private data class Expected(
        val name: String,
        val spec: MultiValueSpec<*>,
        val label: String,
        val example: String? = null,
        val hint: String? = null,
        val item: String,
    )

    private val expected = listOf(
        Expected("EmailSpec", EmailSpec, "Email", "name@example.com", item = "email"),
        Expected(
            "PhoneSpec",
            PhoneSpec,
            "Phone number",
            "+1 555 123 4567",
            "International format recommended.",
            item = "phone number",
        ),
        Expected(
            "OnlineServiceSpec",
            OnlineServiceSpec,
            "Address / URL",
            "https://… or user@server",
            item = "online service",
        ),
        Expected("LinkSpec", LinkSpec, "URL", "https://example.com", item = "link"),
        Expected("TitleSpec", TitleSpec, "Title or role", item = "title"),
        Expected(
            "PersonalInfoSpec",
            PersonalInfoSpec,
            "Expertise / hobby / interest",
            item = "personal information",
        ),
        Expected(
            "LanguagePrefSpec",
            LanguagePrefSpec,
            "Language",
            "de, en-US, de-AT",
            "BCP-47 tag, e.g. de, en-US, de-AT.",
            item = "preferred language",
        ),
        Expected(
            "PronounsSpec",
            PronounsSpec,
            "Pronouns",
            "they/them, she/her",
            item = "pronoun",
        ),
        Expected(
            "GrammaticalGenderSpec",
            GrammaticalGenderSpec,
            "Language",
            "de, fr, es",
            "BCP-47 language tag; the dropdown is the gender.",
            item = "grammatical gender",
        ),
        Expected("AnniversarySpec", AnniversarySpec, "Date", item = "anniversary"),
    )

    @Test
    fun `each spec resolves its label example and hint`() {
        for (row in expected) {
            assertEquals("${row.name} valueLabel", row.label, context.getString(row.spec.valueLabel))
            assertEquals("${row.name} itemLabel", row.item, context.getString(row.spec.itemLabel))
            assertEquals(
                "${row.name} valueExample",
                row.example,
                row.spec.valueExample?.let { context.getString(it) },
            )
            assertEquals(
                "${row.name} valueHint",
                row.hint,
                row.spec.valueHint?.let { context.getString(it) },
            )
        }
    }

    @Test
    fun `no spec falls back to the generic Value N label`() {
        val generic = context.getString(R.string.contact_value_n, 1)
        for (row in expected) {
            val label = context.getString(row.spec.valueLabel)
            assertTrue("${row.name} label is blank", label.isNotBlank())
            assertFalse("${row.name} label still contains 'Value'", label.contains("Value"))
            assertNotEquals("${row.name} still uses the generic label", generic, label)
        }
    }

    @Test
    fun `the grammatical gender free text is labelled Language while its dropdown is the gender`() {
        // The bug in the ticket: "Value 1" hid the fact that this box holds a
        // BCP-47 language tag; the dropdown next to it is the gender.
        assertEquals(context.getString(R.string.contact_language), context.getString(GrammaticalGenderSpec.valueLabel))
        assertEquals(
            context.getString(R.string.contact_item_grammatical_gender),
            context.getString(GrammaticalGenderSpec.itemLabel),
        )
        assertEquals("de, fr, es", GrammaticalGenderSpec.valueExample?.let { context.getString(it) })
    }
}
