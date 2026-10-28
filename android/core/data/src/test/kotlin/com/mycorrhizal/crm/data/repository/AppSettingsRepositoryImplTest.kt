package com.mycorrhizal.crm.data.repository

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.domain.repository.AppSettingsRepository
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.runTest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Before
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

    @Before
    fun resetStaticLocale() {
        AppLocale.languageTag = null
    }

    @After
    fun clearStaticLocale() {
        AppLocale.languageTag = null
    }

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
    fun `contact sort defaults to name and persists a change`() = runTest {
        val r = repo()
        assertEquals(AppSettingsRepository.CONTACT_SORT_NAME, r.contactSortPreference().first())

        r.setContactSortPreference(AppSettingsRepository.CONTACT_SORT_RECENTLY_ADDED)

        assertEquals(
            AppSettingsRepository.CONTACT_SORT_RECENTLY_ADDED,
            r.contactSortPreference().first()
        )
    }

    @Test
    fun `hydrate populates the sync caches from persisted values`() = runTest {
        val writer = repo()
        writer.setThemePreference(AppSettingsRepository.THEME_LIGHT)
        writer.setLanguageOverride("it")
        // Simulate a fresh process: a new instance starts with default caches and
        // the static locale cache is cleared.
        AppLocale.languageTag = null
        val freshProcess = repo()
        assertEquals(AppSettingsRepository.THEME_SYSTEM, freshProcess.currentThemePreference())

        freshProcess.hydrate()

        assertEquals(AppSettingsRepository.THEME_LIGHT, freshProcess.currentThemePreference())
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
