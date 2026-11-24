package com.mycorrhizal.crm

import android.app.Application
import android.os.Build
import androidx.hilt.work.HiltWorkerFactory
import androidx.lifecycle.DefaultLifecycleObserver
import androidx.lifecycle.LifecycleOwner
import androidx.lifecycle.ProcessLifecycleOwner
import androidx.work.Configuration
import coil3.ImageLoader
import coil3.SingletonImageLoader
import coil3.network.okhttp.OkHttpNetworkFetcherFactory
import com.mycorrhizal.crm.data.local.LocalServerHost
import com.mycorrhizal.crm.data.repository.AppLocale
import com.mycorrhizal.crm.data.repository.AppSettingsRepositoryImpl
import com.mycorrhizal.crm.feature.tracking.MycorrhizalNotificationChannels
import com.mycorrhizal.crm.feature.tracking.TrackingWorkerScheduler
import dagger.hilt.android.HiltAndroidApp
import javax.inject.Inject
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import okhttp3.OkHttpClient

/**
 * Application implementing [Configuration.Provider] so WorkManager uses
 * Hilt's worker factory for @HiltWorker injection (the standard
 * Hilt + WorkManager wiring).
 */
@HiltAndroidApp
class MycorrhizalApplication :
    Application(),
    Configuration.Provider {

    @Inject
    lateinit var workerFactory: HiltWorkerFactory

    @Inject
    lateinit var appSettings: AppSettingsRepositoryImpl

    // M5 §3.1: the app's authenticated OkHttp stack (BaseUrl + Auth
    // interceptors), reused by Coil so relative profile-photo URLs get
    // rewritten onto the server origin AND carry the bearer JWT.
    @Inject
    lateinit var okHttpClient: OkHttpClient

    // ADR 0028 Decision 2 / issue #1262: the embedded local server. The app
    // starts it lazily (the transport interceptor) and stops it when the app is
    // backgrounded, so a local profile's server never keeps the process alive
    // in the background — the spike measured zero server-attributable wakeups
    // with this policy. Any later request (including a WorkManager run) restarts
    // it through the same lazy path.
    @Inject
    lateinit var localServerHost: LocalServerHost

    private val lifecycleScope = CoroutineScope(SupervisorJob() + Dispatchers.IO)

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
        // Issues #1921 / #2937: the OS per-app locale is authoritative on API 33+. A chosen OS
        // language is adopted into the local cache; an EMPTY OS locale means "System default",
        // so a stored override is cleared instead of being copied back into the OS (which made
        // "System default" in Android settings revert to the old language).
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            val reconciled = AppLocale.reconcileStoredOverride(AppLocale.systemLanguageTag(this))
            if (reconciled != AppLocale.languageTag) {
                AppLocale.languageTag = reconciled
                runBlocking { appSettings.setLanguageOverride(reconciled) }
            }
        }
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
        // ADR 0028 Decision 2: stop the embedded server when the app is no
        // longer visible. Process death is the other stop path (the child dies
        // with the app).
        ProcessLifecycleOwner.get().lifecycle.addObserver(object : DefaultLifecycleObserver {
            override fun onStop(owner: LifecycleOwner) {
                lifecycleScope.launch { localServerHost.stop() }
            }
        })
    }
}
