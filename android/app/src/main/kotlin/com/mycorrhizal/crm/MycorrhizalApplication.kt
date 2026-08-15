package com.mycorrhizal.crm

import android.app.Application
import androidx.hilt.work.HiltWorkerFactory
import androidx.work.Configuration
import coil3.ImageLoader
import coil3.SingletonImageLoader
import coil3.network.okhttp.OkHttpNetworkFetcherFactory
import com.mycorrhizal.crm.data.repository.AppSettingsRepositoryImpl
import com.mycorrhizal.crm.feature.tracking.MycorrhizalNotificationChannels
import com.mycorrhizal.crm.feature.tracking.TrackingWorkerScheduler
import dagger.hilt.android.EntryPointAccessors
import dagger.hilt.android.HiltAndroidApp
import okhttp3.OkHttpClient
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

    // M5 §3.1: the app's authenticated OkHttp stack (BaseUrl + Auth
    // interceptors), reused by Coil so relative profile-photo URLs get
    // rewritten onto the server origin AND carry the bearer JWT.
    @Inject
    lateinit var okHttpClient: OkHttpClient

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
        // M5 §3.1: Coil must fetch photos through the same OkHttpClient the API
        // uses — its default network stack carries no Authorization header, so
        // an auth'd photo URL would 401 (the M1 landing note recorded this as
        // an open item; this closes it). BaseUrl runs before Auth in that
        // client, so a `mycorrhizal.invalid`-prefixed relative photo path is
        // rewritten to the real server and then gets the JWT.
        SingletonImageLoader.setSafe { context ->
            ImageLoader.Builder(context)
                .components {
                    add(OkHttpNetworkFetcherFactory(callFactory = { okHttpClient }))
                }
                .build()
        }
        // Notification channels: idempotent, safe every boot (§6.8).
        MycorrhizalNotificationChannels.createAll(this)
        // Periodic workers: interaction sync + reminder/cadence/birthday alerts.
        TrackingWorkerScheduler.schedulePeriodic(this)
    }
}
