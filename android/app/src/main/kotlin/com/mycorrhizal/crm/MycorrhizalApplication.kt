package com.mycorrhizal.crm

import android.app.Application
import androidx.hilt.work.HiltWorkerFactory
import androidx.work.Configuration
import com.mycorrhizal.crm.data.repository.AppSettingsRepositoryImpl
import com.mycorrhizal.crm.feature.tracking.MycorrhizalNotificationChannels
import com.mycorrhizal.crm.feature.tracking.TrackingWorkerScheduler
import dagger.hilt.android.EntryPointAccessors
import dagger.hilt.android.HiltAndroidApp
import javax.inject.Inject
import kotlinx.coroutines.runBlocking

/**
 * Application implementing [Configuration.Provider] so WorkManager uses
 * Hilt's worker factory for @HiltWorker injection (the standard
 * Hilt + WorkManager wiring).
 */
@HiltAndroidApp
class MycorrhizalApplication : Application(), Configuration.Provider {

    @Inject
    lateinit var workerFactory: HiltWorkerFactory

    @Inject
    lateinit var appSettings: AppSettingsRepositoryImpl

    override val workManagerConfiguration: Configuration
        get() = Configuration.Builder()
            .setWorkerFactory(workerFactory)
            .build()

    override fun onCreate() {
        super.onCreate()
        // M25: hydrate the language override into the sync cache so the very
        // first activity frame resolves values-XX resources in the user's
        // chosen language (attachBaseContext reads currentLanguageOverride()).
        // One tiny prefs file, once, at startup.
        runBlocking { appSettings.hydrate() }
        // Notification channels: idempotent, safe every boot (§6.8).
        MycorrhizalNotificationChannels.createAll(this)
        // Periodic workers: interaction sync + reminder/cadence/birthday alerts.
        TrackingWorkerScheduler.schedulePeriodic(this)
    }
}
