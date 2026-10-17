package com.mycorrhizal.crm.feature.imports

import android.Manifest
import android.app.Application
import android.content.Context
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.test.core.app.ApplicationProvider
import org.robolectric.Shadows.shadowOf
import org.junit.Assert.assertEquals
import androidx.compose.ui.test.junit4.createComposeRule
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.testing.a11y.assertAccessibleSemantics
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.mockk
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issue #214: mounts the real top-level [ImportContactsScreen] (Scaffold +
 * TopAppBar included) against an [ImportContactsViewModel] backed by mocked
 * repositories — the same construction [ImportContactsViewModelTest] uses,
 * with `readDeviceContacts` swapped for canned data (no real ContentResolver
 * query) — and sweeps it for static a11y invariants.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ImportContactsScreenTest {

    private val jane = DeviceContact(
        contactId = 1,
        lookupKey = "lk-1",
        displayName = "Jane Smith",
        phones = listOf("+15559998888" to 1),
        emails = listOf("jane@example.com"),
        addresses = emptyList(),
        organization = null,
        birthday = null,
    )

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setScreen(
        darkTheme: Boolean = false,
        grantPermission: Boolean = true,
        onImportVcf: () -> Unit = {},
        onImportCsv: () -> Unit = {},
        readDevice: () -> List<DeviceContact> = { listOf(jane) },
    ) {
        if (grantPermission) {
            shadowOf(ApplicationProvider.getApplicationContext<Application>())
                .grantPermissions(Manifest.permission.READ_CONTACTS)
        }
        val apiClient = mockk<ApiClient>()
        val contactRepository = mockk<ContactRepository>()
        coEvery { contactRepository.findByEmail(any()) } returns null
        coEvery { contactRepository.findByPhone(any()) } returns null
        val viewModel = ImportContactsViewModel(
            apiClient = apiClient,
            contactRepository = contactRepository,
            appContext = mockk<Context>(relaxed = true),
        ).apply {
            readDeviceContacts = { readDevice() }
        }

        composeTestRule.setContent {
            MycorrhizalTheme(darkTheme = darkTheme) {
                ImportContactsScreen(viewModel = viewModel, onImportVcf = onImportVcf, onImportCsv = onImportCsv)
            }
        }
    }

    @Test
    fun `import contacts screen has no accessibility violations (light)`() {
        setScreen(darkTheme = false)

        composeTestRule.assertAccessibleSemantics()
    }

    @Test
    fun `import contacts screen has no accessibility violations (dark)`() {
        setScreen(darkTheme = true)

        composeTestRule.assertAccessibleSemantics()
    }

    // Issue #1750: without READ_CONTACTS the screen shows the rationale and never reads.
    @Test
    fun `without the contacts permission the rationale shows and the device is not read`() {
        var reads = 0
        setScreen(grantPermission = false, readDevice = { reads++; emptyList() })

        composeTestRule.onNodeWithText("Grant access").assertIsDisplayed()
        assertEquals(0, reads)
    }

    @Test
    fun `file imports stay reachable without the contacts permission`() {
        var vcf = 0
        var csv = 0
        setScreen(grantPermission = false, onImportVcf = { vcf++ }, onImportCsv = { csv++ })

        composeTestRule.onNodeWithText("Import from a VCF file").performClick()
        composeTestRule.onNodeWithText("Import from a CSV file").performClick()
        assertEquals(1, vcf)
        assertEquals(1, csv)
    }

    @Test
    fun `a SecurityException from the read shows the rationale instead of crashing`() {
        setScreen(readDevice = { throw SecurityException("denied") })

        composeTestRule.onNodeWithText("Grant access").assertIsDisplayed()
    }

    @Test
    fun `file imports stay reachable with the permission but no device contacts`() {
        setScreen(readDevice = { emptyList() })

        composeTestRule.onNodeWithText("No device contacts to import.").assertIsDisplayed()
        composeTestRule.onNodeWithText("Import from a VCF file").assertIsDisplayed()
    }

    @Test
    fun `with the permission the device list shows and the import button is a plain disabled Import`() {
        setScreen()

        composeTestRule.onNodeWithText("Jane Smith").assertIsDisplayed()
        composeTestRule.onNodeWithText("Import").assertIsNotEnabled()
    }

    @Test
    fun `permanent denial is when the OS offers no rationale`() {
        org.junit.Assert.assertTrue(isPermanentlyDenied(shouldShowRationale = false))
        org.junit.Assert.assertFalse(isPermanentlyDenied(shouldShowRationale = true))
    }
}
