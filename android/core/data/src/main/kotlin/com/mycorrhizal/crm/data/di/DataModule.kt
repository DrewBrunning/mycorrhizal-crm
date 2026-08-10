package com.mycorrhizal.crm.data.di

import com.mycorrhizal.crm.data.local.AppDatabase
import com.mycorrhizal.crm.data.local.CachedActivityDao
import com.mycorrhizal.crm.data.local.CachedCircleDao
import com.mycorrhizal.crm.data.local.CachedCircleMemberDao
import com.mycorrhizal.crm.data.local.CachedContactDao
import com.mycorrhizal.crm.data.local.CachedNoteDao
import com.mycorrhizal.crm.data.local.CachedReminderDao
import com.mycorrhizal.crm.data.repository.ActivityRepositoryImpl
import com.mycorrhizal.crm.data.repository.AuthRepositoryImpl
import com.mycorrhizal.crm.data.repository.CircleRepositoryImpl
import com.mycorrhizal.crm.data.repository.ContactRepositoryImpl
import com.mycorrhizal.crm.data.repository.NoteRepositoryImpl
import com.mycorrhizal.crm.data.repository.ReminderRepositoryImpl
import com.mycorrhizal.crm.data.session.DefaultSessionManager
import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.data.session.SessionPrefsStorage
import com.mycorrhizal.crm.data.session.TokenStorage
import com.mycorrhizal.crm.domain.repository.ActivityRepository
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.CircleRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.NoteRepository
import com.mycorrhizal.crm.domain.repository.ReminderRepository
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
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch
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
    fun provideCachedActivityDao(db: AppDatabase): CachedActivityDao = db.cachedActivityDao()

    @Provides
    fun provideCachedNoteDao(db: AppDatabase): CachedNoteDao = db.cachedNoteDao()

    @Provides
    fun provideCachedReminderDao(db: AppDatabase): CachedReminderDao = db.cachedReminderDao()

    @Provides
    fun provideCachedCircleDao(db: AppDatabase): CachedCircleDao = db.cachedCircleDao()

    @Provides
    fun provideCachedCircleMemberDao(db: AppDatabase): CachedCircleMemberDao =
        db.cachedCircleMemberDao()

    @Provides
    @Singleton
    fun provideSessionManager(
        tokenStorage: TokenStorage,
        prefsStorage: SessionPrefsStorage,
    ): DefaultSessionManager {
        val manager = DefaultSessionManager(tokenStorage, prefsStorage)
        // Hydrate the stored JWT/server URL into memory asynchronously so a
        // returning user is already logged in on launch (H3 review fix).
        CoroutineScope(SupervisorJob() + Dispatchers.IO).launch { manager.init() }
        return manager
    }
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

    @Binds
    @Singleton
    abstract fun bindActivityRepository(impl: ActivityRepositoryImpl): ActivityRepository

    @Binds
    @Singleton
    abstract fun bindNoteRepository(impl: NoteRepositoryImpl): NoteRepository

    @Binds
    @Singleton
    abstract fun bindReminderRepository(impl: ReminderRepositoryImpl): ReminderRepository

    @Binds
    @Singleton
    abstract fun bindCircleRepository(impl: CircleRepositoryImpl): CircleRepository
}
