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


    /** Plural resource names in one values dir — the `<plurals>` half of the key set. */
    private fun pluralKeysFor(valuesDir: String): Set<String> {
        val text = contentFor(valuesDir)
        return Regex("""<plurals name="([^"]+)""")
            .findAll(text)
            .map { it.groupValues[1] }
            .toSet()
    }

    /** quantity name -> value for one plural resource, in one values dir. */
    private fun pluralQuantitiesFor(valuesDir: String, name: String): Map<String, String> {
        val text = contentFor(valuesDir)
        val block = Regex("""<plurals name="${Regex.escape(name)}">([\s\S]*?)</plurals>""")
            .find(text)
            ?.groupValues
            ?.get(1)
            ?: error("Missing plural '$name' in $valuesDir")
        return Regex("""<item quantity="([^"]+)">([\s\S]*?)</item>""")
            .findAll(block)
            .associate { it.groupValues[1] to it.groupValues[2] }
    }

    @Test
    fun `all locale files carry the exact English plural key set`() {
        val english = pluralKeysFor("values")
        for (locale in locales) {
            assertEquals(
                "Locale $locale must have the same plural keys as English",
                english,
                pluralKeysFor(locale),
            )
        }
    }

    @Test
    fun `every plural resource defines one and other in every locale`() {
        for (locale in locales) {
            for (name in pluralKeysFor(locale)) {
                val quantities = pluralQuantitiesFor(locale, name)
                assertTrue(
                    "Locale $locale plural '$name' is missing the 'one' quantity",
                    quantities.containsKey("one"),
                )
                assertTrue(
                    "Locale $locale plural '$name' is missing the 'other' quantity",
                    quantities.containsKey("other"),
                )
            }
        }
    }

    @Test
    fun `no locale file uses hand-rolled (s) pluralization`() {
        for (locale in locales) {
            val matches = Regex("""\(s\)""").findAll(contentFor(locale)).toList()
            assertTrue(
                "Locale $locale still contains hand-rolled '(s)' pluralization at " +
                    matches.joinToString { it.range.toString() },
                matches.isEmpty(),
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

    /** key name -> its raw (unescaped) resource value, for one values dir. */
    private fun valuesFor(valuesDir: String): Map<String, String> {
        val text = contentFor(valuesDir)
        return Regex("""<string name="([^"]+)">([\s\S]*?)</string>""")
            .findAll(text)
            .associate { it.groupValues[1] to it.groupValues[2] }
    }

    /** The key's namespace/prefix group: everything before the first underscore. */
    private fun namespaceOf(key: String): String = key.substringBefore('_')

    /**
     * Mirrors the frontend's `locales.test.ts` per-namespace check: a whole
     * namespace (all keys sharing the same `foo_` prefix, e.g. every
     * `settings_*` key) must not be entirely byte-identical to English in a
     * translated locale — that pattern is what an untranslated, copy-pasted
     * English block looks like (this exact bug shipped once on the frontend
     * as a 25-key untranslated block in all four non-English locales).
     *
     * This is deliberately a whole-group check, not a per-key one: individual
     * keys are allowed to legitimately match English (proper nouns, cognates,
     * format-example hints like ISO date strings) as long as at least one
     * other key in the same namespace differs. Namespaces with fewer than two
     * keys are skipped entirely for the same reason — a single-key namespace
     * matching is not evidence of anything.
     */
    @Test
    fun `no key-prefix namespace is left byte-identical to English within a locale`() {
        val english = valuesFor("values")
        val englishKeysByNamespace = english.keys.groupBy(::namespaceOf)

        for (locale in locales.drop(1)) {
            val localeValues = valuesFor(locale)
            for ((namespace, keys) in englishKeysByNamespace) {
                if (keys.size < 2) continue
                val sortedKeys = keys.sorted()
                val englishGroup = sortedKeys.map { english.getValue(it) }
                val localeGroup = sortedKeys.map {
                    localeValues[it] ?: error("Locale $locale is missing key '$it'")
                }
                assertTrue(
                    "Locale $locale: every key under the '$namespace' prefix " +
                        "($sortedKeys) is byte-identical to English — this namespace " +
                        "looks untranslated",
                    englishGroup != localeGroup,
                )
            }
        }
    }
}
