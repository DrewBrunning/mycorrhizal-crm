package com.mycorrhizal.crm.ui

import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Issue #1760: headings, labels and buttons are sentence case (Material 3, and
 * what the web app does) — "Overdue relationships", never "Overdue
 * Relationships". German is skipped because every German noun is capitalised
 * by the language, not by style.
 *
 * The check is a heuristic over short, non-sentence strings: a word after the
 * first that is a Capitalised Word is Title Case unless it is a proper noun
 * ([properNouns]) or the string is one of the reviewed [exceptions]. A word
 * that follows a sentence terminator (`?`, `!`, `:`) starts a new sentence and
 * is allowed to be capitalised.
 */
class SentenceCaseStringsTest {

    private val properNouns = setOf(
        "Immich", "Paperless", "Seafile", "Nextcloud", "ownCloud", "GeoPulse",
        "Gotify", "Mycorrhizal", "Android"
    )

    /** key -> why it legitimately carries a capitalised non-initial word. */
    private val exceptions = mapOf(
        // The name of the Android system app, not a heading word.
        "contact_open_in_contacts" to "names the Contacts app"
    )

    private val word = Regex("""^[A-ZÀ-Ý][a-zà-ÿ]+[,:?!]?$""")

    private fun valuesFor(dir: String): Map<String, String> {
        val text = File("src/main/res/$dir/strings.xml").readText()
        return Regex("""<string name="([^"]+)"[^>]*>([^<]*)</string>""")
            .findAll(text)
            .associate { it.groupValues[1] to it.groupValues[2] }
    }

    /** The Title Case offenders in one locale, as `key | value`. */
    private fun offenders(dir: String): List<String> =
        valuesFor(dir).filter { (key, value) -> key !in exceptions && isTitleCase(value) }
            .map { (key, value) -> "$key | $value" }

    internal fun isTitleCase(value: String): Boolean {
        val words = value.split(' ').filter { it.isNotBlank() }
        if (words.size < 2 || words.size > 7 || value.endsWith(".")) return false
        var sentenceStart = true
        for (w in words) {
            val bare = w.trimEnd(',', ':', '?', '!')
            val capitalised = word.matches(w)
            if (!sentenceStart && capitalised && bare !in properNouns) return true
            if (w.isNotEmpty() && w.first().isLetter()) sentenceStart = false
            if (w.last() in "?!:") sentenceStart = true
        }
        return false
    }

    @Test
    fun `English strings carry no Title Case headings`() {
        assertEquals(
            "Title Case strings in values/strings.xml",
            emptyList<String>(),
            offenders("values")
        )
    }

    @Test
    fun `Spanish French and Italian strings carry no Title Case headings`() {
        for (dir in listOf("values-es", "values-fr", "values-it")) {
            assertEquals(
                "Title Case strings in $dir/strings.xml",
                emptyList<String>(),
                offenders(dir)
            )
        }
    }

    @Test
    fun `heuristic flags Title Case and spares sentence case`() {
        assertTrue(isTitleCase("Overdue Relationships"))
        assertTrue(isTitleCase("Reasons to Reach Out"))
        assertTrue(isTitleCase("Email – Type"))
        assertTrue(!isTitleCase("Overdue relationships"))
        assertTrue(!isTitleCase("Link a Paperless document"))
        assertTrue(!isTitleCase("No account? Create one"))
        assertTrue(!isTitleCase("Single"))
    }
}
