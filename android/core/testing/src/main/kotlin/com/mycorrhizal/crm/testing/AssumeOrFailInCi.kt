package com.mycorrhizal.crm.testing

import org.junit.Assume.assumeTrue

/**
 * Issue #1483: the Android analogue of the Go side's `internal/citest.SkipOrRequire`.
 *
 * A plain `assumeTrue` turns a missing CI-provided dependency (e.g. the
 * `docker-compose.compat-test.yml` backends) into a *skip*, and a skip reports green on a
 * required, release-gating check. This helper skips locally (a developer without the backend
 * running still gets a clean run) but FAILS when the run is told it is a reference-providing CI
 * run via the instrumentation argument `requireReferences=true`
 * (`-Pandroid.testInstrumentationRunnerArguments.requireReferences=true`).
 *
 * Use it ONLY for conditions CI always satisfies. A condition that is legitimately false on the
 * CI emulator (the arm64-only embedded server, ADR 0028) must stay a plain `assumeTrue` and be
 * listed, with a reason, in `android/e2e-expected-skips.txt` instead.
 */
const val REQUIRE_REFERENCES_ARG = "requireReferences"

/** True when [value] is the instrumentation-argument spelling of "yes" (case-insensitive `true`). */
internal fun isRequireReferences(value: String?): Boolean = value?.trim().equals("true", ignoreCase = true)

/** Core decision, argument-injected so it is JVM-testable. */
internal fun assumeOrFailInCi(
    message: String,
    condition: Boolean,
    requireReferences: Boolean,
) {
    if (condition) return
    if (requireReferences) {
        throw AssertionError(
            "$message (requireReferences=true: this dependency is always provided in CI, " +
                "so this is a failure, not a skip)",
        )
    }
    assumeTrue(message, false)
}

/** Reads `requireReferences` from the running instrumentation's arguments. */
private fun requireReferencesFromInstrumentation(): Boolean =
    isRequireReferences(
        androidx.test.platform.app.InstrumentationRegistry
            .getArguments()
            .getString(REQUIRE_REFERENCES_ARG),
    )

/** Skip locally, fail in CI (`requireReferences=true`) when [condition] is false. */
fun assumeOrFailInCi(
    message: String,
    condition: Boolean,
) = assumeOrFailInCi(message, condition, requireReferencesFromInstrumentation())
