package com.mycorrhizal.crm.feature.settings

import android.content.Context
import androidx.annotation.StringRes
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.domain.repository.ExportRepository
import com.mycorrhizal.crm.testing.a11y.assertAccessibleSemantics
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issue #835 (T9 selective-export Android parity) — the "Custom export"
 * screen: format choice, section checkboxes with the sensitivity lock (the
 * same picker ShareContactScreen already covers), the reveal step, and the
 * export button gating/dispatch.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class CustomExportScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun str(@StringRes res: Int, vararg args: Any): String =
        ApplicationProvider.getApplicationContext<Context>().getString(res, *args)

    private val nonSensitiveDefaultLabels = listOf(
        R.string.shares_section_emails,
        R.string.shares_section_phones,
        R.string.shares_section_addresses,
        R.string.shares_section_organizations,
        R.string.shares_section_anniversaries,
        R.string.shares_section_media,
        R.string.shares_section_online_services,
        R.string.shares_section_links,
        R.string.shares_section_notes,
        R.string.shares_section_keywords,
        R.string.shares_section_speak_to_as,
        R.string.shares_section_members,
        R.string.shares_section_languages,
    )

    private fun screen(repo: ExportRepository, vm: CustomExportViewModel = CustomExportViewModel(repo)) {
        composeTestRule.setContent {
            MycorrhizalTheme { CustomExportScreen(onBack = {}, viewModel = vm) }
        }
    }

    @Test
    fun `renders the format options, fields label, and export button`() {
        val repo = mockk<ExportRepository>()
        screen(repo)

        composeTestRule.onNodeWithText(str(R.string.data_custom_export_title)).assertIsDisplayed()
        composeTestRule.onNodeWithText(str(R.string.data_custom_export_format_vcf4)).assertIsDisplayed()
        composeTestRule.onNodeWithText(str(R.string.data_custom_export_format_vcf3)).performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText(str(R.string.data_custom_export_format_jscontact)).performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText(str(R.string.shares_fields_label)).performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText(str(R.string.data_custom_export_button)).performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `export is enabled by default and disabled once every section is unchecked`() {
        val repo = mockk<ExportRepository>()
        screen(repo)

        composeTestRule.onNodeWithText(str(R.string.data_custom_export_button))
            .performScrollTo()
            .assertIsEnabled()

        nonSensitiveDefaultLabels.forEach { labelRes ->
            composeTestRule.onNodeWithText(str(labelRes)).performScrollTo().performClick()
        }

        composeTestRule.onNodeWithText(str(R.string.data_custom_export_button))
            .performScrollTo()
            .assertIsNotEnabled()
    }

    @Test
    fun `sensitive sections start locked until revealed`() {
        val repo = mockk<ExportRepository>()
        screen(repo)

        composeTestRule.onNodeWithText(str(R.string.shares_section_related_to))
            .performScrollTo()
            .assertIsNotEnabled()
    }

    @Test
    fun `the reveal button opens the sensitive confirmation dialog and unlocks on confirm`() {
        val repo = mockk<ExportRepository>()
        screen(repo)

        composeTestRule.onNodeWithText(str(R.string.shares_reveal_button)).performScrollTo().performClick()
        composeTestRule.onNodeWithText(str(R.string.shares_reveal_title)).assertIsDisplayed()

        composeTestRule.onNodeWithText(str(R.string.shares_reveal_confirm_button)).performClick()

        composeTestRule.onNodeWithText(str(R.string.shares_section_related_to))
            .performScrollTo()
            .assertIsEnabled()
    }

    @Test
    fun `tapping export calls the repository for the selected format`() {
        val repo = mockk<ExportRepository>()
        coEvery { repo.exportContactsVcf(any(), any(), any()) } returns Result.success("vcf".toByteArray())
        screen(repo)

        composeTestRule.onNodeWithText(str(R.string.data_custom_export_button))
            .performScrollTo()
            .performClick()
        composeTestRule.waitForIdle()

        coVerify { repo.exportContactsVcf(null, any(), includeSensitive = false) }
    }

    @Test
    fun `custom export screen has no accessibility violations`() {
        val repo = mockk<ExportRepository>()
        screen(repo)

        composeTestRule.assertAccessibleSemantics()
    }
}
