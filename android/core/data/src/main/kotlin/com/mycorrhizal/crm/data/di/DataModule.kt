package com.mycorrhizal.crm.data.di

import com.mycorrhizal.crm.data.local.AppDatabase
import com.mycorrhizal.crm.data.local.CachedContactDao
import com.mycorrhizal.crm.data.local.Converters
import com.mycorrhizal.crm.data.repository.AuthRepositoryImpl
import com.mycorrhizal.crm.data.repository.ContactRepositoryImpl
import com.mycorrhizal.crm.data.session.DefaultSessionManager
import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.data.session.SessionPrefsStorage
import com.mycorrhizal.crm.data.session.TokenStorage
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.BaseUrlProvider
import com.mycorrhizal.crm.network.NetworkFactory
import com.mycorrhizal.crm.network.TokenProvider
import com.squareup.moshi.Moshi
import dagger.Binds
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.android.qualifiers.ApplicationContext
import dagger.hilt.components.SingletonComponent
import javax.inject.Singleton

/**
 * Core dependency graph. Binding at the lowest scope that covers each
 * dependency's lifecycle, per ticket §1.4:
 *  - @Singleton: OkHttp/ApiClient, Room database, session manager
 *  - Repositories are @Singleton here too — they hold no mutable UI state
 *    and are stateless wrappers over Api + Dao (a later phase may narrow to
 *    @ViewModelScoped; the ticket's recommendation is noted there).
 */
@Module
@InstallIn(SingletonComponent::class)
object DataModule {

    @Provides
    @Singleton
    fun provideMoshi(): Moshi = NetworkFactory.moshi()

    @Provides
    @Singleton
    fun provideApiClient(okHttpClient: okhttp3.OkHttpClient, moshi: Moshi): ApiClient =
        ApiClient(okHttpClient, moshi)

    @Provides
    @Singleton
    fun provideDatabase(@ApplicationContext context: android.content.Context): AppDatabase =
        androidx.room.Room.databaseBuilder(
            context,
            AppDatabase::class.java,
            "mycorrhizal-cache.db",
        )
            .fallbackToDestructiveMigration()
            .build()

    @Provides
    fun provideCachedContactDao(db: AppDatabase): CachedContactDao = db.cachedContactDao()

    @Provides
    @Singleton
    fun provideConverters(moshi: Moshi): Converters = Converters(moshi)

    @Provides
    @Singleton
    fun provideSessionManager(
        tokenStorage: TokenStorage,
        prefsStorage: SessionPrefsStorage,
    ): DefaultSessionManager = DefaultSessionManager(tokenStorage, prefsStorage)
}

@Module
@InstallIn(SingletonComponent::class)
abstract class DataBindsModule {
    @Binds
    @Singleton
    abstract fun bindSessionManager(impl: DefaultSessionManager): SessionManager

    @Binds
    @Singleton
    abstract fun bindTokenProvider(impl: SessionManager): TokenProvider

    @Binds
    @Singleton
    abstract fun bindBaseUrlProvider(impl: SessionManager): BaseUrlProvider

    @Binds
    @Singleton
    abstract fun bindAuthRepository(impl: AuthRepositoryImpl): AuthRepository

    @Binds
    @Singleton
    abstract fun bindContactRepository(impl: ContactRepositoryImpl): ContactRepository
}
