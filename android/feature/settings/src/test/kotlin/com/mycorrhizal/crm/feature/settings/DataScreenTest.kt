package com.mycorrhizal.crm.feature.settings

import androidx.compose.ui.test.assertHasClickAction
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import com.mycorrhizal.crm.domain.backup.BundleBackupStatus
import com.mycorrhizal.crm.domain.repository.BundleBackupRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.ExportRepository
import com.mycorrhizal.crm.domain.repository.RelationshipEdgeRepository
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import io.mockk.every
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

// The DataScreen composable had no UI test — DataViewModelTest covers the
// view-model only. This pins the "Suggest addresses" primary action (a filled
// Button, matching the sibling "Suggest relationships") and its wiring to the
// address scan, plus the full-dataset export rows (issue #710). The screen's
// body is a verticalScroll Column (like SettingsScreen), so every row is
// always composed and performScrollTo() reliably brings it into view.
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class DataScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val contactRepository = mockk<ContactRepository>()
    private val relationshipEdgeRepository = mockk<RelationshipEdgeRepository>()
    private val exportRepository = mockk<ExportRepository>()
    private val backupStatus = kotlinx.coroutines.flow.MutableStateFlow(BundleBackupStatus())
    private val bundleBackupRepository = mockk<BundleBackupRepository> {
        every { observeStatus() } returns backupStatus
    }

    private fun setScreen(onCustomExport: () -> Unit = {}, onRestoreBundle: () -> Unit = {}) {
        val viewModel = DataViewModel(contactRepository, relationshipEdgeRepository, exportRepository, bundleBackupRepository)
        composeTestRule.setContent {
            MycorrhizalTheme {
                DataScreen(
                    onBack = {},
                    onCustomExport = onCustomExport,
                    onRestoreBundle = onRestoreBundle,
                    viewModel = viewModel,
                )
            }
        }
    }

    @Test
    fun `suggest addresses is a displayed, clickable action`() {
        setScreen()

        composeTestRule.onNodeWithText("Suggest addresses")
            .performScrollTo()
            .assertIsDisplayed()
            .assertHasClickAction()
    }

    @Test
    fun `tapping suggest addresses runs the address scan`() {
        coEvery { contactRepository.suggestContactAddresses() } returns Result.success(emptyList())
        setScreen()

        composeTestRule.onNodeWithText("Suggest addresses").performScrollTo().performClick()
        composeTestRule.waitForIdle()

        coVerify { contactRepository.suggestContactAddresses() }
    }

    @Test
    fun `the export section offers every dataset format`() {
        setScreen()

        composeTestRule.onNodeWithText("Export CSV backup").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Export vCard 4.0").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Export vCard 3.0").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Export JSContact").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Export audit log").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `tapping export csv fetches the full csv backup`() {
        coEvery { exportRepository.exportDataCsv() } returns Result.success("csv".toByteArray())
        setScreen()

        composeTestRule.onNodeWithText("Export CSV backup").performScrollTo().performClick()
        composeTestRule.waitForIdle()

        coVerify { exportRepository.exportDataCsv() }
    }

    @Test
    fun `tapping custom export navigates to the field picker`() {
        var navigated = false
        setScreen(onCustomExport = { navigated = true })

        composeTestRule.onNodeWithText("Custom export…").performScrollTo().performClick()

        assertTrue(navigated)
    }

    // --- Issue #1264: account bundle export + restore entry points ---------------

    @Test
    fun `the account bundle export row is offered`() {
        setScreen()

        composeTestRule.onNodeWithText("Account bundle (full backup)").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `restore from bundle is hidden on a remote profile`() {
        setScreen()

        composeTestRule.onNodeWithText("Restore from bundle").assertDoesNotExist()
    }

    @Test
    fun `restore from bundle is offered on an empty local profile and navigates`() {
        coEvery { contactRepository.listContacts(limit = 1) } returns
            Result.success(com.mycorrhizal.crm.domain.repository.ContactsPage(emptyList(), null, 1, null))
        backupStatus.value = BundleBackupStatus(isLocalProfile = true)
        var navigated = false
        setScreen(onRestoreBundle = { navigated = true })
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Restore from bundle").performScrollTo().assertIsDisplayed().performClick()

        assertTrue(navigated)
    }

    @Test
    fun `restore from bundle is hidden on a local profile that already has contacts`() {
        coEvery { contactRepository.listContacts(limit = 1) } returns Result.success(
            com.mycorrhizal.crm.domain.repository.ContactsPage(
                listOf(com.mycorrhizal.crm.model.network.ContactSummary(id = 1)), null, 1, null,
            ),
        )
        backupStatus.value = BundleBackupStatus(isLocalProfile = true)
        setScreen()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Restore from bundle").assertDoesNotExist()
    }
}
