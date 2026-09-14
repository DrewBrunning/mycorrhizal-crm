package com.mycorrhizal.crm.data.local

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.model.MoshiProvider
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withContext
import org.junit.Assert.assertEquals
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Pins the committed Room schema dump (`core/data/schemas/.../$CURRENT_VERSION.json`) to
 * the schema KSP actually generated for this build.
 *
 * The dump is the artifact `MigrationTestHelper` would build a historical database from
 * ([AppDatabase]'s doc comment), so a stale one is not cosmetic: it describes a database
 * shape the compiled `AppDatabase` would reject. This happened — I18N-02 (issue #485)
 * switched the `cached_contacts_fts` tokenizer to `unicode61` and committed an `18.json`
 * that was a byte-for-byte copy of the v17 dump (same `identityHash`, `tokenizer: simple`),
 * so the dump disagreed with both the entity and `MIGRATION_17_18`.
 *
 * Room stamps a compiled-in `identityHash` into `room_master_table` when it creates a
 * database; the same hash is what KSP writes into the schema dump. Opening an in-memory
 * `AppDatabase` and comparing its identity hash to the dump's is therefore a direct,
 * general check that the committed dump matches the generated schema — any entity change
 * that alters the schema (a column, a tokenizer, an index) moves the hash and fails here
 * until the dump is regenerated. `android/core/data/build.gradle.kts` wires `schemas/`
 * onto this source set's assets, which is how the test reads the committed file.
 *
 * Regenerate with `cd android && ./gradlew :core:data:kspDebugKotlin --rerun` and commit
 * the resulting diff. The `Room schema dumps are current` step in `android-tests.yml` runs
 * the same regeneration and fails on a dirty tree, so this cannot silently rot.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class RoomSchemaDumpTest {

    @Test
    fun `the committed schema dump matches the generated database identity`() = runBlocking {
        val context = ApplicationProvider.getApplicationContext<Context>()

        val runtimeHash = withContext(Dispatchers.IO) {
            val db = Room.inMemoryDatabaseBuilder(context, AppDatabase::class.java).build()
            try {
                db.query("SELECT identity_hash FROM room_master_table", null).use { cursor ->
                    check(cursor.moveToFirst()) { "room_master_table has no identity row" }
                    cursor.getString(0)
                }
            } finally {
                db.close()
            }
        }

        val dumpedHash = withContext(Dispatchers.IO) {
            val asset = "${AppDatabase::class.qualifiedName}/$CURRENT_VERSION.json"
            val json = context.assets.open(asset).bufferedReader().readText()
            val root = MoshiProvider.get().adapter(Any::class.java).fromJson(json) as Map<*, *>
            (root["database"] as Map<*, *>)["identityHash"] as String
        }

        assertEquals(
            "The committed Room schema dump is stale — regenerate it with " +
                "`./gradlew :core:data:kspDebugKotlin --rerun` and commit the diff.",
            dumpedHash,
            runtimeHash,
        )
    }
}
