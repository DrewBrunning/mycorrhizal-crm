package com.mycorrhizal.crm.data.repository

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.domain.repository.AppSettingsRepository
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class AppSettingsRepositoryImplTest {

    private val context = ApplicationProvider.getApplicationContext<Context>()

    private fun repo(): AppSettingsRepositoryImpl = AppSettingsRepositoryImpl(context)

    @Test
    fun `theme defaults to system and persists a change`() = runTest {
        val r = repo()
        assertEquals(AppSettingsRepository.THEME_SYSTEM, r.themePreference().first())

        r.setThemePreference(AppSettingsRepository.THEME_DARK)

        assertEquals(AppSettingsRepository.THEME_DARK, r.themePreference().first())
        assertEquals(AppSettingsRepository.THEME_DARK, r.currentThemePreference())
    }

    @Test
    fun `language override defaults to null and persists a change`() = runTest {
        val r = repo()
        assertNull(r.languageOverride().first())
        assertNull(AppLocale.languageTag)

        r.setLanguageOverride("de")

        assertEquals("de", r.languageOverride().first())
        // The sync cache the Activity's attachBaseContext reads is updated too.
        assertEquals("de", AppLocale.languageTag)
    }

    @Test
    fun `hydrate populates the sync caches from persisted values`() = runTest {
        val r = repo()
        r.setThemePreference(AppSettingsRepository.THEME_LIGHT)
        r.setLanguageOverride("it")
        // Simulate a fresh process by resetting the static cache.
        AppLocale.languageTag = null

        repo().hydrate()

        assertEquals(AppSettingsRepository.THEME_LIGHT, r.currentThemePreference())
        assertEquals("it", AppLocale.languageTag)
    }

    @Test
    fun `clearing the language override removes it`() = runTest {
        val r = repo()
        r.setLanguageOverride("fr")
        assertEquals("fr", AppLocale.languageTag)

        r.setLanguageOverride(null)

        assertNull(r.languageOverride().first())
        assertNull(AppLocale.languageTag)
    }
}
