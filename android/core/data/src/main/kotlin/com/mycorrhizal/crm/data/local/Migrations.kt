package com.mycorrhizal.crm.data.local

import androidx.room.migration.Migration
import androidx.sqlite.SQLiteConnection
import androidx.sqlite.execSQL

/**
 * T76: adds `phonesNormalized` (offline phone search — see [PhoneKey]) to `cached_contacts`
 * and its FTS mirror `cached_contacts_fts`.
 *
 * Hand-written rather than relying on `fallbackToDestructiveMigration` (this app's general
 * cache-rebuild policy, [AppDatabase]): `pending_interactions` is a real not-yet-synced
 * outbox — device call/SMS tracking staged for server sync (§6.1/6.2) — and a destructive
 * migration drops every table in the database, that one included. No other table's schema
 * changed for this version bump, so no other SQL is needed here.
 *
 * FTS4 virtual tables cannot `ALTER TABLE ... ADD COLUMN`, so `cached_contacts_fts` is dropped
 * and recreated with the new column list, then rebuilt from `cached_contacts`' current rows
 * via the FTS `'rebuild'` command (a fresh external-content FTS table starts with an empty
 * index; only future trigger-driven writes populate it otherwise). Room's own migration
 * pipeline drops and recreates every `@Fts4` entity's sync triggers around *every* registered
 * migration automatically (`RoomOpenDelegate.onPreMigrate`/`onPostMigrate`) — this migration
 * only needs to handle the table-level DDL delta, not the triggers themselves.
 */
val MIGRATION_13_14: Migration = object : Migration(startVersion = 13, endVersion = 14) {
    override fun migrate(connection: SQLiteConnection) {
        connection.execSQL("ALTER TABLE `cached_contacts` ADD COLUMN `phonesNormalized` TEXT")
        connection.execSQL("DROP TABLE IF EXISTS `cached_contacts_fts`")
        connection.execSQL(
            "CREATE VIRTUAL TABLE IF NOT EXISTS `cached_contacts_fts` USING FTS4(`fn` TEXT, " +
                "`firstname` TEXT, `lastname` TEXT, `primaryEmail` TEXT, `primaryPhone` TEXT, " +
                "`phonesNormalized` TEXT, `org` TEXT, content=`cached_contacts`)",
        )
        connection.execSQL("INSERT INTO `cached_contacts_fts`(`cached_contacts_fts`) VALUES('rebuild')")
    }
}
