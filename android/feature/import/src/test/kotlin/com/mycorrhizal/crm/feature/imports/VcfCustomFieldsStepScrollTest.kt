package com.mycorrhizal.crm.feature.imports

import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.network.DiscoveredCustomProperty
import com.mycorrhizal.crm.model.network.FieldDefinition
import com.mycorrhizal.crm.model.network.ImportFieldMapping
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issue #2630: on a short screen, choosing "Map to existing field" must bring the field picker it
 * reveals into view instead of leaving it below the fold.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], qualifiers = "w400dp-h420dp")
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class VcfCustomFieldsStepScrollTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val candidates =
        (1..4).map { DiscoveredCustomProperty(name = "x-prop-$it", value = "v") }

    @Test
    fun `choosing map reveals the definition picker on screen`() {
        val mappings = mutableStateMapOf<String, ImportFieldMapping>()
        composeTestRule.setContent {
            MycorrhizalTheme {
                CustomFieldsStep(
                    candidates = candidates,
                    definitions = listOf(FieldDefinition(id = "d1", label = "Hobby")),
                    mappings = mappings,
                    canContinue = true,
                    actions = object : CustomFieldActions {
                        override fun setFieldMappingAction(propertyName: String, action: String) {
                            mappings[propertyName] = ImportFieldMapping(propertyName, action)
                        }

                        override fun setFieldMappingDefinition(
                            propertyName: String,
                            definitionId: String
                        ) = Unit

                        override fun setFieldMappingLabel(propertyName: String, label: String) =
                            Unit

                        override fun setFieldMappingType(propertyName: String, type: String) = Unit

                        override fun continueFromCustomFields() = Unit
                    }
                )
            }
        }

        composeTestRule.onNodeWithTag("vcf-cf-x-prop-1-map").performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithTag("vcf-cf-x-prop-1-field").assertIsDisplayed()
    }
}
