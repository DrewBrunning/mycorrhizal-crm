package com.mycorrhizal.crm.screenshot

import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import com.github.takahirom.roborazzi.RobolectricDeviceQualifiers
import com.mycorrhizal.crm.domain.about.AppBuildInfo
import com.mycorrhizal.crm.feature.auth.LoginMode
import com.mycorrhizal.crm.feature.auth.LoginScreenContent
import com.mycorrhizal.crm.feature.auth.LoginUiState
import com.mycorrhizal.crm.feature.contacts.ContactDetailContent
import com.mycorrhizal.crm.feature.contacts.ContactListScreenContent
import com.mycorrhizal.crm.feature.contacts.ContactListUiState
import com.mycorrhizal.crm.feature.settings.SettingsRootContent
import com.mycorrhizal.crm.feature.settings.SettingsUiState
import com.mycorrhizal.crm.model.network.Address
import com.mycorrhizal.crm.model.network.AddressComponent
import com.mycorrhizal.crm.model.network.Anniversary
import com.mycorrhizal.crm.model.network.AnniversaryDate
import com.mycorrhizal.crm.model.network.CRMEnvelope
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.ContactSummary
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

/**
 * `en-rXA` (the resource-qualifier spelling of `en-XA`) on the same Pixel 7
 * profile as the base goldens, so the pseudo and base PNGs differ only by the
 * locale expansion/markers, not by screen density or size.
 *
 * Robolectric resolves this against the app's merged resources; it only exists
 * because `:app`'s debug build sets `isPseudoLocalesEnabled = true`.
 */
internal const val PSEUDO_LOCALE_QUALIFIERS: String =
    "en-rXA-" + RobolectricDeviceQualifiers.Pixel7

/**
 * Issue #2535: fixtures shared by the pseudo-locale screenshot goldens and the
 * hardcoded-string sweep.
 *
 * The values are deliberately the SAME hand-built state the module-local base
 * goldens use (issue #2534, `:feature:{auth,contacts,settings}`), so a human can
 * compare `ContactList_pseudo.png` against `ContactList_light_1.0.png`. Nothing
 * here may come from a ViewModel, Hilt, the network, a clock or a generated ID.
 *
 * The `*_USER_CONTENT` sets are the narrow allowlists for strings that
 * legitimately render without Android's en-XA `[`…`]` markers. Every entry is
 * user-entered or fixture-derived data (a name, address, note, or the letter
 * the list groups a fixture name under) — never UI copy. Do not widen these to
 * make a new literal pass: a plain-ASCII control label belongs in
 * `core/ui/src/main/res/values/strings.xml` (plus its de/es/fr/it siblings).
 */
internal object PseudoLocaleFixtures {
    val LOGIN_STATE = LoginUiState(
        serverUrl = "https://crm.example.com",
        mode = LoginMode.PASSWORD
    )

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

    /** Fixed identity: changing any value here is a deliberate golden update. */
    private const val UID = "urn:uuid:25350000-0000-4000-8000-000000000001"

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

    val SETTINGS_STATE = SettingsUiState(
        buildInfo = AppBuildInfo(
            versionName = "1.3.0",
            versionCode = 1042,
            buildType = "release",
            flavor = "obtainium",
            applicationId = "com.mycorrhizal.crm"
        )
    )

    /** Typed server URL only; every visible label is a `stringResource`. */
    val LOGIN_USER_CONTENT: Set<String> = emptySet()

    /**
     * The five fixture names, their email/phone subtitle, and the first letter
     * of the lastname the list groups each row under (L, H, T, J).
     */
    val CONTACT_LIST_USER_CONTENT: Set<String> = setOf(
        "Ada Lovelace",
        "Grace Hopper",
        "Alan Turing",
        "Katherine Johnson",
        "Margaret Hamilton",
        "ada@example.com · +44 20 7946 0001",
        "grace@example.com · +1 202 555 0102",
        "alan@example.com · +44 20 7946 0003",
        "katherine@example.com · +1 202 555 0104",
        "margaret@example.com · +1 202 555 0105",
        "L",
        "H",
        "T",
        "J"
    )

    /**
     * The populated contact's own data: name, quoted nickname, keywords,
     * email/phone/IMPP values, address, and the two free-text CRM notes.
     */
    val CONTACT_DETAIL_USER_CONTENT: Set<String> = setOf(
        "Dr Ada Lovelace",
        "\"The Enchantress\"",
        "mathematics, computing",
        "ada@example.com",
        "ada.lovelace@example.org",
        "+44 20 7946 0001",
        "+44 20 7946 0002",
        "matrix",
        "matrix:@ada:example.org",
        "12 St James's Square\nLondon SW1Y 4LB\nUnited Kingdom",
        "Introduced at a lecture on difference engines",
        "Prefers email"
    )

    /** Only the fixture build identity is data; it is not rendered on the root. */
    val SETTINGS_USER_CONTENT: Set<String> = emptySet()
}

/** Renders the login screen exactly as the pseudo goldens and the sweep see it. */
@Composable
internal fun PseudoLoginScreen() {
    MycorrhizalTheme(darkTheme = false) {
        LoginScreenContent(
            uiState = PseudoLocaleFixtures.LOGIN_STATE,
            onServerUrlChange = {},
            onModeChange = {},
            onSubmit = { _, _, _, _ -> }
        )
    }
}

/** Renders the populated contact list exactly as the pseudo goldens see it. */
@Composable
internal fun PseudoContactListScreen() {
    MycorrhizalTheme(darkTheme = false) {
        ContactListScreenContent(
            uiState = ContactListUiState(contacts = PseudoLocaleFixtures.SAMPLE_CONTACTS),
            onSearchQueryChange = {},
            onContactClick = {}
        )
    }
}

/** Renders the populated contact detail exactly as the pseudo goldens see it. */
@Composable
internal fun PseudoContactDetailScreen() {
    MycorrhizalTheme(darkTheme = false) {
        // The screen itself is a plain LazyColumn; in the app the surrounding
        // Scaffold paints the themed background. Paint it here too so the
        // pseudo golden is comparable with the base one.
        Surface(
            modifier = Modifier.fillMaxSize(),
            color = MaterialTheme.colorScheme.background
        ) {
            ContactDetailContent(contact = PseudoLocaleFixtures.CONTACT, dateFormat = "eu")
        }
    }
}

/** Renders the Settings root exactly as the pseudo goldens see it. */
@Composable
internal fun PseudoSettingsScreen() {
    MycorrhizalTheme(darkTheme = false) {
        Surface(
            modifier = Modifier.fillMaxSize(),
            color = MaterialTheme.colorScheme.background
        ) {
            SettingsRootContent(state = PseudoLocaleFixtures.SETTINGS_STATE, onLogout = {})
        }
    }
}
