package com.mycorrhizal.crm.feature.tracking

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.PendingInteraction
import com.mycorrhizal.crm.domain.repository.PendingInteractionRepository
import com.mycorrhizal.crm.domain.repository.TrackingSettingsRepository
import dagger.hilt.android.EntryPointAccessors
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch

/**
 * Captures received SMS as PendingInteractions (§6.2). Runs the capture in a
 * coroutine on a background scope (broadcast onReceive must not block). Only
 * the sender number + timestamp are recorded — never the message body.
 *
 * DI (issue #327): the manifest instantiates the public no-arg constructor,
 * which has no injected dependencies, so the real repositories are resolved
 * lazily from the app's Hilt entry point on the first broadcast. This is
 * deliberately NOT `@AndroidEntryPoint`: Hilt wraps `onReceive()` — the
 * receiver's only entry point — with codegen that unconditionally requires a
 * real `@HiltAndroidApp Application` (which no test in this repo boots), so
 * the receiver was previously untestable. The internal constructor injects
 * the dependencies directly (mirroring CallLogSyncWorker's constructor
 * shape), skipping Hilt entirely.
 */
class SmsReceiver private constructor(
    private val dependencyProvider: (Context) -> SmsDependencies,
    private val parseSms: (Intent) -> SmsEntry?,
    private val dispatcher: CoroutineDispatcher,
) : BroadcastReceiver() {

    /** Manifest-instantiated path: no DI at construction time, resolve lazily. */
    constructor() : this(
        dependencyProvider = ::resolveSmsReceiverDependencies,
        parseSms = SmsReader::parseFromExtras,
        dispatcher = Dispatchers.IO,
    )

    /** Direct-construction path (tests): dependencies injected, no Hilt component touched. */
    internal constructor(
        pendingInteractionRepository: PendingInteractionRepository,
        contactRepository: ContactRepository,
        trackingSettings: TrackingSettingsRepository,
        dispatcher: CoroutineDispatcher,
        parseSms: (Intent) -> SmsEntry?,
    ) : this(
        dependencyProvider = {
            SmsDependencies(
                pendingInteractionRepository = pendingInteractionRepository,
                contactRepository = contactRepository,
                trackingSettings = trackingSettings,
            )
        },
        parseSms = parseSms,
        dispatcher = dispatcher,
    )

    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != android.provider.Telephony.Sms.Intents.SMS_RECEIVED_ACTION) return
        val sms = parseSms(intent) ?: return
        val scope = CoroutineScope(SupervisorJob() + dispatcher)
        scope.launch {
            val deps = dependencyProvider(context.applicationContext)
            if (!deps.trackingSettings.smsTrackingEnabled()) return@launch
            val contact = runCatching {
                sms.address?.let { deps.contactRepository.findByPhone(it) }
            }.getOrNull()
            deps.pendingInteractionRepository.record(
                PendingInteraction(
                    timestampMillis = sms.timestampMillis,
                    kind = InteractionCapture.KIND_MESSAGE,
                    direction = InteractionCapture.DIR_INCOMING,
                    phoneNumber = sms.address,
                    matchedContactId = contact?.id,
                ),
            )
        }
    }

    @dagger.hilt.EntryPoint
    @dagger.hilt.InstallIn(dagger.hilt.components.SingletonComponent::class)
    interface SmsReceiverEntryPoint {
        fun pendingInteractions(): PendingInteractionRepository
        fun contacts(): ContactRepository
        fun trackingSettings(): TrackingSettingsRepository
    }
}

/** The three repositories [SmsReceiver] needs to capture one SMS. */
internal data class SmsDependencies(
    val pendingInteractionRepository: PendingInteractionRepository,
    val contactRepository: ContactRepository,
    val trackingSettings: TrackingSettingsRepository,
)

// Production-only: resolves the real repositories from the app's Hilt entry
// point for the manifest-instantiated receiver. Structurally untestable — it
// requires a real @HiltAndroidApp Application, and a per-class Hilt test
// component pulls in the whole app graph (issue #327 documents both failed
// attempts) — so the coverage gate excludes it.
private fun resolveSmsReceiverDependencies(appContext: Context): SmsDependencies {
    val entryPoint = EntryPointAccessors.fromApplication(appContext, SmsReceiver.SmsReceiverEntryPoint::class.java) // # pragma: no cover — needs a Hilt Application (issue #327)
    return SmsDependencies(entryPoint.pendingInteractions(), entryPoint.contacts(), entryPoint.trackingSettings()) // # pragma: no cover — needs a Hilt Application (issue #327)
}
