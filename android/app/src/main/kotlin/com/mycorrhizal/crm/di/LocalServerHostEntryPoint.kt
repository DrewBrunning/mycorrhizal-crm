package com.mycorrhizal.crm.di

import com.mycorrhizal.crm.data.local.LocalServerHost
import dagger.hilt.EntryPoint
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent

/**
 * Reaches the app's singleton [LocalServerHost] from code Hilt does not inject
 * — the on-device local-mode instrumented tests (`LocalOnlyModeE2eTest`,
 * `LocalBundleRoundTripE2eTest`).
 *
 * It must live in the app's own sources, not in androidTest: Hilt aggregates
 * `@EntryPoint` interfaces into the component generated for
 * `MycorrhizalApplication` at app compile time, so one declared in the test APK
 * is never part of that component and `EntryPointAccessors.fromApplication`
 * fails with a ClassCastException. The tests use the real application (not
 * `HiltTestApplication`) because they exercise the real embedded server.
 */
@EntryPoint
@InstallIn(SingletonComponent::class)
interface LocalServerHostEntryPoint {
    fun localServerHost(): LocalServerHost
}
