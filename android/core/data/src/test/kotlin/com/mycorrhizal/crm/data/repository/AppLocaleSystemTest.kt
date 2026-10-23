package com.mycorrhizal.crm.data.repository

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/**
 * Issue #1921: the API 33+ half of [AppLocale] — mirroring the in-app choice to
 * the OS per-app locale and reading an explicit system choice back.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
class AppLocaleSystemTest {

    private val context
        get() = ApplicationProvider.getApplicationContext<Context>()

    @Before
    fun clearLocales() {
        AppLocale.applyToSystem(context, null)
        AppLocale.languageTag = null
    }

    @After
    fun resetLocales() {
        AppLocale.applyToSystem(context, null)
        AppLocale.languageTag = null
    }

    @Test
    fun `the in-app tag is mirrored to and read back from the OS locale`() {
        AppLocale.applyToSystem(context, "de")

        assertEquals("de", AppLocale.systemLanguageTag(context))
    }

    @Test
    fun `a null tag clears the OS per-app locale`() {
        AppLocale.applyToSystem(context, "fr")
        assertEquals("fr", AppLocale.systemLanguageTag(context))

        AppLocale.applyToSystem(context, null)

        assertNull(AppLocale.systemLanguageTag(context))
    }
}
