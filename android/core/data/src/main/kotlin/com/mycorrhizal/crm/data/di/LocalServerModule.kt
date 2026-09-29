package com.mycorrhizal.crm.data.di

import android.content.Context
import com.mycorrhizal.crm.data.local.AndroidLocalServerHealthProbe
import com.mycorrhizal.crm.data.local.AndroidLocalServerHost
import com.mycorrhizal.crm.data.local.AndroidLocalServerProcessLauncher
import com.mycorrhizal.crm.data.local.KeystoreLocalServerSecretCipherProvider
import com.mycorrhizal.crm.data.local.LocalServerAvailability
import com.mycorrhizal.crm.data.local.LocalServerBinaryProvider
import com.mycorrhizal.crm.data.local.LocalServerHealthProbe
import com.mycorrhizal.crm.data.local.LocalServerHost
import com.mycorrhizal.crm.data.local.LocalServerPaths
import com.mycorrhizal.crm.data.local.LocalServerProcessLauncher
import com.mycorrhizal.crm.data.local.LocalServerSecretCipherProvider
import com.mycorrhizal.crm.data.local.LocalServerSecretStore
import dagger.Binds
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.android.qualifiers.ApplicationContext
import dagger.hilt.components.SingletonComponent
import java.io.File
import javax.inject.Singleton

/**
 * ADR 0028 Decision 2 / issue #1262: the embedded local-server host. The
 * secrets store and the host are singletons — exactly one embedded server may
 * run in the process — and every path is app-private under `filesDir`, so it
 * survives neither logout nor the Room mirror's wipe and is deleted only by the
 * explicit "Delete local data" action.
 */
@Module
@InstallIn(SingletonComponent::class)
abstract class LocalServerModule {

    @Binds
    @Singleton
    abstract fun bindLocalServerHost(impl: AndroidLocalServerHost): LocalServerHost

    companion object {

        @Provides
        @Singleton
        fun provideLocalServerPaths(@ApplicationContext context: Context): LocalServerPaths =
            LocalServerPaths.under(context.filesDir)

        @Provides
        @Singleton
        fun provideLocalServerBinaryProvider(
            @ApplicationContext context: Context,
        ): LocalServerBinaryProvider = LocalServerBinaryProvider {
            File(context.applicationInfo.nativeLibraryDir, LocalServerAvailability.BINARY_NAME)
                .takeIf { it.exists() }
        }

        @Provides
        @Singleton
        fun provideLocalServerSecretCipherProvider(): LocalServerSecretCipherProvider =
            KeystoreLocalServerSecretCipherProvider()

        @Provides
        @Singleton
        fun provideLocalServerSecretStore(
            @ApplicationContext context: Context,
            ciphers: LocalServerSecretCipherProvider,
        ): LocalServerSecretStore = LocalServerSecretStore(
            keyFile = File(context.filesDir, "local-server/keys.bin"),
            ciphers = ciphers,
        )

        @Provides
        @Singleton
        fun provideLocalServerProcessLauncher(): LocalServerProcessLauncher =
            AndroidLocalServerProcessLauncher()

        @Provides
        @Singleton
        fun provideLocalServerHealthProbe(): LocalServerHealthProbe =
            AndroidLocalServerHealthProbe()
    }
}
