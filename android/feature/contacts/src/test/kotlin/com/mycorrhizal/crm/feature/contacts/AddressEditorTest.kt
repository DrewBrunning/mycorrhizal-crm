package com.mycorrhizal.crm.feature.contacts

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performSemanticsAction
import androidx.compose.ui.test.performTextClearance
import androidx.compose.ui.test.performTextReplacement
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performTextInput
import com.mycorrhizal.crm.model.network.Address
import com.mycorrhizal.crm.model.network.AddressComponent
import com.mycorrhizal.crm.ui.components.AddressEditor
import com.mycorrhizal.crm.ui.components.AddressGeocodeState
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class AddressEditorTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun `typing street and city emits the real registry kinds name and locality`() {
        // M7 test case 5: the registry kinds are `name`/`locality`, NOT `street`/`city` —
        // getting that wrong is exactly what shipped T67's device-import bug.
        var addresses by mutableStateOf(listOf(Address()))
        composeTestRule.setContent {
            MycorrhizalTheme {
                AddressEditor(addresses = addresses, onChange = { addresses = it })
            }
        }
        composeTestRule.onNodeWithText("Street").performTextInput("123 Main St")
        composeTestRule.onNodeWithText("City").performTextInput("Springfield")
        val components = addresses.firstOrNull()?.components.orEmpty()
        assertEquals("123 Main St", components.firstOrNull { it.kind == "name" }?.value)
        assertEquals("Springfield", components.firstOrNull { it.kind == "locality" }?.value)
        // No `street`/`city` kinds anywhere.
        assertEquals(null, components.firstOrNull { it.kind == "street" })
        assertEquals(null, components.firstOrNull { it.kind == "city" })
    }

    @Test
    fun `editing a loaded address preserves id contexts and pref`() {
        var addresses by mutableStateOf(
            listOf(
                Address(
                    id = "addr-1",
                    contexts = listOf("home", "delivery"),
                    pref = 1,
                    coordinates = "geo:1,2",
                    components = listOf(AddressComponent(kind = "name", value = "1 Elm St")),
                ),
            ),
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                AddressEditor(addresses = addresses, onChange = { addresses = it })
            }
        }
        composeTestRule.onNodeWithText("City").performTextInput("Metropolis")
        val addr = addresses.firstOrNull()
        assertEquals("addr-1", addr?.id)
        assertEquals(listOf("home", "delivery"), addr?.contexts)
        assertEquals(1, addr?.pref)
        assertEquals("geo:1,2", addr?.coordinates)
        // The untouched street component survives.
        assertEquals("1 Elm St", addr?.components?.firstOrNull { it.kind == "name" }?.value)
    }

    @Test
    fun `loaded additional fields auto-reveal without a toggle tap`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                AddressEditor(
                    addresses = listOf(
                        Address(
                            id = "addr-1",
                            components = listOf(
                                AddressComponent(kind = "name", value = "1 Elm St"),
                                AddressComponent(kind = "postOfficeBox", value = "PO 42"),
                            ),
                        ),
                    ),
                    onChange = {},
                )
            }
        }
        composeTestRule.onNodeWithText("PO box").assertIsDisplayed()
    }

    // --- ADR 0031 / issue #1287: coordinates + find coordinates ---

    private fun setEditor(
        initial: List<Address>,
        geocode: AddressGeocodeState = AddressGeocodeState(canGeocode = true),
        onFind: ((String) -> Unit)? = {},
        onFound: (String) -> Unit = {},
    ): () -> List<Address> {
        var addresses by mutableStateOf(initial)
        composeTestRule.setContent {
            MycorrhizalTheme {
                AddressEditor(
                    addresses = addresses,
                    onChange = { addresses = it },
                    geocode = geocode,
                    onFindCoordinates = onFind?.let { f -> { id: String -> f(id); onFound(id) } },
                )
            }
        }
        return { addresses }
    }

    private val coordinatesLabel = "Coordinates (latitude, longitude)"

    @Test
    fun `stored coordinates display as a readable lat, lng pair`() {
        setEditor(listOf(Address(id = "a", coordinates = "geo:51.5007,-0.1246")))

        composeTestRule.onNodeWithText("51.5007, -0.1246").assertIsDisplayed()
    }

    @Test
    fun `an unparseable stored value is shown verbatim and flagged`() {
        setEditor(listOf(Address(id = "a", coordinates = "geo:nonsense")))

        composeTestRule.onNodeWithText("geo:nonsense").assertIsDisplayed()
        composeTestRule.onNodeWithText("Enter latitude", substring = true).assertIsDisplayed()
    }

    @Test
    fun `a valid typed pair is committed as a geo URI`() {
        val current = setEditor(listOf(Address(id = "a")))

        composeTestRule.onNodeWithText(coordinatesLabel).performTextInput("12.5, -45.25")

        assertEquals("geo:12.5,-45.25", current().single().coordinates)
    }

    @Test
    fun `an out-of-range pair is flagged and not committed`() {
        val current = setEditor(listOf(Address(id = "a", coordinates = "geo:1,2")))

        composeTestRule.onNodeWithText("1.0, 2.0").performTextReplacement("95, 10")

        composeTestRule.onNodeWithText("Enter latitude", substring = true).assertIsDisplayed()
        assertEquals("geo:1,2", current().single().coordinates)
    }

    @Test
    fun `clearing the field removes the coordinates`() {
        val current = setEditor(listOf(Address(id = "a", coordinates = "geo:1,2")))

        composeTestRule.onNodeWithText("1.0, 2.0").performTextClearance()

        assertNull(current().single().coordinates)
    }

    @Test
    fun `find coordinates fires for the saved address`() {
        val found = mutableListOf<String>()
        setEditor(listOf(Address(id = "a1")), onFound = { found += it })

        composeTestRule.onNodeWithText("Find coordinates").assertIsEnabled().performClick()

        assertEquals(listOf("a1"), found)
    }

    @Test
    fun `a geocode result written onto the address replaces the typed text`() {
        var addresses by mutableStateOf(listOf(Address(id = "a1")))
        composeTestRule.setContent {
            MycorrhizalTheme { AddressEditor(addresses = addresses, onChange = { addresses = it }) }
        }
        composeTestRule.onNodeWithText(coordinatesLabel).performTextInput("9,")

        addresses = listOf(Address(id = "a1", coordinates = "geo:48.85,2.35"))
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("48.85, 2.35").assertIsDisplayed()
    }

    @Test
    fun `typing a trailing zero is not reformatted under the cursor`() {
        val current = setEditor(listOf(Address(id = "a")))

        composeTestRule.onNodeWithText(coordinatesLabel).performTextInput("1, -0.10")
        composeTestRule.waitForIdle()

        // The field keeps the typed text verbatim -- that is the "not
        // reformatted under the cursor" contract, and it must not be reset by
        // the LaunchedEffect re-sync.
        composeTestRule.onNodeWithText("1, -0.10").assertIsDisplayed()
        // The committed value is the canonical plain-decimal geo URI, with no
        // trailing zeros and never scientific notation (#1443): `1.0` -> `1`,
        // matching the web and backend formatters. The old expectation pinned
        // Double.toString's "1.0"; stripping zeros is the intended change.
        assertEquals("geo:1,-0.1", current().single().coordinates)
    }

    private fun assertSensitiveBlocked(sensitivity: String) {
        val found = mutableListOf<String>()
        setEditor(listOf(Address(id = "a1", sensitivity = sensitivity)), onFound = { found += it })

        composeTestRule.onNodeWithText("Find coordinates").assertIsNotEnabled()
        composeTestRule.onNodeWithText("never sent to the geocoder", substring = true).assertExists()
        assertTrue(found.isEmpty())
        // Manual entry still works for a sensitive address.
        composeTestRule.onNodeWithText(coordinatesLabel).assertIsEnabled()
    }

    @Test
    fun `find coordinates is disabled with a reason for a private address`() = assertSensitiveBlocked("private")

    @Test
    fun `find coordinates is disabled with a reason for a secret address`() = assertSensitiveBlocked("secret")

    @Test
    fun `find coordinates works for a new unsaved row and reports its positional key`() {
        val found = mutableListOf<String>()
        setEditor(listOf(Address()), geocode = AddressGeocodeState(canGeocode = true), onFound = { found += it })

        composeTestRule.onNodeWithText("Find coordinates").assertIsEnabled().performClick()

        assertEquals(listOf("row-0"), found)
    }

    @Test
    fun `find coordinates is disabled with a save-first reason when the contact is not saved`() {
        setEditor(listOf(Address()), geocode = AddressGeocodeState(canGeocode = false))
        composeTestRule.onNodeWithText("Find coordinates").assertIsNotEnabled()
        composeTestRule.onNodeWithText("Save this contact first", substring = true).assertExists()
    }

    @Test
    fun `find coordinates is disabled with a local-profile reason`() {
        val found = mutableListOf<String>()
        setEditor(
            listOf(Address(id = "a1")),
            geocode = AddressGeocodeState(canGeocode = true, localProfile = true),
            onFound = { found += it },
        )
        composeTestRule.onNodeWithText("Find coordinates").assertIsNotEnabled()
        composeTestRule.onNodeWithText("Not available in local profiles", substring = true).assertExists()
        assertTrue(found.isEmpty())
        // Manual entry still works.
        composeTestRule.onNodeWithText(coordinatesLabel).assertIsEnabled()
    }

    @Test
    fun `a new row's in-flight state and error are keyed by its positional key`() {
        setEditor(
            listOf(Address()),
            geocode = AddressGeocodeState(canGeocode = true, inFlight = setOf("row-0")),
        )
        composeTestRule.onNodeWithText("Looking up coordinates…").assertIsNotEnabled()
    }

    @Test
    fun `adding a row mints a stable id so geocode state is keyed by identity`() {
        val current = setEditor(listOf(Address(id = "a1")))

        composeTestRule.onNodeWithContentDescription("Add")
            // Off-screen in the unscrolled test host, so invoke the click action directly.
            .performSemanticsAction(androidx.compose.ui.semantics.SemanticsActions.OnClick)
        composeTestRule.waitForIdle()

        val added = current()[1]
        assertTrue(!added.id.isNullOrBlank())
        assertEquals(listOf("home"), added.contexts)
    }

    @Test
    fun `the sensitivity picker shows normal for a null value and preserves it untouched`() {
        val current = setEditor(listOf(Address(id = "a")))

        composeTestRule.onNodeWithText("Sensitivity").assertIsDisplayed()
        composeTestRule.onNodeWithText("Normal").assertIsDisplayed()
        composeTestRule.onNodeWithText("City").performTextInput("Metropolis")

        assertNull(current().single().sensitivity)
    }

    @Test
    fun `a loaded secret value shows as Secret`() {
        setEditor(listOf(Address(id = "a", sensitivity = "secret")))

        composeTestRule.onNodeWithText("Secret").assertIsDisplayed()
    }

    @Test
    fun `the picker offers normal private and secret and writes the choice`() {
        val current = setEditor(listOf(Address(id = "a")))

        composeTestRule.onNodeWithText("Normal").performClick()
        composeTestRule.onNodeWithText("Private").assertIsDisplayed()
        composeTestRule.onNodeWithText("Secret").assertIsDisplayed()
        composeTestRule.onNodeWithText("Private").performClick()

        assertEquals("private", current().single().sensitivity)
    }

    @Test
    fun `picking private disables find coordinates live and picking normal re-enables it`() {
        val found = mutableListOf<String>()
        setEditor(listOf(Address(id = "a1")), onFound = { found += it })
        composeTestRule.onNodeWithText("Find coordinates").assertIsEnabled()

        composeTestRule.onNodeWithText("Normal").performClick()
        composeTestRule.onNodeWithText("Private").performClick()
        composeTestRule.onNodeWithText("Find coordinates").assertIsNotEnabled()
        composeTestRule.onNodeWithText("never sent to the geocoder", substring = true).assertExists()

        composeTestRule.onNodeWithText("Private").performClick()
        composeTestRule.onNodeWithText("Normal").performClick()
        composeTestRule.onNodeWithText("Find coordinates").assertIsEnabled().performClick()
        assertEquals(listOf("a1"), found)
    }

    @Test
    fun `no contact id or no callback means save first`() {
        setEditor(listOf(Address(id = "a1")), geocode = AddressGeocodeState(canGeocode = false))
        composeTestRule.onNodeWithText("Find coordinates").assertIsNotEnabled()
    }

    @Test
    fun `without a find callback the action is unavailable`() {
        setEditor(listOf(Address(id = "a1")), onFind = null)
        composeTestRule.onNodeWithText("Find coordinates").assertIsNotEnabled()
    }

    @Test
    fun `an in-flight lookup disables the button and says so`() {
        setEditor(
            listOf(Address(id = "a1")),
            geocode = AddressGeocodeState(canGeocode = true, inFlight = setOf("a1")),
        )

        composeTestRule.onNodeWithText("Looking up coordinates…").assertIsNotEnabled()
    }

    @Test
    fun `a failed lookup shows its error under the row`() {
        setEditor(
            listOf(Address(id = "a1")),
            geocode = AddressGeocodeState(canGeocode = true, errors = mapOf("a1" to "geocoding is not enabled on this server")),
        )

        composeTestRule.onNodeWithText("geocoding is not enabled on this server").assertExists()
    }

    @Test
    fun `editing the street keeps coordinates and sensitivity`() {
        val current = setEditor(
            listOf(Address(id = "a", sensitivity = "secret", coordinates = "geo:1,2")),
        )

        composeTestRule.onNodeWithText("City").performTextInput("Metropolis")

        assertEquals("secret", current().single().sensitivity)
        assertEquals("geo:1,2", current().single().coordinates)
    }
}
