package com.mycorrhizal.crm.feature.timeline

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.Scaffold
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
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
class NotesScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(
        state: NotesUiState,
        onEditNote: (Int) -> Unit = {},
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                Scaffold { padding ->
                    Box(modifier = Modifier.padding(padding)) {
                        when {
                            state.isLoading -> LoadingSkeleton()
                            state.notes.isEmpty() && state.error == null ->
                                EmptyState("No notes yet")
                            else -> {
                                LazyColumn {
                                    items(state.notes) { note ->
                                        NoteListItem(
                                            note = note,
                                            onClick = { onEditNote(note.id) },
                                        )
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
    }

    @Test
    fun `shows empty state when no notes`() {
        setContent(NotesUiState(contactId = 5, notes = emptyList()))
        composeTestRule.onNodeWithText("No notes yet").assertIsDisplayed()
    }

    @Test
    fun `shows note list items`() {
        setContent(
            NotesUiState(
                contactId = 5,
                notes = listOf(
                    Note(id = 3, content = "Loves climbing"),
                    Note(id = 4, content = "Met at conference"),
                ),
            ),
        )
        composeTestRule.onNodeWithText("Loves climbing").assertIsDisplayed()
        composeTestRule.onNodeWithText("Met at conference").assertIsDisplayed()
    }

    @Test
    fun `tapping a note invokes the edit callback`() {
        var editedId: Int? = null
        setContent(
            NotesUiState(
                contactId = 5,
                notes = listOf(Note(id = 3, content = "Loves climbing")),
            ),
            onEditNote = { editedId = it },
        )
        composeTestRule.onNodeWithText("Loves climbing").performClick()
        assertEquals(3, editedId)
    }
}
