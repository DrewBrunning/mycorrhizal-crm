package com.mycorrhizal.crm.ui

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Mirrors the frontend's `locales.test.ts`: every locale file must carry the
 * exact same key set as English (no key missing, none extra), and a locale
 * must not be left byte-identical to English in a way that suggests the whole
 * file was never translated.
 *
 * The resource files live under core/ui/src/main/res in the values folders;
 * tests resolve them relative to the module working directory.
 */
class LocalesConsistencyTest {

    private val locales = listOf("values", "values-de", "values-es", "values-fr", "values-it")

    private fun keysFor(valuesDir: String): Set<String> {
        val file = File("src/main/res/$valuesDir/strings.xml")
        assertTrue("Missing resource file $valuesDir/strings.xml", file.exists())
        val text = file.readText()
        return Regex("""<string name="([^"]+)"""")
            .findAll(text)
            .map { it.groupValues[1] }
            .toSet()
    }

    private fun contentFor(valuesDir: String): String =
        File("src/main/res/$valuesDir/strings.xml").readText()

    @Test
    fun `all locale files carry the exact English key set`() {
        val english = keysFor("values")
        for (locale in locales) {
            assertEquals(
                "Locale $locale must have the same keys as English",
                english,
                keysFor(locale),
            )
        }
    }

    @Test
    fun `non-English locales are not byte-identical to English`() {
        val english = contentFor("values")
        for (locale in locales.drop(1)) {
            assertTrue(
                "Locale $locale must not be byte-identical to English",
                contentFor(locale) != english,
            )
        }
    }
}
