package com.mycorrhizal.crm.di

import com.mycorrhizal.crm.applock.AppLockLocalAuthPrompt
import com.mycorrhizal.crm.applock.AppUnlockPrompterFactory
import com.mycorrhizal.crm.applock.BiometricAppUnlockPrompter
import com.mycorrhizal.crm.data.auth.LocalAuthPrompt
import dagger.Binds
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent

/** Binds the app-lock CryptoObject prompt for login-screen biometric re-auth. */
@Module
@InstallIn(SingletonComponent::class)
abstract class LocalAuthPromptModule {
    @Binds
    abstract fun bindLocalAuthPrompt(impl: AppLockLocalAuthPrompt): LocalAuthPrompt

    companion object {
        @Provides
        fun provideAppUnlockPrompterFactory(): AppUnlockPrompterFactory =
            AppUnlockPrompterFactory { BiometricAppUnlockPrompter(it) }
    }
}
