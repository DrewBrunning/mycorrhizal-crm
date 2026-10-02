package com.mycorrhizal.crm.di

import com.mycorrhizal.crm.BuildConfig
import com.mycorrhizal.crm.domain.about.AppBuildInfo
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent

/** Issue #1420: exposes `:app`'s BuildConfig to the Settings About section. */
@Module
@InstallIn(SingletonComponent::class)
object AppBuildInfoModule {
    @Provides
    fun provideAppBuildInfo(): AppBuildInfo = appBuildInfo(
        versionName = BuildConfig.VERSION_NAME,
        versionCode = BuildConfig.VERSION_CODE,
        buildType = BuildConfig.BUILD_TYPE,
        flavor = BuildConfig.FLAVOR,
        applicationId = BuildConfig.APPLICATION_ID,
        commit = BuildConfig.GIT_COMMIT,
        buildDate = BuildConfig.BUILD_DATE,
    )
}

/** Blank commit / build date (an unstamped local build) become null. */
internal fun appBuildInfo(
    versionName: String,
    versionCode: Int,
    buildType: String,
    flavor: String,
    applicationId: String,
    commit: String,
    buildDate: String,
) = AppBuildInfo(
    versionName = versionName,
    versionCode = versionCode,
    buildType = buildType,
    flavor = flavor,
    applicationId = applicationId,
    commit = commit.takeIf { it.isNotBlank() },
    buildDate = buildDate.takeIf { it.isNotBlank() },
)
