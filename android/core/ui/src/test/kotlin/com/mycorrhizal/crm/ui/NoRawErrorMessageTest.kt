package com.mycorrhizal.crm.ui

import java.io.File
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Issue #2106: a feature screen words an error with `ApiError.localizedMessage()`
 * at composition time, never with the English `displayMessage` string (which is
 * the fallback and log text only). detekt's `ForbiddenMethodCall` does not see
 * property reads, hence this test.
 *
 * Tests resolve the feature modules relative to the module working directory
 * (`android/core/ui`).
 */
class NoRawErrorMessageTest {

    private val featureRoot = File("../../feature")

    @Test
    fun `no feature module reads ApiError displayMessage`() {
        assertTrue("Missing feature directory ${featureRoot.absolutePath}", featureRoot.isDirectory)
        val offenders = featureRoot.listFiles { f -> f.isDirectory }.orEmpty()
            .map { File(it, "src/main") }
            .filter { it.isDirectory }
            .flatMap { src -> src.walkTopDown().filter { it.isFile && it.extension == "kt" }.toList() }
            .flatMap { file ->
                file.readLines().withIndex()
                    .filter { (_, line) ->
                        USAGE.containsMatchIn(
                            line
                        ) && !line.trimStart().startsWith("//")
                    }
                    .map { (index, _) -> "${file.relativeTo(featureRoot)}:${index + 1}" }
            }
        assertTrue(
            "Use ApiError.localizedMessage() instead of displayMessage in: $offenders",
            offenders.isEmpty()
        )
    }

    private companion object {
        val USAGE = Regex("""\.displayMessage\b""")
    }
}
