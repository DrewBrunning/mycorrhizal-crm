package com.mycorrhizal.crm.feature.settings

import android.content.Context
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
class LicensesScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun `loadBundledNotices reads present assets and skips blank or missing ones`() {
        val context = ApplicationProvider.getApplicationContext<Context>()

        val notices = loadBundledNotices(
            context,
            listOf(
                "bundled-notices/present.txt" to "Present notice",
                "bundled-notices/blank.txt" to "Blank notice",
                "bundled-notices/missing.txt" to "Missing notice"
            )
        )

        assertEquals(1, notices.size)
        assertEquals("Present notice", notices.single().title)
        assertEquals("Bundled notice body.", notices.single().content.trim())
    }

    @Test
    fun `the licenses screen falls back to the empty state without metadata or assets`() {
        composeTestRule.setContent {
            MycorrhizalTheme { ThirdPartyLicensesScreen(onBack = {}) }
        }

        composeTestRule.onNodeWithText("Third-party licenses").assertExists()
        composeTestRule.onNodeWithTag("licenses-empty").assertExists()
    }

    @Test
    fun `bundled notices render even without Gradle library metadata`() {
        val notices = listOf(
            BundledNotice("Go server and standard library", "Go standard library BSD-3-Clause")
        )

        composeTestRule.setContent {
            MycorrhizalTheme {
                ThirdPartyLicensesContent(libs = null, bundledNotices = notices)
            }
        }

        composeTestRule.onNodeWithText("Go server and standard library").assertExists()
        composeTestRule.onNodeWithText(
            "No license information is available in this build."
        ).assertDoesNotExist()
    }
}
