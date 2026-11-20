package com.mycorrhizal.crm.model.network

import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * Cross-language parity for the hand-maintained export/share section mirror
 * (frontend trap #4). Reading the Go source is deliberate: the backend is the
 * authority, and a Kotlin-only test could agree with the Kotlin copy while both
 * drift from [backend/models/field_selection.go].
 */
class ShareFieldSectionsTest {

    private val goSource = File("../../../backend/models/field_selection.go").readText()

    @Test
    fun `tokens and order match the backend FieldSections`() {
        val constants = Regex("(Section\\w+)\\s*=\\s*\"([^\"]+)\"")
            .findAll(goSource)
            .associate { it.groupValues[1] to it.groupValues[2] }
        val body = Regex("""func FieldSections\(\) \[\]string \{([\s\S]*?)\n}""")
            .find(goSource)
            ?.groupValues
            ?.get(1)
            ?: error("could not find FieldSections in backend/models/field_selection.go")
        val backendTokens = Regex("""Section\w+""")
            .findAll(body)
            .map { constants[it.value] ?: error("unknown Go section constant ${it.value}") }
            .toList()

        assertEquals(backendTokens, ShareFieldSections.ALL.map { it.token })
    }

    @Test
    fun `sensitive flags match the backend sensitiveSections set`() {
        val constants = Regex("(Section\\w+)\\s*=\\s*\"([^\"]+)\"")
            .findAll(goSource)
            .associate { it.groupValues[1] to it.groupValues[2] }
        val body = Regex("""var sensitiveSections = map\[string]bool\{([\s\S]*?)\n}""")
            .find(goSource)
            ?.groupValues
            ?.get(1)
            ?: error("could not find sensitiveSections in backend/models/field_selection.go")
        val backendSensitive = Regex("""(Section\w+):\s*true""")
            .findAll(body)
            .map {
                constants[it.groupValues[1]]
                    ?: error("unknown Go section constant ${it.groupValues[1]}")
            }
            .toSet()

        assertEquals(
            backendSensitive,
            ShareFieldSections.ALL.filter { it.sensitive }.map { it.token }.toSet()
        )
    }
}
