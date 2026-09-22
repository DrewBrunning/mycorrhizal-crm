package com.mycorrhizal.crm.feature.settings

import android.content.Context
import androidx.annotation.StringRes
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
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

/** Issue #830. Mounts the real top-level [FieldDefinitionsScreen], mirroring `TagsScreenTest`. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
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
        coEvery { repository.delete(any()) } returns Result.success(Unit)
        val viewModel = FieldDefinitionsViewModel(repository)

        composeTestRule.setContent {
            MycorrhizalTheme {
                FieldDefinitionsScreen(onBack = {}, onCreate = {}, onEdit = onEdit, viewModel = viewModel)
            }
        }
        return repository
    }

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
    fun `a definition row shows its label and type chip`() {
        setScreen()

        composeTestRule.onNodeWithText("Coffee order").assertIsDisplayed()
        composeTestRule.onNodeWithText(str(R.string.custom_field_type_string)).assertIsDisplayed()
    }

    @Test
    fun `the multi chip only appears for a multi-valued definition`() {
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

        composeTestRule.onNodeWithText(str(R.string.settings_custom_fields_multi)).assertIsDisplayed()
    }

    @Test
    fun `tapping edit invokes the onEdit callback with the definition id`() {
        var editedId: String? = null
        setScreen(onEdit = { editedId = it })

        composeTestRule.onNodeWithContentDescription(str(R.string.settings_custom_fields_edit_named, "Coffee order"))
            .performClick()

        assertEquals("d1", editedId)
    }

    @Test
    fun `tapping delete then confirming calls the repository`() {
        val repository = setScreen()

        composeTestRule.onNodeWithContentDescription(str(R.string.settings_custom_fields_delete_named, "Coffee order"))
            .performClick()
        composeTestRule.onNodeWithText(str(R.string.action_delete)).performClick()

        coVerify { repository.delete("d1") }
    }

    @Test
    fun `a failed load with an empty list renders the error text`() {
        setScreen(Result.failure(ApiError.Client(500, "boom")))

        // The error is both the body Text and the transient snackbar, so assert
        // on the collection rather than a single node (mirrors TagsScreenTest).
        assertTrue(composeTestRule.onAllNodesWithText("boom").fetchSemanticsNodes().isNotEmpty())
    }
}
