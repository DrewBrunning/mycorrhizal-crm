package com.mycorrhizal.crm.data.local

import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test

/**
 * Issue #480 recommended action 3: the real hazard this whole ticket exists to catch is a future
 * version bump that adds no migration and no deliberate decision — [AppDatabase]'s
 * `fallbackToDestructiveMigration` then silently wipes `pending_interactions` (the not-yet-synced
 * outbox) for anyone caught on that version pair, with no error, no warning, nothing. This test
 * is the guard: every consecutive version pair from [EARLIEST_KNOWN_VERSION] to the database's
 * current [AppDatabase.version] must be covered by either a registered [Migration] in
 * [REGISTERED_MIGRATIONS] or an explicit entry in [ACCEPTED_DESTRUCTIVE_GAPS] — leaving a pair in
 * neither list fails this test, not a silent wipe in production.
 *
 * Hand-verify (per CLAUDE.md): comment out one entry in [REGISTERED_MIGRATIONS] and confirm this
 * test starts failing, then restore it — a guard that has never failed has proven nothing.
 */
class MigrationVersionCoverageTest {

    @Test
    fun `every version pair from the earliest known version to current is covered`() {
        assertTrue(
            "EARLIEST_KNOWN_VERSION ($EARLIEST_KNOWN_VERSION) must be below CURRENT_VERSION ($CURRENT_VERSION)",
            EARLIEST_KNOWN_VERSION < CURRENT_VERSION,
        )

        val registeredStarts = REGISTERED_MIGRATIONS.map { it.startVersion }.toSet()
        val uncovered = mutableListOf<String>()

        for (start in EARLIEST_KNOWN_VERSION until CURRENT_VERSION) {
            val end = start + 1
            val hasMigration = start in registeredStarts
            val isAcceptedGap = start in ACCEPTED_DESTRUCTIVE_GAPS
            if (!hasMigration && !isAcceptedGap) {
                uncovered += "$start -> $end"
            }
            if (hasMigration && isAcceptedGap) {
                fail(
                    "Version pair $start -> $end is both a registered migration AND an accepted " +
                        "destructive gap — that's a contradiction, not a valid state. Pick one.",
                )
            }
        }

        assertTrue(
            "The following version pairs have neither a registered Migration in " +
                "REGISTERED_MIGRATIONS nor a documented entry in ACCEPTED_DESTRUCTIVE_GAPS — an " +
                "upgrade across one of these silently wipes pending_interactions via " +
                "fallbackToDestructiveMigration with no warning: $uncovered",
            uncovered.isEmpty(),
        )
    }

    @Test
    fun `every registered migration's start and end versions are exactly adjacent`() {
        // A Migration spanning more than one version (e.g. 13 -> 15) would make the version-pair
        // sweep above miscount coverage — REGISTERED_MIGRATIONS is only ever built one hop per
        // entry in this codebase, and this pins that assumption.
        for (migration in REGISTERED_MIGRATIONS) {
            assertTrue(
                "Migration ${migration.startVersion} -> ${migration.endVersion} is not a single " +
                    "adjacent hop; the coverage sweep assumes startVersion + 1 == endVersion",
                migration.endVersion == migration.startVersion + 1,
            )
        }
    }

    @Test
    fun `REGISTERED_MIGRATIONS has no duplicate start versions`() {
        val starts = REGISTERED_MIGRATIONS.map { it.startVersion }
        assertTrue(
            "Two registered migrations start at the same version — Room would only ever run " +
                "one of them: $starts",
            starts.size == starts.toSet().size,
        )
    }
}
