package com.mycorrhizal.crm.feature.settings

import android.content.Context
import androidx.annotation.StringRes
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.ExperimentalTestApi
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performCustomAccessibilityActionWithLabel
import androidx.compose.ui.test.performTouchInput
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.domain.repository.FieldDefinitionRepository
import com.mycorrhizal.crm.model.network.FieldConstraints
import com.mycorrhizal.crm.model.network.FieldDefinition
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.a11y.assertAccessibleSemantics
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/** Issue #830; row interaction reshaped by #1775. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@OptIn(ExperimentalTestApi::class)
class FieldDefinitionsScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun str(@StringRes res: Int, vararg args: Any): String =
        ApplicationProvider.getApplicationContext<Context>().getString(res, *args)

    private fun setScreen(
        result: Result<List<FieldDefinition>> = Result.success(
            listOf(
                FieldDefinition(
                    id = "d1",
                    label = "Coffee order",
                    key = "coffee_order",
                    type = "string",
                    sensitivity = "normal",
                    projection = "internal-only",
                ),
            ),
        ),
        onEdit: (String) -> Unit = {},
    ): FieldDefinitionRepository {
        val repository = mockk<FieldDefinitionRepository>()
        coEvery { repository.list() } returns result
        coEvery { repository.reorder(any()) } returns Result.success(emptyList())
        val viewModel = FieldDefinitionsViewModel(repository)

        composeTestRule.setContent {
            MycorrhizalTheme {
                FieldDefinitionsScreen(onBack = {}, onCreate = {}, onEdit = onEdit, viewModel = viewModel)
            }
        }
        composeTestRule.waitForIdle()
        return repository
    }

    private fun twoDefinitions() = Result.success(
        listOf(
            FieldDefinition(id = "d1", label = "Coffee order", type = "string", sensitivity = "normal", position = 0),
            FieldDefinition(id = "d2", label = "Tea order", type = "string", sensitivity = "normal", position = 1),
        ),
    )

    @Test
    fun `field definitions screen has no accessibility violations`() {
        setScreen()

        composeTestRule.assertAccessibleSemantics()
    }

    @Test
    fun `an empty list renders the empty state`() {
        setScreen(Result.success(emptyList()))

        composeTestRule.onNodeWithText(str(R.string.settings_custom_fields_empty)).assertIsDisplayed()
    }

    @Test
    fun `a definition row shows its label and readable metadata text`() {
        setScreen()

        composeTestRule.onNodeWithText("Coffee order").assertIsDisplayed()
        // Readable one-line metadata, not the old low-contrast disabled chips.
        composeTestRule.onNodeWithText(
            "${str(R.string.custom_field_type_string)} · ${str(R.string.relationships_sensitivity_normal)}",
        ).assertIsDisplayed()
    }

    @Test
    fun `a multi-valued definition adds the multi metadata`() {
        setScreen(
            Result.success(
                listOf(
                    FieldDefinition(
                        id = "d1",
                        label = "Favorite drinks",
                        type = "string",
                        constraints = FieldConstraints(multi = true),
                        sensitivity = "normal",
                    ),
                ),
            ),
        )

        composeTestRule.onNodeWithText(
            "${str(R.string.custom_field_type_string)} · ${str(R.string.settings_custom_fields_multi)} · " +
                str(R.string.relationships_sensitivity_normal),
        ).assertIsDisplayed()
    }

    @Test
    fun `the raw vCard property name is not shown in the list`() {
        setScreen(
            Result.success(
                listOf(
                    FieldDefinition(
                        id = "d1",
                        label = "Favourite coffee",
                        type = "string",
                        sensitivity = "normal",
                        projection = "vcard:X-FAVORITE-COFFEE",
                    ),
                ),
            ),
        )

        composeTestRule.onNodeWithText("vcard:X-FAVORITE-COFFEE").assertDoesNotExist()
    }

    @Test
    fun `tapping the row invokes the onEdit callback with the definition id`() {
        var editedId: String? = null
        setScreen(onEdit = { editedId = it })

        composeTestRule.onNodeWithText("Coffee order").performClick()

        assertEquals("d1", editedId)
    }

    @Test
    fun `a row exposes only the drag handle as a visible action`() {
        setScreen()

        // The old per-row edit/delete/move icon buttons are gone.
        composeTestRule.onNodeWithContentDescription(str(R.string.settings_custom_fields_edit_named, "Coffee order"))
            .assertDoesNotExist()
        composeTestRule.onNodeWithContentDescription(str(R.string.settings_custom_fields_delete_named, "Coffee order"))
            .assertDoesNotExist()
        composeTestRule.onNodeWithContentDescription(str(R.string.settings_custom_fields_move_up_named, "Coffee order"))
            .assertDoesNotExist()
        composeTestRule.onNodeWithContentDescription(str(R.string.settings_custom_fields_move_down_named, "Coffee order"))
            .assertDoesNotExist()

        composeTestRule.onNodeWithContentDescription(str(R.string.settings_custom_fields_reorder_named, "Coffee order"))
            .assertIsDisplayed()
    }

    @Test
    fun `the drag handle exposes move up and down as TalkBack custom actions`() {
        setScreen(twoDefinitions())

        val actions = composeTestRule
            .onNodeWithContentDescription(str(R.string.settings_custom_fields_reorder_named, "Coffee order"))
            .fetchSemanticsNode()
            .config
            .getOrNull(SemanticsActions.CustomActions)

        assertEquals(
            listOf(str(R.string.settings_custom_fields_move_down_named, "Coffee order")),
            actions?.map { it.label },
        )
    }

    @Test
    fun `performing the TalkBack move-down action persists the swapped order`() {
        val repository = setScreen(twoDefinitions())

        composeTestRule
            .onNodeWithContentDescription(str(R.string.settings_custom_fields_reorder_named, "Coffee order"))
            .performCustomAccessibilityActionWithLabel(
                str(R.string.settings_custom_fields_move_down_named, "Coffee order"),
            )

        coVerify { repository.reorder(listOf("d2", "d1")) }
    }

    @Test
    fun `dragging the handle down reorders the definition`() {
        val repository = setScreen(twoDefinitions())

        composeTestRule
            .onNodeWithContentDescription(str(R.string.settings_custom_fields_reorder_named, "Coffee order"))
            .performTouchInput {
                down(center)
                advanceEventTime(700)
                moveBy(Offset(0f, 200f))
                advanceEventTime(100)
                up()
            }
        composeTestRule.waitForIdle()

        coVerify { repository.reorder(listOf("d2", "d1")) }
    }

    @Test
    fun `a failed load with an empty list renders the error text`() {
        setScreen(Result.failure(ApiError.Client(500, "boom")))

        // The error is both the body Text and the transient snackbar, so assert
        // on the collection rather than a single node (mirrors TagsScreenTest).
        assertTrue(composeTestRule.onAllNodesWithText("boom").fetchSemanticsNodes().isNotEmpty())
    }
}
