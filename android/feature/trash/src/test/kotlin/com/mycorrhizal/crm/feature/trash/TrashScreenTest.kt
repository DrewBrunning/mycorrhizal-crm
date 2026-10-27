package com.mycorrhizal.crm.feature.trash

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.TrashedContact
import com.mycorrhizal.crm.testing.a11y.assertAccessibleSemantics
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], qualifiers = "w480dp-h1600dp")
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class TrashScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val ada = TrashedContact(
        contact = ContactSummary(id = 1, firstname = "Ada", lastname = "Test"),
        deletedAt = "2026-10-01T00:00:00Z",
        purgeAt = "2026-10-31T00:00:00Z"
    )
    private val bob = TrashedContact(
        contact = ContactSummary(id = 2, firstname = "Bob", lastname = "Test"),
        deletedAt = "2026-10-02T00:00:00Z",
        purgeAt = null
    )

    @Test
    fun `rows show the name and fire restore and delete-forever for that row`() {
        var restored: Int? = null
        var purged: Int? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                TrashList(
                    items = listOf(ada, bob),
                    retentionDays = 30,
                    enabled = true,
                    onRestore = { restored = it.contact.id },
                    onPurge = { purged = it.contact.id }
                )
            }
        }

        composeTestRule.onNodeWithText("Ada Test").assertIsDisplayed()
        composeTestRule.onNodeWithText("Bob Test").assertIsDisplayed()
        composeTestRule.onNodeWithTag("trash-restore-2").performClick()
        composeTestRule.onNodeWithTag("trash-delete-1").performClick()
        assertEquals(2, restored)
        assertEquals(1, purged)
    }

    @Test
    fun `retention zero says the contacts are kept until the trash is emptied`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                TrashList(
                    items = listOf(bob),
                    retentionDays = 0,
                    enabled = true,
                    onRestore = {},
                    onPurge = {}
                )
            }
        }

        composeTestRule.onNodeWithTag("trash-retention").assertIsDisplayed()
        composeTestRule.onNodeWithText("Deleted contacts are kept until you empty the trash.")
            .assertIsDisplayed()
    }

    @Test
    fun `list passes the accessibility sweep`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                TrashList(
                    items = listOf(ada, bob),
                    retentionDays = 30,
                    enabled = true,
                    onRestore = {},
                    onPurge = {}
                )
            }
        }

        composeTestRule.assertAccessibleSemantics()
    }
}
