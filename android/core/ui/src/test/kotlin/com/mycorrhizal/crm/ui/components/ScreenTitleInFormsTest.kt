package com.mycorrhizal.crm.ui.components

import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.assertIsFocused
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/** Issue #1922: the shared form frame (Notes -> New note, activities, reminders, ...) focuses its title. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ScreenTitleInFormsTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun `a form's title takes focus instead of the Back button`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                val snackbar = remember { SnackbarHostState() }
                FormScaffold(
                    title = "New note",
                    onBack = {},
                    saveLabel = "Create",
                    isSaving = false,
                    onSave = {},
                    snackbarHostState = snackbar
                ) { padding: PaddingValues ->
                    Text(
                        "Form body",
                        modifier = Modifier.padding(padding)
                    )
                }
            }
        }
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("New note").assertIsFocused()
    }
}
