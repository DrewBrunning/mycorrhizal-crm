package com.mycorrhizal.crm.ui

import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Issue #2920, Android half of the web's `local/no-raw-error-message` rule.
 *
 * A Throwable's `message` is English (a JVM "Trust anchor for certification
 * path not found", a coordinator's own sentence) and a string literal
 * fallback (`?: "unknown"`) is English too. Either one shown in the UI reaches
 * every language untranslated, and no pseudo-locale screen test sees it unless
 * a fixture happens to drive that exact failure. This scans the UI modules'
 * sources instead, so the next such line fails here whether or not a test
 * reaches it:
 *
 * - [THROWABLE_MESSAGE]: `e.message` / `failure?.localizedMessage` read off an
 *   exception-named value. Logging is exempt (the line or the two before it
 *   open a `Log.`/Timber call). Map the error with `toApiError()` and render
 *   `ApiError.localizedMessage()`, or give the failure a typed reason and a
 *   string resource (see `AttachFailure.messageRes()`).
 * - [LITERAL_FALLBACK]: a `?: "text"` fallback feeding UI text (a
 *   `stringResource` format argument, `text =`, or `Text(`), on the same line
 *   or as a wrapped continuation of it.
 *
 * A finding that is genuinely not UI text goes in [ALLOWED] with its reason.
 */
class RawErrorTextGuardTest {

    @Test
    fun `no UI source shows a raw exception message or an English literal fallback`() {
        val findings = uiSourceRoots().flatMap { root ->
            root.walkTopDown()
                .filter { it.isFile && it.extension == "kt" }
                .sortedBy { it.path }
                .flatMap { scanFile(it, androidRoot()) }
                .toList()
        }
        assertEquals(
            "Show translated text instead (see this test's KDoc):\n" + findings.joinToString("\n"),
            emptyList<String>(),
            findings
        )
    }

    @Test
    fun `the scan covers every feature module`() {
        val roots = uiSourceRoots()
        assertTrue("expected the feature modules to be scanned: $roots", roots.size > 10)
    }

    @Test
    fun `each pattern is caught and logging is not`() {
        val probe = File.createTempFile("RawErrorProbe", ".kt").apply {
            writeText(
                listOf(
                    "val a = failure.message",
                    "val b = e?.localizedMessage ?: x",
                    "val c = stringResource(R.string.x, error ?: \"unknown\")",
                    "Text(text = delivery.statusCode?.toString() ?: \"err\")",
                    "Log.w(",
                    "    \"Tag\",",
                    "    \"failed: \${e.message}\"",
                    ")",
                    "val ok = error.localizedMessage()",
                    "val token = meta.mimeType ?: \"image/jpeg\"",
                    "Text(label)",
                    "StateChips(value = draft[key] ?: \"inherit\")",
                    "Text(",
                    "    text = result.message",
                    "        ?: \"unknown\"",
                    ")",
                    "stringResource(",
                    "    R.string.x,",
                    "    error ?: result.message",
                    "        ?: \"unknown\"",
                    ")"
                ).joinToString("\n")
            )
            deleteOnExit()
        }
        val lines = scanFile(probe, checkNotNull(probe.parentFile))
            .map { it.split(": ")[1].toInt() }
        assertEquals(listOf(1, 2, 3, 4, 15, 20), lines)
    }

    private fun androidRoot(): File = File("../..").canonicalFile

    private fun uiSourceRoots(): List<File> {
        // Gradle runs a module's unit tests from that module (core/ui).
        val root = androidRoot()
        val features = File(root, "feature").listFiles().orEmpty()
            .map { File(it, "src/main/kotlin") }
        val shared = listOf("core/ui", "app").map { File(root, "$it/src/main/kotlin") }
        return (features + shared)
            .filter { it.isDirectory }
            .sortedBy { it.path }
    }

    private fun scanFile(file: File, base: File): List<String> {
        val lines = file.readLines()
        val rel = file.canonicalFile.relativeTo(base.canonicalFile).path
        return lines.mapIndexedNotNull { index, line ->
            val trimmed = line.trim()
            if (trimmed.startsWith("//") || trimmed.startsWith("*")) return@mapIndexedNotNull null
            val why = when {
                THROWABLE_MESSAGE.containsMatchIn(line) && !isLogging(lines, index) ->
                    "an exception's message is English"

                LITERAL_FALLBACK.containsMatchIn(line) && shownAsText(lines, index) ->
                    "an English literal fallback shown as text"

                else -> null
            }
            when {
                why == null -> null
                ALLOWED.keys.any { "$rel: $trimmed".startsWith(it) } -> null
                else -> "$rel: ${index + 1}: $trimmed — $why"
            }
        }
    }

    /**
     * The fallback's statement feeds UI text: `stringResource(…)`, `text =` or
     * `Text(` on this line, or, for a wrapped `?: "unknown"` continuation line,
     * on one of the [CONTEXT_LINES] before it.
     */
    private fun shownAsText(lines: List<String>, index: Int): Boolean {
        if (UI_TEXT.containsMatchIn(lines[index])) return true
        // Only a wrapped continuation (`?: "x"` starting its own line) belongs
        // to the statement above; any other line is its own statement.
        if (!lines[index].trimStart().startsWith("?:")) return false
        val before = maxOf(0, index - CONTEXT_LINES) until index
        return before.any { UI_TEXT.containsMatchIn(lines[it]) }
    }

    private fun isLogging(lines: List<String>, index: Int): Boolean =
        (maxOf(0, index - 2)..index).any { LOGGING.containsMatchIn(lines[it]) }

    private companion object {
        val THROWABLE_MESSAGE = Regex(
            """\b(e|ex|t|th|throwable|failure|cause|exception|err|error)\??\.(message|localizedMessage)\b(?!\s*\()"""
        )
        const val CONTEXT_LINES = 3

        /** `?: "word…"`: an English fallback, not a token like "image/jpeg" (no space or slash check needed: see UI_TEXT). */
        val LITERAL_FALLBACK = Regex("""\?:\s*"[A-Za-z]""")
        val UI_TEXT = Regex("""\bstringResource\(|\btext\s*=|\bText\(""")
        val LOGGING = Regex("""\b(Log\.[vdiwe]|Timber\.|logger\.)""")

        /** "path: line-prefix" → why it is not UI text. Keep it empty if you can. */
        val ALLOWED: Map<String, String> = emptyMap()
    }
}
