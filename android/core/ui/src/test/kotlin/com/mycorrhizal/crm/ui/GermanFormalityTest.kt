package com.mycorrhizal.crm.ui

import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Issue #2823 / #2686: the German locale addresses the user with the formal
 * "Sie" (maintainer decision, 2026-11-17). Mirrors the web guard
 * `frontend/src/i18n/germanFormality.test.ts`: fails on the informal pronoun
 * set and on informal imperatives, so a new "du" string cannot slip back in.
 * Whole words only; Sie/Ihr/Ihnen are the formal forms and are not listed.
 */
class GermanFormalityTest {

    private val informalPronouns = Regex(
        """(?<![\p{L}])(?:du|dich|dir|dein|deine|deinen|deinem|deiner|deines|euch|euer|eure|euren|eurem|eurer|eures)(?![\p{L}])""",
        RegexOption.IGNORE_CASE
    )

    private val informalImperatives = Regex(
        """(?<![\p{L}])(?:wähle|prüfe|erstelle|verwende|exportiere|füge|sende|setze|lege|verbinde|aktualisiere|verknüpfe|importiere|ändere|teile|erfasse|lasse|beschreibe|führe|aktiviere|bestätige|benenne|gib|bleibe|bleib|mach|hake|tippe|behalte|erlaube|schließe|öffne|lösche|entferne|nutze|speichere|erhöhe|lade|verfeinere|notiere|verwalte|widerrufe|filtere|entsperre|markiere|protokolliere|hänge|korrigiere|erteile|kopiere|klicke|vergib|vergiss|scanne|deaktiviere|gewähre|synchronisiere|überprüfe|bearbeite|wechsle|trenne|nimm|wiederhole|schalte|starte|stelle sicher|versuche es|suche dann|lies|sieh|beginne|achte|beachte|warte|drücke|wische)(?![\p{L}])""",
        RegexOption.IGNORE_CASE
    )

    /**
     * Genuine false positives only: string name -> reason. Every entry must
     * justify why the informal-looking word is not addressing the user.
     */
    private val allowlist: Map<String, String> = emptyMap()

    /** name (plural items as `name#quantity`) -> raw value, for values-de. */
    private fun germanStrings(): Map<String, String> {
        val file = File("src/main/res/values-de/strings.xml")
        assertTrue("Missing values-de/strings.xml", file.exists())
        val text = file.readText()
        val out = linkedMapOf<String, String>()
        Regex("""<string name="([^"]+)"[^>]*>([\s\S]*?)</string>""").findAll(text).forEach {
            out[it.groupValues[1]] = it.groupValues[2]
        }
        Regex("""<plurals name="([^"]+)"[^>]*>([\s\S]*?)</plurals>""").findAll(text).forEach { p ->
            Regex("""<item quantity="([^"]+)">([\s\S]*?)</item>""").findAll(p.groupValues[2]).forEach {
                out["${p.groupValues[1]}#${it.groupValues[1]}"] = it.groupValues[2]
            }
        }
        return out
    }

    private fun offenders(pattern: Regex): List<String> =
        germanStrings().filter { (name, value) -> name !in allowlist && pattern.containsMatchIn(value) }.keys.toList()

    @Test
    fun `German strings address the user formally with no du or dein pronouns`() {
        assertEquals(
            "values-de strings with informal pronouns (use Sie/Ihnen/Ihr): ${offenders(informalPronouns)}",
            emptyList<String>(),
            offenders(informalPronouns)
        )
    }

    @Test
    fun `German strings use Sie imperatives not informal ones`() {
        assertEquals(
            "values-de strings with informal imperatives (use e.g. 'Geben Sie ein'): ${offenders(informalImperatives)}",
            emptyList<String>(),
            offenders(informalImperatives)
        )
    }

    @Test
    fun `the formal form is actually used`() {
        val formal = Regex("""(?<![\p{L}])(?:Sie|Ihr\p{L}*)(?![\p{L}])""")
        assertTrue(germanStrings().values.count { formal.containsMatchIn(it) } > 50)
    }

    @Test
    fun `every allowlist entry names a real German string`() {
        assertTrue(germanStrings().keys.containsAll(allowlist.keys))
    }
}
