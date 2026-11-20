package com.mycorrhizal.crm.feature.contacts.screenshot

import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onRoot
import com.github.takahirom.roborazzi.RobolectricDeviceQualifiers
import com.github.takahirom.roborazzi.captureRoboImage
import com.mycorrhizal.crm.feature.contacts.ContactDetailContent
import com.mycorrhizal.crm.model.network.Address
import com.mycorrhizal.crm.model.network.AddressComponent
import com.mycorrhizal.crm.model.network.Anniversary
import com.mycorrhizal.crm.model.network.AnniversaryDate
import com.mycorrhizal.crm.model.network.CRMEnvelope
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.Email
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.model.network.NameComponent
import com.mycorrhizal.crm.model.network.Nickname
import com.mycorrhizal.crm.model.network.OnlineService
import com.mycorrhizal.crm.model.network.OrgUnit
import com.mycorrhizal.crm.model.network.Organization
import com.mycorrhizal.crm.model.network.PartialDate
import com.mycorrhizal.crm.model.network.PersonalInfo
import com.mycorrhizal.crm.model.network.Phone
import com.mycorrhizal.crm.model.network.Title
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
 * Issue #2534: visual-regression goldens for the contact detail page.
 *
 * The stateless [ContactDetailContent] is rendered from a fixed, fully populated
 * [ContactRecordResponse] — no ViewModel, Hilt, network, clock or generated IDs —
 * across light/dark and font scale 1.0/2.0, plus one medium-tablet run. Goldens
 * live in `src/test/screenshots/`; regenerate them with
 * `./gradlew :feature:contacts:recordRoborazziDebug`.
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(sdk = [35], qualifiers = RobolectricDeviceQualifiers.Pixel7)
class ContactDetailScreenshotTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun lightThemeAtDefaultFontScale() = capture("ContactDetail_light_1.0.png", darkTheme = false)

    @Test
    fun darkThemeAtDefaultFontScale() = capture("ContactDetail_dark_1.0.png", darkTheme = true)

    @Test
    fun lightThemeAtLargeFontScale() =
        capture("ContactDetail_light_2.0.png", darkTheme = false, fontScale = 2f)

    @Test
    fun darkThemeAtLargeFontScale() =
        capture("ContactDetail_dark_2.0.png", darkTheme = true, fontScale = 2f)

    @Test
    @Config(qualifiers = RobolectricDeviceQualifiers.MediumTablet)
    fun tabletLightThemeAtDefaultFontScale() =
        capture("ContactDetail_light_1.0_tablet.png", darkTheme = false)

    private fun capture(goldenName: String, darkTheme: Boolean, fontScale: Float = 1f) {
        // Before setContent: Compose reads the font scale when it composes.
        RuntimeEnvironment.setFontScale(fontScale)
        composeTestRule.setContent {
            MycorrhizalTheme(darkTheme = darkTheme) {
                // The screen itself is a plain LazyColumn; in the app the
                // surrounding Scaffold paints the themed background. Paint it
                // here too so a background-token change is captured.
                Surface(
                    modifier = Modifier.fillMaxSize(),
                    color = MaterialTheme.colorScheme.background
                ) {
                    ContactDetailContent(contact = CONTACT, dateFormat = "eu")
                }
            }
        }
        // onRoot() waits for idle, so the capture cannot race a pending layout.
        composeTestRule.onRoot().captureRoboImage(File(OUTPUT_DIR, goldenName))
    }

    private companion object {
        /** Relative to the module directory, which is the test task's working directory. */
        val OUTPUT_DIR = File("src/test/screenshots")

        /** Fixed identity: changing any value here is a deliberate golden update. */
        const val UID = "urn:uuid:25340000-0000-4000-8000-000000000001"

        val CONTACT = ContactRecordResponse(
            id = 7,
            uid = UID,
            revision = 3,
            card = Card(
                uid = UID,
                kind = "individual",
                name = Name(
                    components = listOf(
                        NameComponent(kind = "title", value = "Dr"),
                        NameComponent(kind = "given", value = "Ada"),
                        NameComponent(kind = "surname", value = "Lovelace")
                    ),
                    full = "Dr Ada Lovelace",
                    isOrdered = true
                ),
                nicknames = listOf(
                    Nickname(id = "n1", name = "The Enchantress", pref = 1)
                ),
                organizations = listOf(
                    Organization(
                        id = "o1",
                        name = "Analytical Engines Ltd",
                        units = listOf(OrgUnit(name = "Mathematics"))
                    )
                ),
                titles = listOf(
                    Title(
                        id = "t1",
                        name = "Chief Mathematician",
                        kind = "title",
                        organizationId = "o1"
                    )
                ),
                emails = listOf(
                    Email(
                        id = "e1",
                        address = "ada@example.com",
                        contexts = listOf("work"),
                        pref = 1
                    ),
                    Email(
                        id = "e2",
                        address = "ada.lovelace@example.org",
                        contexts = listOf("private")
                    )
                ),
                phones = listOf(
                    Phone(
                        id = "p1",
                        number = "+44 20 7946 0001",
                        features = listOf("voice"),
                        contexts = listOf("work"),
                        pref = 1
                    ),
                    Phone(
                        id = "p2",
                        number = "+44 20 7946 0002",
                        contexts = listOf("private")
                    )
                ),
                imppAddresses = listOf(
                    OnlineService(
                        id = "i1",
                        service = "matrix",
                        uri = "matrix:@ada:example.org",
                        contexts = listOf("work")
                    )
                ),
                addresses = listOf(
                    Address(
                        id = "a1",
                        full = "12 St James's Square\nLondon SW1Y 4LB\nUnited Kingdom",
                        components = listOf(
                            AddressComponent(kind = "name", value = "12 St James's Square"),
                            AddressComponent(kind = "locality", value = "London"),
                            AddressComponent(kind = "postcode", value = "SW1Y 4LB"),
                            AddressComponent(kind = "country", value = "United Kingdom")
                        ),
                        countryCode = "GB",
                        contexts = listOf("work")
                    )
                ),
                anniversaries = listOf(
                    Anniversary(
                        id = "ann1",
                        kind = "birthday",
                        date = AnniversaryDate(
                            partial = PartialDate(year = 1815, month = 12, day = 10)
                        )
                    )
                ),
                personalInfo = listOf(
                    PersonalInfo(
                        id = "pi1",
                        kind = "expertise",
                        value = "Mathematics",
                        level = "expert"
                    ),
                    PersonalInfo(id = "pi2", kind = "interest", value = "Poetry")
                ),
                keywords = listOf("mathematics", "computing")
            ),
            crm = CRMEnvelope(
                kind = "human",
                circles = listOf("Friends", "Work"),
                howWeMet = "Introduced at a lecture on difference engines",
                workInformation = "Works on the Analytical Engine",
                contactInformation = "Prefers email"
            ),
            isFavorite = true
        )
    }
}
