package com.mycorrhizal.crm.ui.components

import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextReplacement
import com.mycorrhizal.crm.model.network.Author
import com.mycorrhizal.crm.model.network.CardNote
import com.mycorrhizal.crm.model.network.Timestamp
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issue #1628: behaviour of the `Card.notes[]` editor (#832). The index-scoped
 * edit/delete map and the `noteAuthorCaption` present/absent branches are the
 * load-bearing parts.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class CardNotesEditorTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setEditor(initial: List<CardNote>, onChange: (List<CardNote>) -> Unit = {}) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                CardNotesEditor(items = initial, onChange = onChange, label = "Notes")
            }
        }
    }

    @Test
    fun `editing the middle note changes only that index`() {
        var latest: List<CardNote>? = null
        setEditor(
            listOf(CardNote(note = "one"), CardNote(note = "two"), CardNote(note = "three")),
            onChange = { latest = it },
        )

        composeTestRule.onNodeWithText("Value 2").performTextReplacement("TWO")

        // Only index 1 is rewritten; the other notes keep their text and order.
        assertEquals(listOf("one", "TWO", "three"), latest!!.map { it.note })
    }

    @Test
    fun `deleting the first note preserves the rest in order`() {
        var latest: List<CardNote>? = null
        setEditor(
            listOf(CardNote(note = "one"), CardNote(note = "two"), CardNote(note = "three")),
            onChange = { latest = it },
        )

        composeTestRule.onAllNodesWithContentDescription("Remove")[0].performClick()

        assertEquals(listOf("two", "three"), latest!!.map { it.note })
    }

    @Test
    fun `an import-populated author and created render as a read-only caption`() {
        setEditor(
            listOf(
                CardNote(
                    note = "imported",
                    author = Author(name = "Ada Lovelace"),
                    created = Timestamp(utc = "2026-01-01"),
                ),
            ),
        )

        composeTestRule.onNodeWithText("— Ada Lovelace, 2026-01-01").assertExists()
    }

    @Test
    fun `a hand-typed note has no author caption`() {
        setEditor(listOf(CardNote(note = "typed by hand")))

        composeTestRule.onNodeWithText("— Ada Lovelace, 2026-01-01").assertDoesNotExist()
        // Same row, no caption: the note text itself is still shown.
        composeTestRule.onNodeWithText("typed by hand").assertExists()
    }
}
