package com.mycorrhizal.crm.ui

import android.content.Context
import android.content.res.Configuration
import androidx.test.core.app.ApplicationProvider
import java.util.Locale
import org.junit.Assert.assertEquals
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issue #1770: Android has no hand-rolled "(s)" pluralization. Counts select a
 * real `<plurals>` resource so the singular and plural forms are grammatical in
 * every shipped locale (which don't all collapse to English's one/other rule).
 *
 * Rendering the resource — rather than only asserting the XML shape — is what
 * catches a translation that keeps the English plural, a missing quantity, or
 * a `0`/`1` selection regression.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class PluralResourcesTest {

    private val base: Context = ApplicationProvider.getApplicationContext()

    private fun contextFor(tag: String): Context {
        val config = Configuration(base.resources.configuration).apply {
            setLocale(Locale.forLanguageTag(tag))
        }
        return base.createConfigurationContext(config)
    }

    private fun plural(context: Context, id: Int, quantity: Int): String =
        context.resources.getQuantityString(id, quantity, quantity)

    @Test
    fun `contact count renders singular and plural in all five locales`() {
        val expected = mapOf(
            "en" to ("1 contact" to "2 contacts"),
            "de" to ("1 Kontakt" to "2 Kontakte"),
            "es" to ("1 contacto" to "2 contactos"),
            "fr" to ("1 contact" to "2 contacts"),
            "it" to ("1 contatto" to "2 contatti"),
        )
        for ((tag, forms) in expected) {
            val ctx = contextFor(tag)
            assertEquals("$tag singular", forms.first, plural(ctx, R.plurals.triage_contacts_count, 1))
            assertEquals("$tag plural", forms.second, plural(ctx, R.plurals.triage_contacts_count, 2))
        }
    }

    @Test
    fun `days-until renders singular and a large plural in all five locales`() {
        val expected = mapOf(
            "en" to ("in 1 day" to "in 333 days"),
            "de" to ("in 1 Tag" to "in 333 Tagen"),
            "es" to ("en 1 día" to "en 333 días"),
            "fr" to ("dans 1 jour" to "dans 333 jours"),
            "it" to ("tra 1 giorno" to "tra 333 giorni"),
        )
        for ((tag, forms) in expected) {
            val ctx = contextFor(tag)
            assertEquals("$tag singular", forms.first, plural(ctx, R.plurals.prep_upcoming_dates_in_days, 1))
            assertEquals("$tag plural", forms.second, plural(ctx, R.plurals.prep_upcoming_dates_in_days, 333))
        }
    }

    @Test
    fun `duplicate pair count renders singular and plural in all five locales`() {
        val expected = mapOf(
            "en" to ("1 potential duplicate pair" to "2 potential duplicate pairs"),
            "de" to ("1 potenzielles Duplikatpaar" to "2 potenzielle Duplikatpaare"),
            "es" to ("1 posible pareja de duplicados" to "2 posibles parejas de duplicados"),
            "fr" to ("1 paire de doublons potentielle" to "2 paires de doublons potentielles"),
            "it" to ("1 possibile coppia di duplicati" to "2 possibili coppie di duplicati"),
        )
        for ((tag, forms) in expected) {
            val ctx = contextFor(tag)
            assertEquals("$tag singular", forms.first, plural(ctx, R.plurals.duplicates_count, 1))
            assertEquals("$tag plural", forms.second, plural(ctx, R.plurals.duplicates_count, 2))
        }
    }

    @Test
    fun `zero uses the plural form, not the singular`() {
        // French CLDR treats 0 as "one"; every other shipped locale uses "other".
        // Pin each so a future quantity edit can't silently change the meaning.
        assertPlural(contextFor("en"), R.plurals.triage_contacts_count, 0, "0 contacts")
        assertPlural(contextFor("de"), R.plurals.triage_contacts_count, 0, "0 Kontakte")
        assertPlural(contextFor("es"), R.plurals.triage_contacts_count, 0, "0 contactos")
        assertPlural(contextFor("fr"), R.plurals.triage_contacts_count, 0, "0 contact")
        assertPlural(contextFor("it"), R.plurals.triage_contacts_count, 0, "0 contatti")
    }

    private fun assertPlural(context: Context, id: Int, quantity: Int, expected: String) {
        assertEquals(expected, plural(context, id, quantity))
    }
}
