package com.mycorrhizal.crm.storage

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.mycorrhizal.crm.data.local.AppDatabase
import com.mycorrhizal.crm.data.local.CachedCadencePolicy
import com.mycorrhizal.crm.data.local.PhoneKey
import com.mycorrhizal.crm.data.local.REGISTERED_MIGRATIONS
import java.io.File
import kotlinx.coroutines.runBlocking
import net.zetetic.database.sqlcipher.SupportOpenHelperFactory
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Issue #480 recommended action 6: the real upgrade path this app's users take. A device
 * upgrading from an old APK to a new one keeps its private app data directory — including the
 * Room mirror file — exactly as `adb install -r` (or a Play/F-Droid-style in-place update) would;
 * the DB file never round-trips through anything the JVM-side `Migration*Test` suite can't
 * already exercise *except* SQLCipher itself, which is Android-native and only runs here (see
 * [RoomCacheEncryptionTest]'s doc comment). This test is that missing piece: the real registered
 * migration chain, run against a real SQLCipher-encrypted file, on a real device/emulator.
 *
 * A literal two-APK `adb install -r` dance was considered and rejected as the mechanism for this
 * test: the ticket itself notes release APKs already carry a real monotonic `versionCode`
 * (`docker-publish.yml`), which is what makes Android accept the install in the first place —
 * that mechanism is orthogonal to whether the *database* migrates correctly once the new code
 * runs, which is exactly what building the old schema directly and opening it with the current
 * `AppDatabase` (this test, and every `Migration*Test`) proves. A real install would exercise
 * process/package mechanics this repo has no gap in; it would not add coverage of the migration
 * SQL itself.
 */
@RunWith(AndroidJUnit4::class)
class RoomMigrationEncryptedTest {

    private val context: Context = ApplicationProvider.getApplicationContext()
    private val passphrase = "1a1b1c1d1e1f202122232425262728292a2b2c2d2e2f303132333435363738"

    @Before
    fun setUp() {
        System.loadLibrary("sqlcipher")
    }

    private lateinit var dbFile: File

    @After
    fun tearDown() {
        if (::dbFile.isInitialized) {
            dbFile.delete()
            listOf("-journal", "-wal", "-shm").forEach { suffix ->
                File(dbFile.parentFile, dbFile.name + suffix).delete()
            }
        }
    }

    /**
     * The real, ever-shipped v13 shape (same DDL as
     * `Migration13To14Test.createV13Database`/`LocalDatabaseSchemaFixtures`), but built through
     * SQLCipher's own `SQLiteDatabase` rather than the framework one — a real device's mirror
     * file has always been encrypted since issue #385 shipped, so this is what an actual v13
     * install's file looks like on disk, not a plaintext stand-in.
     */
    private fun createEncryptedV13Database() {
        dbFile = context.getDatabasePath("room-migration-encrypted-test.db")
        dbFile.delete()
        val db = net.zetetic.database.sqlcipher.SQLiteDatabase.openOrCreateDatabase(
            dbFile,
            passphrase.toByteArray(),
            null,
            null,
        )
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
    fun realDeviceUpgradeMigratesTheEncryptedOutboxAndCacheIntact() = runBlocking {
        createEncryptedV13Database()

        // The exact same builder shape DataModule uses in production: encrypted open helper +
        // the real registered migrations, no destructive fallback needed since every hop from
        // v13 to current has a migration.
        val db = Room.databaseBuilder(context, AppDatabase::class.java, dbFile.absolutePath)
            .openHelperFactory(SupportOpenHelperFactory(passphrase.toByteArray()))
            .addMigrations(*REGISTERED_MIGRATIONS.toTypedArray())
            .build()

        // The not-yet-synced outbox — the entire reason these migrations are hand-written —
        // survives the full chain under encryption, not just under the plaintext factory the
        // JVM `Migration*Test` suite is limited to.
        val pending = db.pendingInteractionDao().getUnsynced()
        assertEquals(1, pending.size)
        assertEquals("5551234", pending[0].phoneNumber)

        // The pre-existing contact survived, its new isFavorite column defaulted correctly, and
        // the FTS mirror (dropped/rebuilt mid-chain) is searchable end to end under encryption —
        // proven by writing a fresh row through the DAO and searching for it, the same shape
        // Migration13To14Test uses for the plaintext case.
        val contact = db.cachedContactDao().getById(1)
        assertEquals("Dana White", contact?.fn)
        assertFalse(contact?.isFavorite ?: true)

        db.cachedContactDao().upsert(
            com.mycorrhizal.crm.data.local.CachedContact(
                id = 2,
                fn = "New Contact",
                primaryPhone = "555-0100",
                phonesNormalized = PhoneKey.flatten(listOf("555-0100")),
            ),
        )
        val found = db.cachedContactDao().searchFtsMatch("phonesNormalized:5550100*")
        assertEquals(1, found.size)
        assertEquals("New Contact", found[0].fn)

        // The M12 cadence-policies table (added mid-chain) is fully usable, not just present.
        db.cachedCadencePolicyDao().upsert(
            CachedCadencePolicy(id = "policy-1", entityId = "contact-1", targetIntervalDays = 30),
        )
        assertEquals(1, db.cachedCadencePolicyDao().getForContact("contact-1").size)

        db.close()
    }
}
