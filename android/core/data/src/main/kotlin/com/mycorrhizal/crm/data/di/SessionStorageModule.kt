package com.mycorrhizal.crm.data.di

import android.content.Context
import com.mycorrhizal.crm.data.session.DataStoreSessionPrefsStorage
import com.mycorrhizal.crm.data.session.EncryptedTokenStorage
import com.mycorrhizal.crm.data.session.SessionPrefsStorage
import com.mycorrhizal.crm.data.session.TokenStorage
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.android.qualifiers.ApplicationContext
import dagger.hilt.components.SingletonComponent
import javax.inject.Singleton

@Module
@InstallIn(SingletonComponent::class)
object SessionStorageModule {
    @Provides
    @Singleton
    fun provideTokenStorage(@ApplicationContext context: Context): TokenStorage =
        EncryptedTokenStorage(context)

    @Provides
    @Singleton
    fun provideSessionPrefsStorage(@ApplicationContext context: Context): SessionPrefsStorage =
        DataStoreSessionPrefsStorage(context)
}
