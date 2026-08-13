package com.mycorrhizal.crm.data.di

import com.mycorrhizal.crm.data.local.AppDatabase
import com.mycorrhizal.crm.data.local.CachedActivityDao
import com.mycorrhizal.crm.data.local.CachedCircleDao
import com.mycorrhizal.crm.data.local.CachedCircleMemberDao
import com.mycorrhizal.crm.data.local.CachedContactDao
import com.mycorrhizal.crm.data.local.CustomLinkActionDao
import com.mycorrhizal.crm.data.local.CachedContactTagDao
import com.mycorrhizal.crm.data.local.CachedConversationAgendaDao
import com.mycorrhizal.crm.data.local.CachedGiftDao
import com.mycorrhizal.crm.data.local.CachedHouseholdDao
import com.mycorrhizal.crm.data.local.CachedHouseholdMemberDao
import com.mycorrhizal.crm.data.local.CachedLifeEventDao
import com.mycorrhizal.crm.data.local.CachedNoteDao
import com.mycorrhizal.crm.data.local.CachedPreferenceDao
import com.mycorrhizal.crm.data.local.CachedRelationshipEdgeDao
import com.mycorrhizal.crm.data.local.CachedReminderDao
import com.mycorrhizal.crm.data.local.CachedTagDao
import com.mycorrhizal.crm.data.local.PendingInteractionDao
import com.mycorrhizal.crm.data.repository.ActivityRepositoryImpl
import com.mycorrhizal.crm.data.repository.AuthRepositoryImpl
import com.mycorrhizal.crm.data.repository.CircleRepositoryImpl
import com.mycorrhizal.crm.data.repository.CustomLinkActionRepositoryImpl
import com.mycorrhizal.crm.data.repository.ContactRepositoryImpl
import com.mycorrhizal.crm.data.repository.ConversationAgendaRepositoryImpl
import com.mycorrhizal.crm.data.repository.GiftRepositoryImpl
import com.mycorrhizal.crm.data.repository.HouseholdRepositoryImpl
import com.mycorrhizal.crm.data.repository.LifeEventRepositoryImpl
import com.mycorrhizal.crm.data.repository.MergeRepositoryImpl
import com.mycorrhizal.crm.data.repository.NoteRepositoryImpl
import com.mycorrhizal.crm.data.repository.PreferenceRepositoryImpl
import com.mycorrhizal.crm.data.repository.BulkOperationRepositoryImpl
import com.mycorrhizal.crm.data.repository.PendingInteractionRepositoryImpl
import com.mycorrhizal.crm.data.repository.ReminderRepositoryImpl
import com.mycorrhizal.crm.data.repository.RelationshipEdgeRepositoryImpl
import com.mycorrhizal.crm.data.repository.TagRepositoryImpl
import com.mycorrhizal.crm.data.repository.TrackingSettingsRepositoryImpl
import com.mycorrhizal.crm.data.session.DefaultSessionManager
import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.data.session.SessionPrefsStorage
import com.mycorrhizal.crm.data.session.TokenStorage
import com.mycorrhizal.crm.domain.repository.ActivityRepository
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.BulkOperationRepository
import com.mycorrhizal.crm.domain.repository.CircleRepository
import com.mycorrhizal.crm.domain.repository.CustomLinkActionRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.ConversationAgendaRepository
import com.mycorrhizal.crm.domain.repository.GiftRepository
import com.mycorrhizal.crm.domain.repository.HouseholdRepository
import com.mycorrhizal.crm.domain.repository.LifeEventRepository
import com.mycorrhizal.crm.domain.repository.MergeRepository
import com.mycorrhizal.crm.domain.repository.NoteRepository
import com.mycorrhizal.crm.domain.repository.PendingInteractionRepository
import com.mycorrhizal.crm.domain.repository.PreferenceRepository
import com.mycorrhizal.crm.domain.repository.ReminderRepository
import com.mycorrhizal.crm.domain.repository.RelationshipEdgeRepository
import com.mycorrhizal.crm.domain.repository.TagRepository
import com.mycorrhizal.crm.domain.repository.TrackingSettingsRepository
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
            // T76: an explicit migration for 13->14 so pending_interactions (a real
            // not-yet-synced outbox) survives that specific version bump; the destructive
            // fallback remains for any other/unexpected version gap, per this cache's
            // general rebuild-from-server policy (see AppDatabase's doc comment).
            .addMigrations(com.mycorrhizal.crm.data.local.MIGRATION_13_14)
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
    fun provideCachedTagDao(db: AppDatabase): CachedTagDao = db.cachedTagDao()

    @Provides
    fun provideCachedContactTagDao(db: AppDatabase): CachedContactTagDao =
        db.cachedContactTagDao()

    @Provides
    fun provideCachedHouseholdDao(db: AppDatabase): CachedHouseholdDao =
        db.cachedHouseholdDao()

    @Provides
    fun provideCachedHouseholdMemberDao(db: AppDatabase): CachedHouseholdMemberDao =
        db.cachedHouseholdMemberDao()

    @Provides
    fun provideCachedRelationshipEdgeDao(db: AppDatabase): CachedRelationshipEdgeDao =
        db.cachedRelationshipEdgeDao()

    @Provides
    fun provideCachedLifeEventDao(db: AppDatabase): CachedLifeEventDao = db.cachedLifeEventDao()

    @Provides
    fun provideCachedGiftDao(db: AppDatabase): CachedGiftDao = db.cachedGiftDao()

    @Provides
    fun provideCachedPreferenceDao(db: AppDatabase): CachedPreferenceDao = db.cachedPreferenceDao()

    @Provides
    fun provideCachedConversationAgendaDao(db: AppDatabase): CachedConversationAgendaDao =
        db.cachedConversationAgendaDao()

    @Provides
    fun providePendingInteractionDao(db: AppDatabase): PendingInteractionDao =
        db.pendingInteractionDao()

    @Provides
    fun provideCustomLinkActionDao(db: AppDatabase): CustomLinkActionDao =
        db.customLinkActionDao()

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

    @Binds
    @Singleton
    abstract fun bindTagRepository(impl: TagRepositoryImpl): TagRepository

    @Binds
    @Singleton
    abstract fun bindHouseholdRepository(impl: HouseholdRepositoryImpl): HouseholdRepository

    @Binds
    @Singleton
    abstract fun bindRelationshipEdgeRepository(impl: RelationshipEdgeRepositoryImpl): RelationshipEdgeRepository

    @Binds
    @Singleton
    abstract fun bindLifeEventRepository(impl: LifeEventRepositoryImpl): LifeEventRepository

    @Binds
    @Singleton
    abstract fun bindGiftRepository(impl: GiftRepositoryImpl): GiftRepository

    @Binds
    @Singleton
    abstract fun bindPreferenceRepository(impl: PreferenceRepositoryImpl): PreferenceRepository

    @Binds
    @Singleton
    abstract fun bindConversationAgendaRepository(impl: ConversationAgendaRepositoryImpl): ConversationAgendaRepository

    @Binds
    @Singleton
    abstract fun bindMergeRepository(impl: MergeRepositoryImpl): MergeRepository

    @Binds
    @Singleton
    abstract fun bindBulkOperationRepository(impl: BulkOperationRepositoryImpl): BulkOperationRepository

    @Binds
    @Singleton
    abstract fun bindPendingInteractionRepository(impl: PendingInteractionRepositoryImpl): PendingInteractionRepository

    @Binds
    @Singleton
    abstract fun bindCustomLinkActionRepository(impl: CustomLinkActionRepositoryImpl): CustomLinkActionRepository

    @Binds
    @Singleton
    abstract fun bindTrackingSettingsRepository(impl: TrackingSettingsRepositoryImpl): TrackingSettingsRepository
}
