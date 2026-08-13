package com.mycorrhizal.crm.data.local

import android.content.Context
import android.database.sqlite.SQLiteDatabase
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * T76's real hazard: `pending_interactions` is a not-yet-synced outbox
 * (`AppDatabase`'s doc comment), not rebuildable cache data — a destructive migration on this
 * version bump would silently delete it, which is why [MIGRATION_13_14] is hand-written instead
 * of relying on `fallbackToDestructiveMigration`. This test builds a real v13-shaped database
 * (the pre-T76 schema, captured from a clean checkout's generated Room code), migrates it with
 * the real database builder, and asserts both halves of the ticket: no data loss, and the new
 * phone index actually works.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class Migration13To14Test {

    private lateinit var context: Context
    private lateinit var dbFile: java.io.File

    @Before
    fun setup() {
        context = ApplicationProvider.getApplicationContext()
        dbFile = context.getDatabasePath("migration-13-14-test.db")
        dbFile.delete()
    }

    @After
    fun teardown() {
        dbFile.delete()
    }

    /**
     * Hand-builds the exact v13 schema — every table Room's `RoomOpenDelegate.onValidateSchema`
     * checks after a migration, not just the two this migration touches, or validation fails on
     * tables that were never given a chance to differ. `cached_contacts`/`cached_contacts_fts`
     * are the pre-T76 shape (captured from a clean checkout's generated Room code, minus
     * `phonesNormalized`); every other table is unchanged between v13 and v14, so its DDL is
     * identical either way.
     */
    private fun createV13Database() {
        val db = SQLiteDatabase.openOrCreateDatabase(dbFile, null)
        db.execSQL(
            "CREATE TABLE `cached_contacts` (`id` INTEGER NOT NULL, `uid` TEXT, `firstname` TEXT, " +
                "`lastname` TEXT, `nickname` TEXT, `fn` TEXT, `primaryEmail` TEXT, `primaryPhone` TEXT, " +
                "`birthday` TEXT, `org` TEXT, `photoThumbnail` TEXT, `circles` TEXT, `archived` INTEGER NOT NULL, " +
                "`deleted` INTEGER NOT NULL, `deviceLookupKey` TEXT, `card` TEXT, `crm` TEXT, `updatedAt` TEXT, " +
                "PRIMARY KEY(`id`))",
        )
        db.execSQL(
            "CREATE VIRTUAL TABLE `cached_contacts_fts` USING FTS4(`fn` TEXT, `firstname` TEXT, " +
                "`lastname` TEXT, `primaryEmail` TEXT, `primaryPhone` TEXT, `org` TEXT, content=`cached_contacts`)",
        )
        db.execSQL(
            "CREATE TABLE `cached_activities` (`id` INTEGER NOT NULL, `title` TEXT, `description` TEXT, " +
                "`location` TEXT, `date` TEXT, `type` TEXT, `deleted` INTEGER NOT NULL, PRIMARY KEY(`id`))",
        )
        db.execSQL(
            "CREATE TABLE `cached_notes` (`id` INTEGER NOT NULL, `content` TEXT, `date` TEXT, " +
                "`deleted` INTEGER NOT NULL, PRIMARY KEY(`id`))",
        )
        db.execSQL(
            "CREATE TABLE `cached_reminders` (`id` INTEGER NOT NULL, `message` TEXT, `remindAt` TEXT, " +
                "`recurrence` TEXT, `completed` INTEGER NOT NULL, PRIMARY KEY(`id`))",
        )
        db.execSQL(
            "CREATE TABLE `cached_circles` (`id` TEXT NOT NULL, `name` TEXT NOT NULL, `updatedAt` TEXT, " +
                "PRIMARY KEY(`id`))",
        )
        db.execSQL(
            "CREATE TABLE `cached_circle_members` (`id` INTEGER NOT NULL, `circleId` TEXT NOT NULL, " +
                "`memberVCardUid` TEXT NOT NULL, PRIMARY KEY(`id`))",
        )
        db.execSQL(
            "CREATE TABLE `cached_tags` (`id` TEXT NOT NULL, `name` TEXT NOT NULL, `updatedAt` TEXT, " +
                "PRIMARY KEY(`id`))",
        )
        db.execSQL(
            "CREATE TABLE `cached_contact_tags` (`id` INTEGER NOT NULL, `tagId` TEXT NOT NULL, " +
                "`contactVCardUid` TEXT NOT NULL, PRIMARY KEY(`id`))",
        )
        db.execSQL(
            "CREATE TABLE `cached_households` (`id` TEXT NOT NULL, `name` TEXT NOT NULL, `type` TEXT NOT NULL, " +
                "`updatedAt` TEXT, PRIMARY KEY(`id`))",
        )
        db.execSQL(
            "CREATE TABLE `cached_household_members` (`id` INTEGER NOT NULL, `householdId` TEXT NOT NULL, " +
                "`memberVCardUid` TEXT NOT NULL, `role` TEXT, `since` TEXT, `until` TEXT, PRIMARY KEY(`id`))",
        )
        db.execSQL(
            "CREATE TABLE `cached_relationship_edges` (`id` TEXT NOT NULL, `sourceId` TEXT NOT NULL, " +
                "`targetId` TEXT NOT NULL, `type` TEXT NOT NULL, `directional` INTEGER NOT NULL, " +
                "`status` TEXT NOT NULL, `sensitivity` TEXT NOT NULL, `updatedAt` TEXT, `deleted` INTEGER NOT NULL, " +
                "PRIMARY KEY(`id`))",
        )
        db.execSQL(
            "CREATE TABLE `cached_life_events` (`id` TEXT NOT NULL, `entityId` TEXT NOT NULL, `type` TEXT, " +
                "`category` TEXT, `date` TEXT, `description` TEXT, `remind` INTEGER, `updatedAt` TEXT, " +
                "`deleted` INTEGER NOT NULL, PRIMARY KEY(`id`))",
        )
        db.execSQL(
            "CREATE TABLE `cached_gifts` (`id` TEXT NOT NULL, `entityId` TEXT NOT NULL, `status` TEXT NOT NULL, " +
                "`occasion` TEXT, `description` TEXT NOT NULL, `updatedAt` TEXT, `deleted` INTEGER NOT NULL, " +
                "PRIMARY KEY(`id`))",
        )
        db.execSQL(
            "CREATE TABLE `cached_preferences` (`id` TEXT NOT NULL, `entityId` TEXT NOT NULL, " +
                "`category` TEXT NOT NULL, `key` TEXT, `value` TEXT NOT NULL, `sensitivity` TEXT NOT NULL, " +
                "`updatedAt` TEXT, `deleted` INTEGER NOT NULL, PRIMARY KEY(`id`))",
        )
        db.execSQL(
            "CREATE TABLE `cached_conversation_agenda` (`id` TEXT NOT NULL, `entityId` TEXT NOT NULL, " +
                "`content` TEXT NOT NULL, `referenceUrl` TEXT, `discussedAt` TEXT, `updatedAt` TEXT, " +
                "`deleted` INTEGER NOT NULL, PRIMARY KEY(`id`))",
        )
        db.execSQL(
            "CREATE TABLE `pending_interactions` (`id` INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, " +
                "`timestampMillis` INTEGER NOT NULL, `kind` TEXT NOT NULL, `direction` TEXT, `phoneNumber` TEXT, " +
                "`matchedContactId` INTEGER, `synced` INTEGER NOT NULL, `syncedAt` TEXT)",
        )
        db.execSQL(
            "CREATE TABLE `custom_link_actions` (`id` INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, " +
                "`protocol` TEXT NOT NULL, `label` TEXT NOT NULL, `kind` TEXT NOT NULL, `mimeType` TEXT NOT NULL, " +
                "`intentUriTemplate` TEXT NOT NULL)",
        )
        db.execSQL(
            "INSERT INTO cached_contacts (id, fn, primaryPhone, archived, deleted) " +
                "VALUES (1, 'Dana White', '(800) 555-1234', 0, 0)",
        )
        db.execSQL(
            "INSERT INTO pending_interactions (timestampMillis, kind, phoneNumber, synced) " +
                "VALUES (1000, 'call', '5551234', 0)",
        )
        db.version = 13
        db.close()
    }

    @Test
    fun `migration preserves the pending outbox and enables phone search`() = runBlocking {
        createV13Database()

        val db = Room.databaseBuilder(context, AppDatabase::class.java, dbFile.absolutePath)
            .addMigrations(MIGRATION_13_14)
            .build()

        // The not-yet-synced outbox row survived — the whole reason this migration is
        // hand-written instead of destructive.
        val pending = db.pendingInteractionDao().getUnsynced()
        assertEquals(1, pending.size)
        assertEquals("5551234", pending[0].phoneNumber)

        // The pre-existing cached_contacts row is still there too (an ALTER TABLE ADD COLUMN
        // preserves rows; only a destructive rebuild would have dropped it).
        val contact = db.cachedContactDao().getById(1)
        assertEquals("Dana White", contact?.fn)

        // The FTS mirror was dropped, recreated with the new column, and its content-sync
        // triggers rebuilt by Room's own migration pipeline (onPostMigrate) — proven by
        // writing a fresh row through the DAO (not raw SQL) and confirming the recreated
        // triggers actually keep cached_contacts_fts in sync under the new column set.
        db.cachedContactDao().upsert(
            CachedContact(
                id = 2,
                fn = "New Contact",
                primaryPhone = "555-0100",
                phonesNormalized = PhoneKey.flatten(listOf("555-0100")),
            ),
        )
        val found = db.cachedContactDao().searchFtsMatch("phonesNormalized:5550100*")
        assertEquals(1, found.size)
        assertEquals("New Contact", found[0].fn)

        db.close()
    }
}
