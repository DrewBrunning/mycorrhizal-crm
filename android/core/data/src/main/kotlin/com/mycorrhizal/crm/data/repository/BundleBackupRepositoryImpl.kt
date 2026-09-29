package com.mycorrhizal.crm.data.repository

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.longPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.domain.backup.BundleBackupStatus
import com.mycorrhizal.crm.domain.profile.ServerProfileKind
import com.mycorrhizal.crm.domain.repository.BundleBackupRepository
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.flow.map

private val Context.bundleBackupDataStore: DataStore<Preferences> by preferencesDataStore(name = "bundle_backup")

/**
 * DataStore-backed [BundleBackupRepository]. Keys are `last_export:<profileId>`
 * and `dismissed:<profileId>`, so each profile's bookkeeping is isolated and a
 * profile switch simply re-keys the flow.
 */
@Singleton
class BundleBackupRepositoryImpl @Inject constructor(
    @ApplicationContext private val context: Context,
    private val sessionManager: SessionManager,
) : BundleBackupRepository {

    @OptIn(ExperimentalCoroutinesApi::class)
    override fun observeStatus(): Flow<BundleBackupStatus> =
        sessionManager.observeActiveProfile().flatMapLatest { profile ->
            if (profile == null) {
                flowOf(BundleBackupStatus())
            } else {
                context.bundleBackupDataStore.data.map { prefs ->
                    BundleBackupStatus(
                        isLocalProfile = profile.kind is ServerProfileKind.Local,
                        lastExportAt = prefs[lastExportKey(profile.id)],
                        dismissedAt = prefs[dismissedKey(profile.id)],
                    )
                }
            }
        }

    override suspend fun recordExport(atMillis: Long) {
        val id = sessionManager.activeProfileId() ?: return
        // A fresh export satisfies the reminder, so it also clears any snooze.
        context.bundleBackupDataStore.edit {
            it[lastExportKey(id)] = atMillis
            it.remove(dismissedKey(id))
        }
    }

    override suspend fun dismissReminder(atMillis: Long) {
        val id = sessionManager.activeProfileId() ?: return
        context.bundleBackupDataStore.edit { it[dismissedKey(id)] = atMillis }
    }

    override suspend fun forget(profileId: String) {
        context.bundleBackupDataStore.edit {
            it.remove(lastExportKey(profileId))
            it.remove(dismissedKey(profileId))
        }
    }

    private fun lastExportKey(profileId: String) = longPreferencesKey("last_export:$profileId")
    private fun dismissedKey(profileId: String) = longPreferencesKey("dismissed:$profileId")
}
