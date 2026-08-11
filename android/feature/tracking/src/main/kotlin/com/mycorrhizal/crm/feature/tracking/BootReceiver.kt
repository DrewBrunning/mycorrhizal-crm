package com.mycorrhizal.crm.feature.tracking

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent

/**
 * Re-registers the periodic tracking workers after a reboot (§6.3
 * BootReceiver). WorkManager persists its own jobs across reboots, but this
 * is a belt-and-braces re-enqueue and the ticket specifies it.
 */
class BootReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action == Intent.ACTION_BOOT_COMPLETED) {
            TrackingWorkerScheduler.schedulePeriodic(context)
        }
    }
}
