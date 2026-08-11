package com.mycorrhizal.crm.feature.tracking

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.PendingInteraction
import com.mycorrhizal.crm.domain.repository.PendingInteractionRepository
import com.mycorrhizal.crm.domain.repository.TrackingSettingsRepository
import dagger.hilt.android.EntryPointAccessors
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch

/**
 * Captures received SMS as PendingInteractions (§6.2). Runs the capture in a
 * coroutine on a background scope (broadcast onReceive must not block). Only
 * the sender number + timestamp are recorded — never the message body.
 */
class SmsReceiver : BroadcastReceiver() {

    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != android.provider.Telephony.Sms.Intents.SMS_RECEIVED_ACTION) return
        val sms = SmsReader.parseFromExtras(intent) ?: return
        val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
        scope.launch {
            val entryPoint = EntryPointAccessors.fromApplication(
                context.applicationContext,
                SmsReceiverEntryPoint::class.java,
            )
            if (!entryPoint.trackingSettings().smsTrackingEnabled()) return@launch
            val contact = runCatching {
                sms.address?.let { entryPoint.contacts().findByPhone(it) }
            }.getOrNull()
            entryPoint.pendingInteractions().record(
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
    }}
