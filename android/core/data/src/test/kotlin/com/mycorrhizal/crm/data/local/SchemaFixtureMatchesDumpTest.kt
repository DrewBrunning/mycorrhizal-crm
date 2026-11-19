package com.mycorrhizal.crm.data.local

import android.content.Context
import android.database.sqlite.SQLiteDatabase
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.model.MoshiProvider
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * The hand-built "before" databases in [LocalDatabaseSchemaFixtures] are only worth trusting if
 * they are the schema a real release shipped. For versions with a committed Room dump
 * (`core/data/schemas/.../N.json`, 16 and up) this pins the fixture to the dump: every table's
 * columns, affinities, nullability and primary key must be identical, so a migration test that
 * "starts from v19" starts from the real v19 (the shape v1.6.0 shipped) and not from a guess.
 * FTS shadow tables are skipped (the dump models them as `ftsVersion` entities, not columns).
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class SchemaFixtureMatchesDumpTest {

    private data class Col(val name: String, val type: String, val notNull: Boolean, val pk: Int)

    private lateinit var dbFile: java.io.File

    @Before
    fun setup() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        dbFile = context.getDatabasePath("fixture-vs-dump-test.db")
        dbFile.delete()
    }

    @After
    fun teardown() {
        dbFile.delete()
    }

    @Suppress("UNCHECKED_CAST")
    private fun dumpedTables(version: Int): Map<String, Set<Col>> {
        val context = ApplicationProvider.getApplicationContext<Context>()
        val json = context.assets.open("${AppDatabase::class.qualifiedName}/$version.json")
            .bufferedReader().readText()
        val root = MoshiProvider.get().adapter(Any::class.java).fromJson(json) as Map<*, *>
        val entities = (root["database"] as Map<*, *>)["entities"] as List<Map<String, Any?>>
        return entities.filter { it["ftsVersion"] == null }.associate { entity ->
            val pkColumns = ((entity["primaryKey"] as Map<*, *>)["columnNames"] as List<*>)
                .map { it as String }
            val fields = entity["fields"] as List<Map<String, Any?>>
            entity["tableName"] as String to fields.map { f ->
                val name = f["columnName"] as String
                Col(
                    name = name,
                    type = f["affinity"] as String,
                    notNull = f["notNull"] == true, // omitted from the dump when false
                    pk = pkColumns.indexOf(name) + 1
                )
            }.toSet()
        }
    }

    private fun fixtureColumns(db: SQLiteDatabase, table: String): Set<Col> =
        db.rawQuery("PRAGMA table_info(`$table`)", null).use { c ->
            buildSet {
                while (c.moveToNext()) {
                    add(
                        Col(
                            name = c.getString(c.getColumnIndexOrThrow("name")),
                            type = c.getString(c.getColumnIndexOrThrow("type")).uppercase(),
                            notNull = c.getInt(c.getColumnIndexOrThrow("notnull")) == 1,
                            // PRAGMA pk is 1-based position within the key, 0 when not in it.
                            pk = c.getInt(c.getColumnIndexOrThrow("pk"))
                        )
                    )
                }
            }
        }

    @Test
    fun `the v19 fixture has exactly the tables and columns of the committed 19 json`() {
        val dumped = dumpedTables(19)
        val db = SQLiteDatabase.openOrCreateDatabase(dbFile, null)
        try {
            LocalDatabaseSchemaFixtures.createV19Tables(db)
            assertTrue("the dump lists tables", dumped.isNotEmpty())
            assertTrue(dumped.containsKey("cached_circles"))
            for ((table, columns) in dumped) {
                assertEquals("columns of $table", columns, fixtureColumns(db, table))
            }
        } finally {
            db.close()
        }
    }
}
