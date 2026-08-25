package com.mycorrhizal.crm.data.local

import android.content.Context
import com.mycorrhizal.crm.data.session.SessionDataCleaner
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/**
 * Issue #385: wipes the device's cached user data when the session ends
 * (logout, or account removal surfacing as an invalid token). The Room mirror
 * is a server-rebuildable cache, so [AppDatabase.clearAllTables] — which drops
 * every row including the FTS4 index, then rebuilds it — fully purges offline
 * PII; the cached profile-photo/attachment files (Coil's disk cache + the
 * FileProvider's vCard share staging, both under [Context.cacheDir]) are
 * deleted alongside so the "stolen device" story leaves no recoverable contact
 * data outside the (now encrypted) database.
 *
 * The not-yet-synced `pending_interactions` outbox is intentionally cleared
 * here too: ending a session is an explicit hand-over of the device, and the
 * privacy wipe takes precedence over that rebuildable-from-phone-metadata data.
 */
class LocalDataCleaner(
    private val context: Context,
    private val database: AppDatabase,
) : SessionDataCleaner {

    override suspend fun clear() {
        withContext(Dispatchers.IO) {
            database.clearAllTables()
            context.cacheDir.deleteRecursively()
        }
    }
}
