package com.mycorrhizal.crm.di

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test
import java.io.File

// Issue #1374: a Hilt @EntryPoint / @InstallIn declared in a test APK is never aggregated into the
// component generated for MycorrhizalApplication (that happens when the APP compiles), so
// EntryPointAccessors.fromApplication(...) on it throws ClassCastException. LocalOnlyModeE2eTest and
// LocalBundleRoundTripE2eTest shipped that way and had never passed, only skipped on the x86_64 CI
// emulator. The instrumentation runner is plain AndroidJUnitRunner (no HiltTestApplication), so any
// such declaration under a src/androidTest tree is a bug. Fix pattern: declare it in the app's
// sources, as di/LocalServerHostEntryPoint.kt does.
//
// The androidTest roots are handed in by app/build.gradle.kts (-DandroidTestHilt.roots).
class AndroidTestHiltDeclarationsTest {

    @Test
    fun `no androidTest source declares a Hilt entry point or module install`() {
        val roots = System.getProperty("androidTestHilt.roots")
        assertTrue(
            "androidTestHilt.roots system property missing: run via Gradle (app/build.gradle.kts wires it)",
            !roots.isNullOrBlank(),
        )
        val files = roots.split(';').filter { it.isNotBlank() }.map(::File).filter { it.isDirectory }
            .flatMap { root ->
                root.walkTopDown().filter { it.isFile && (it.extension == "kt" || it.extension == "java") }.toList()
            }
            .associate { it.path to it.readText() }
        assertTrue("expected to scan at least the app's androidTest sources, found none", files.isNotEmpty())
        val problems = violations(files)
        if (problems.isNotEmpty()) {
            fail(
                "Hilt @EntryPoint/@InstallIn declared under src/androidTest (issue #1374). Hilt never " +
                    "aggregates these into MycorrhizalApplication's component (the runner is plain " +
                    "AndroidJUnitRunner), so EntryPointAccessors.fromApplication fails with a " +
                    "ClassCastException at runtime. Move the declaration into the app's main sources, like " +
                    "app/src/main/kotlin/com/mycorrhizal/crm/di/LocalServerHostEntryPoint.kt:\n" +
                    problems.joinToString("\n"),
            )
        }
    }

    // --- the checker itself (a gate that has never failed proves nothing) -------------------

    @Test
    fun `flags entry point and install in annotations`() {
        val src = """
            @EntryPoint
            @InstallIn(SingletonComponent::class)
            interface X
        """.trimIndent()
        assertEquals(2, violations(mapOf("a/T.kt" to src)).size)
        assertEquals(1, violations(mapOf("a/T.kt" to "@dagger.hilt.EntryPoint interface X")).size)
    }

    @Test
    fun `ignores accessor usage comments and strings`() {
        val src = """
            import dagger.hilt.EntryPoint
            import dagger.hilt.android.EntryPointAccessors
            // @EntryPoint would be wrong here
            * @InstallIn in kdoc
            val h = EntryPointAccessors.fromApplication(c, LocalServerHostEntryPoint::class.java)
        """.trimIndent()
        assertEquals(emptyList<String>(), violations(mapOf("a/T.kt" to src)))
    }

    private fun violations(files: Map<String, String>): List<String> {
        val pattern = Regex("""^\s*@(?:dagger\.hilt\.)?(EntryPoint|InstallIn)\b""")
        return files.flatMap { (path, text) ->
            text.lines().mapIndexedNotNull { i, line ->
                pattern.find(line)?.let { "$path:${i + 1}: @${it.groupValues[1]}" }
            }
        }
    }
}
