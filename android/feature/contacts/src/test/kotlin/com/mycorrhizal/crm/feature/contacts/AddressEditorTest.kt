package com.mycorrhizal.crm.feature.contacts

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performTextInput
import com.mycorrhizal.crm.model.network.Address
import com.mycorrhizal.crm.model.network.AddressComponent
import com.mycorrhizal.crm.ui.components.AddressEditor
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
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
}
