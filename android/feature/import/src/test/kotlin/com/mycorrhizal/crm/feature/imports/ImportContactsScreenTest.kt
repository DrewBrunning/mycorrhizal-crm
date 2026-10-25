package com.mycorrhizal.crm.feature.imports

import android.Manifest
import android.app.Application
import android.content.Context
import android.content.Intent
import android.provider.Settings
import androidx.activity.compose.LocalActivityResultRegistryOwner
import androidx.activity.result.ActivityResultRegistry
import androidx.activity.result.ActivityResultRegistryOwner
import androidx.activity.result.contract.ActivityResultContract
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.core.app.ActivityOptionsCompat
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleOwner
import androidx.lifecycle.LifecycleRegistry
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.testing.MainDispatcherRule
import com.mycorrhizal.crm.testing.a11y.assertAccessibleSemantics
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.UnconfinedTestDispatcher
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.Shadows.shadowOf
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issue #214: mounts the real top-level [ImportContactsScreen] (Scaffold +
 * TopAppBar included) against an [ImportContactsViewModel] backed by mocked
 * repositories — the same construction [ImportContactsViewModelTest] uses,
 * with `readDeviceContacts` swapped for canned data (no real ContentResolver
 * query) — and sweeps it for static a11y invariants.
 */
@OptIn(ExperimentalCoroutinesApi::class)
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
        birthday = null
    )

    // The screen's load runs viewModelScope (Dispatchers.Main) and
    // withContext(Dispatchers.IO). Unconfined replaces both so a load finishes
    // before control returns to the test -- otherwise waitForIdle can return
    // after the IO read but before the state update recomposes (issue #1802:
    // the resume-reload test flaked on exactly that gap).
    @get:Rule
    val mainDispatcherRule = MainDispatcherRule(UnconfinedTestDispatcher())

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setScreen(
        darkTheme: Boolean = false,
        grantPermission: Boolean = true,
        onImportVcf: () -> Unit = {},
        onImportCsv: () -> Unit = {},
        readDevice: () -> List<DeviceContact> = { listOf(jane) },
        // null: the real host Activity's registry. Otherwise the system dialog's
        // answer, delivered through a fake registry (granting it in the shadow
        // package manager first when true, as the OS would).
        permissionDialogAnswer: Boolean? = null
    ) {
        if (grantPermission) grantContacts()
        val apiClient = mockk<ApiClient>()
        val contactRepository = mockk<ContactRepository>()
        coEvery { contactRepository.findByEmail(any()) } returns null
        coEvery { contactRepository.findByPhone(any()) } returns null
        val viewModel = ImportContactsViewModel(
            apiClient = apiClient,
            contactRepository = contactRepository,
            appContext = mockk<Context>(relaxed = true)
        ).apply {
            readDeviceContacts = { readDevice() }
            ioDispatcher = mainDispatcherRule.testDispatcher
        }

        val registryOwner = permissionDialogAnswer?.let { answer ->
            object : ActivityResultRegistryOwner {
                override val activityResultRegistry = object : ActivityResultRegistry() {
                    override fun <I, O> onLaunch(
                        requestCode: Int,
                        contract: ActivityResultContract<I, O>,
                        input: I,
                        options: ActivityOptionsCompat?
                    ) {
                        dialogLaunches++
                        if (answer) grantContacts()
                        dispatchResult(requestCode, answer)
                    }
                }
            }
        }

        composeTestRule.setContent {
            MycorrhizalTheme(darkTheme = darkTheme) {
                CompositionLocalProvider(LocalLifecycleOwner provides lifecycleOwner) {
                    val screen = @Composable {
                        ImportContactsScreen(
                            viewModel = viewModel,
                            onImportVcf = onImportVcf,
                            onImportCsv = onImportCsv
                        )
                    }
                    if (registryOwner != null) {
                        CompositionLocalProvider(
                            LocalActivityResultRegistryOwner provides registryOwner
                        ) { screen() }
                    } else {
                        screen()
                    }
                }
            }
        }
    }

    private var dialogLaunches = 0

    /** A lifecycle the test drives, so pause/resume can be simulated. */
    private val lifecycleOwner = object : LifecycleOwner {
        val registry = LifecycleRegistry.createUnsafe(
            this
        ).apply { currentState = Lifecycle.State.RESUMED }
        override val lifecycle: Lifecycle get() = registry
    }

    private fun pauseAndResume() {
        composeTestRule.runOnIdle {
            lifecycleOwner.registry.currentState = Lifecycle.State.STARTED
            lifecycleOwner.registry.currentState = Lifecycle.State.RESUMED
        }
        composeTestRule.waitForIdle()
    }

    private fun grantContacts() {
        shadowOf(ApplicationProvider.getApplicationContext<Application>())
            .grantPermissions(Manifest.permission.READ_CONTACTS)
    }

    private fun setOsRationale(show: Boolean) {
        shadowOf(ApplicationProvider.getApplicationContext<Application>().packageManager)
            .setShouldShowRequestPermissionRationale(Manifest.permission.READ_CONTACTS, show)
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
        setScreen(grantPermission = false, readDevice = {
            reads++
            emptyList()
        })

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

        composeTestRule.onNodeWithText("No device contacts to import").assertIsDisplayed()
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

    @Test
    fun `permission rationale state has no accessibility violations`() {
        setScreen(grantPermission = false)

        composeTestRule.onNodeWithText("Grant access").assertIsDisplayed()
        composeTestRule.assertAccessibleSemantics()
    }

    // Issue #1750 review: the launcher callback, not just the pure helper.
    @Test
    fun `granting from the system dialog loads the device list`() {
        setScreen(grantPermission = false, permissionDialogAnswer = true)

        composeTestRule.onNodeWithText("Grant access").performClick()
        composeTestRule.waitForIdle()

        assertEquals(1, dialogLaunches)
        composeTestRule.onNodeWithText("Jane Smith").assertIsDisplayed()
        composeTestRule.onNodeWithText("Grant access").assertDoesNotExist()
    }

    @Test
    fun `a first denial keeps the rationale with Grant access`() {
        setOsRationale(true)
        setScreen(grantPermission = false, permissionDialogAnswer = false)

        composeTestRule.onNodeWithText("Grant access").performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Grant access").assertIsDisplayed()
        composeTestRule.onNodeWithText("Open settings").assertDoesNotExist()
    }

    @Test
    fun `a permanent denial offers Open settings, which opens this app's settings page`() {
        setOsRationale(false)
        setScreen(grantPermission = false, permissionDialogAnswer = false)

        composeTestRule.onNodeWithText("Grant access").performClick()
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithText("Open settings").performClick()

        val app = ApplicationProvider.getApplicationContext<Application>()
        val intent: Intent = shadowOf(app).nextStartedActivity
        assertEquals(Settings.ACTION_APPLICATION_DETAILS_SETTINGS, intent.action)
        assertEquals("package:${app.packageName}", intent.data.toString())
    }

    @Test
    fun `granting in system settings loads the list on return`() {
        var reads = 0
        setScreen(grantPermission = false, readDevice = {
            reads++
            listOf(jane)
        })
        composeTestRule.onNodeWithText("Grant access").assertIsDisplayed()

        grantContacts()
        pauseAndResume()

        assertEquals(1, reads)
        composeTestRule.onNodeWithText("Jane Smith").assertIsDisplayed()
    }

    // Issue #1750 review: reloading on every resume wiped the selection
    // whenever the app was backgrounded or the screen locked.
    @Test
    fun `a resume after the list loaded keeps the selection and does not re-read`() {
        var reads = 0
        setScreen(readDevice = {
            reads++
            listOf(jane)
        })
        composeTestRule.onNodeWithText("Jane Smith").performClick()
        composeTestRule.onNodeWithText(
            "Import 1 selected (0 possible duplicates)"
        ).assertIsDisplayed()

        pauseAndResume()

        assertEquals(1, reads)
        composeTestRule.onNodeWithText(
            "Import 1 selected (0 possible duplicates)"
        ).assertIsDisplayed()
    }
}
