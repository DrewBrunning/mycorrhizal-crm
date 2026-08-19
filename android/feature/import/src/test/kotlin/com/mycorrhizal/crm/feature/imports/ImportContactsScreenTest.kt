package com.mycorrhizal.crm.feature.imports

import android.content.Context
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

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setScreen(darkTheme: Boolean) {
        val apiClient = mockk<ApiClient>()
        val contactRepository = mockk<ContactRepository>()
        coEvery { contactRepository.findByEmail(any()) } returns null
        coEvery { contactRepository.findByPhone(any()) } returns null
        val viewModel = ImportContactsViewModel(
            apiClient = apiClient,
            contactRepository = contactRepository,
            appContext = mockk<Context>(relaxed = true),
        ).apply {
            readDeviceContacts = {
                listOf(
                    DeviceContact(
                        contactId = 1,
                        lookupKey = "lk-1",
                        displayName = "Jane Smith",
                        phones = listOf("+15559998888" to 1),
                        emails = listOf("jane@example.com"),
                        addresses = emptyList(),
                        organization = null,
                        birthday = null,
                    ),
                )
            }
        }

        composeTestRule.setContent {
            MycorrhizalTheme(darkTheme = darkTheme) {
                ImportContactsScreen(viewModel = viewModel)
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
}
