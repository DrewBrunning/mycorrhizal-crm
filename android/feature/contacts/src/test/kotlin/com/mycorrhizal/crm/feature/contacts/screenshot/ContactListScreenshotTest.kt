package com.mycorrhizal.crm.feature.contacts.screenshot

import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onRoot
import com.github.takahirom.roborazzi.RobolectricDeviceQualifiers
import com.github.takahirom.roborazzi.captureRoboImage
import com.mycorrhizal.crm.feature.contacts.ContactListScreenContent
import com.mycorrhizal.crm.feature.contacts.ContactListUiState
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import java.io.File
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issue #2534: visual-regression goldens for the contact list.
 *
 * The stateless `ContactListScreenContent` is rendered with a hand-built,
 * deterministic [ContactListUiState] — no ViewModel, no Hilt, no network, no
 * clock, no generated IDs — so a diff can only come from a layout, theme or
 * font-scale change. The matrix is light/dark x font scale 1.0/2.0 on a Pixel 7,
 * plus one medium-tablet run. Goldens live in `src/test/screenshots/`
 * (`roborazzi { outputDir ... }` in build.gradle.kts); regenerate them with
 * `./gradlew :feature:contacts:recordRoborazziDebug`.
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(sdk = [35], qualifiers = RobolectricDeviceQualifiers.Pixel7)
class ContactListScreenshotTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun lightThemeAtDefaultFontScale() = capture("ContactList_light_1.0.png", darkTheme = false)

    @Test
    fun darkThemeAtDefaultFontScale() = capture("ContactList_dark_1.0.png", darkTheme = true)

    @Test
    fun lightThemeAtLargeFontScale() =
        capture("ContactList_light_2.0.png", darkTheme = false, fontScale = 2f)

    @Test
    fun darkThemeAtLargeFontScale() =
        capture("ContactList_dark_2.0.png", darkTheme = true, fontScale = 2f)

    @Test
    @Config(qualifiers = RobolectricDeviceQualifiers.MediumTablet)
    fun tabletLightThemeAtDefaultFontScale() =
        capture("ContactList_light_1.0_tablet.png", darkTheme = false)

    @Test
    fun emptyState() =
        capture("ContactList_empty_light_1.0.png", darkTheme = false, contacts = emptyList())

    private fun capture(
        goldenName: String,
        darkTheme: Boolean,
        fontScale: Float = 1f,
        contacts: List<ContactSummary> = SAMPLE_CONTACTS
    ) {
        // Before setContent: Compose reads the font scale when it composes.
        RuntimeEnvironment.setFontScale(fontScale)
        composeTestRule.setContent {
            MycorrhizalTheme(darkTheme = darkTheme) {
                ContactListScreenContent(
                    uiState = ContactListUiState(contacts = contacts),
                    onSearchQueryChange = {},
                    onContactClick = {}
                )
            }
        }
        // onRoot() waits for idle, so the capture cannot race a pending layout.
        composeTestRule.onRoot().captureRoboImage(File(OUTPUT_DIR, goldenName))
    }

    private companion object {
        /** Relative to the module directory, which is the test task's working directory. */
        val OUTPUT_DIR = File("src/test/screenshots")

        val SAMPLE_CONTACTS = listOf(
            ContactSummary(
                id = 1,
                firstname = "Ada",
                lastname = "Lovelace",
                fn = "Ada Lovelace",
                primaryEmail = "ada@example.com",
                primaryPhone = "+44 20 7946 0001",
                org = "Analytical Engines",
                isFavorite = true
            ),
            ContactSummary(
                id = 2,
                firstname = "Grace",
                lastname = "Hopper",
                fn = "Grace Hopper",
                primaryEmail = "grace@example.com",
                primaryPhone = "+1 202 555 0102",
                org = "US Navy"
            ),
            ContactSummary(
                id = 3,
                firstname = "Alan",
                lastname = "Turing",
                fn = "Alan Turing",
                primaryEmail = "alan@example.com",
                primaryPhone = "+44 20 7946 0003",
                org = "Bletchley Park"
            ),
            ContactSummary(
                id = 4,
                firstname = "Katherine",
                lastname = "Johnson",
                fn = "Katherine Johnson",
                primaryEmail = "katherine@example.com",
                primaryPhone = "+1 202 555 0104",
                org = "NASA"
            ),
            ContactSummary(
                id = 5,
                firstname = "Margaret",
                lastname = "Hamilton",
                fn = "Margaret Hamilton",
                primaryEmail = "margaret@example.com",
                primaryPhone = "+1 202 555 0105",
                org = "MIT"
            )
        )
    }
}
