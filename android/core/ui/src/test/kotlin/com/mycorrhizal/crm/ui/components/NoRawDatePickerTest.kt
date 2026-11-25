package com.mycorrhizal.crm.ui.components

import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * #2947: every date entry goes through the shared [DateField] (typed entry, the user's date
 * format, week start). A raw Material `DatePicker(` anywhere else reintroduces a picker-only
 * date box with the wrong header format, so only `core/ui` may call it.
 */
class NoRawDatePickerTest {

    @Test
    fun `no raw DatePicker call outside core-ui`() {
        // Tests run with the module directory (android/core/ui) as the working directory.
        val androidRoot = File("../..").canonicalFile
        val rawCall = Regex("""\bDatePicker\(""")
        val offenders = androidRoot.walkTopDown()
            .onEnter { dir -> dir.name != "build" && dir.name != ".gradle" }
            .filter { it.isFile && it.extension == "kt" }
            .filter { "/src/main/" in it.invariantPath() && "/core/ui/" !in it.invariantPath() }
            .filter { file -> file.readLines().any { rawCall.containsMatchIn(it) } }
            .map { it.relativeTo(androidRoot).path }
            .toList()
        assertEquals("Raw DatePicker( outside core/ui: $offenders", emptyList<String>(), offenders)
    }

    private fun File.invariantPath() = path.replace('\\', '/')
}
