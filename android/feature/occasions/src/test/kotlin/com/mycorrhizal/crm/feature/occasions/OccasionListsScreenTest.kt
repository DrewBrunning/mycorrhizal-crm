package com.mycorrhizal.crm.feature.occasions

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.domain.repository.OccasionObligationRepository
import com.mycorrhizal.crm.model.network.GiftShoppingItem
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import java.io.File
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class OccasionListsScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @get:Rule
    val tmp = TemporaryFolder()

    private val repository = mockk<OccasionObligationRepository>()

    private fun item(
        id: String,
        name: String = "Dana White",
        label: String = "Birthday gift",
        days: Int = 6,
        status: String = "needed",
        contactId: Int = 7
    ) = GiftShoppingItem(
        contactId = contactId,
        contactName = name,
        obligationId = id,
        label = label,
        date = "2026-12-18",
        daysUntil = days,
        status = status
    )

    private fun renderShopping(
        result: Result<List<GiftShoppingItem>>,
        onOpenContact: (Int) -> Unit = {}
    ): GiftShoppingListViewModel {
        coEvery { repository.giftShoppingList(30) } returns result
        val vm = GiftShoppingListViewModel(repository)
        composeTestRule.setContent {
            MycorrhizalTheme {
                GiftShoppingListScreen(onBack = {}, onOpenContact = onOpenContact, viewModel = vm)
            }
        }
        composeTestRule.waitForIdle()
        return vm
    }

    @Test
    fun `rows show contact, label, countdown and status`() {
        renderShopping(
            Result.success(
                listOf(
                    item("a", days = 0, status = "needed"),
                    item("b", name = "Sam", label = "Xmas", days = 1, status = "purchased"),
                    item("c", name = "Lee", label = "Cake", days = 12, status = "given")
                )
            )
        )
        composeTestRule.onNodeWithText("Dana White — Birthday gift").assertIsDisplayed()
        composeTestRule.onNodeWithText("Today").assertIsDisplayed()
        composeTestRule.onNodeWithText("Needed").assertIsDisplayed()
        composeTestRule.onNodeWithText("Sam — Xmas").assertIsDisplayed()
        composeTestRule.onNodeWithText("in 1 day").assertIsDisplayed()
        composeTestRule.onNodeWithText("Purchased").assertIsDisplayed()
        composeTestRule.onNodeWithText("in 12 days").assertIsDisplayed()
        composeTestRule.onNodeWithText("Given").assertIsDisplayed()
    }

    @Test
    fun `an unknown status falls back to its raw token`() {
        renderShopping(Result.success(listOf(item("a", status = "mystery"))))
        composeTestRule.onNodeWithText("mystery").assertIsDisplayed()
    }

    @Test
    fun `tapping a row opens that contact`() {
        var opened = -1
        renderShopping(
            Result.success(listOf(item("a", contactId = 42))),
            onOpenContact = { opened = it }
        )
        composeTestRule.onNodeWithText("Dana White — Birthday gift").performClick()
        assertEquals(42, opened)
    }

    @Test
    fun `empty window shows the empty message`() {
        renderShopping(Result.success(emptyList()))
        composeTestRule.onNodeWithText("No gifts needed in this window.").assertIsDisplayed()
    }

    @Test
    fun `a load failure shows the error with a retry that refetches`() {
        renderShopping(Result.failure(ApiError.Client(500, "boom")))
        composeTestRule.onNodeWithText("boom").assertIsDisplayed()
        composeTestRule.onNodeWithText("No gifts needed in this window.").assertDoesNotExist()
        coEvery { repository.giftShoppingList(30) } returns Result.success(listOf(item("a")))
        composeTestRule.onNodeWithText("Retry").performClick()
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithText("Dana White — Birthday gift").assertIsDisplayed()
    }

    @Test
    fun `the 90 day chip loads the longer window`() {
        renderShopping(Result.success(emptyList()))
        coEvery { repository.giftShoppingList(90) } returns
            Result.success(listOf(item("far", label = "Far gift", days = 80)))
        composeTestRule.onNodeWithText("90 days").performClick()
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithText("Dana White — Far gift").assertIsDisplayed()
        coVerify { repository.giftShoppingList(90) }
    }

    private fun renderActions(
        onShare: (ByteArray) -> Unit = {},
        onOpenShoppingList: () -> Unit = {}
    ): OccasionCardListViewModel {
        val vm = OccasionCardListViewModel(repository)
        composeTestRule.setContent {
            MycorrhizalTheme {
                OccasionListsActions(
                    onOpenShoppingList = onOpenShoppingList,
                    onShareCsv = onShare,
                    viewModel = vm
                )
            }
        }
        composeTestRule.waitForIdle()
        return vm
    }

    @Test
    fun `share card list fetches the csv and hands the bytes to the share callback`() {
        coEvery { repository.cardListCsv("card") } returns Result.success("a,b".toByteArray())
        var shared: ByteArray? = null
        renderActions(onShare = { shared = it })
        composeTestRule.onNodeWithText("Card list").assertIsDisplayed()
        composeTestRule.onNodeWithText("Share as CSV").performClick()
        composeTestRule.waitForIdle()
        assertArrayEquals("a,b".toByteArray(), shared)
    }

    @Test
    fun `a failed card list download shows the error and shares nothing`() {
        coEvery { repository.cardListCsv("card") } returns
            Result.failure(ApiError.Client(500, "boom"))
        var shared = false
        renderActions(onShare = { shared = true })
        composeTestRule.onNodeWithText("Share as CSV").performClick()
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithText("boom").assertIsDisplayed()
        assertTrue(!shared)
    }

    @Test
    fun `the gift shopping list row opens the list`() {
        var opened = false
        renderActions(onOpenShoppingList = { opened = true })
        composeTestRule.onNodeWithText("Gift shopping list").performClick()
        assertTrue(opened)
    }

    @Test
    fun `the csv is written under the cache exports directory the file provider exposes`() {
        val cache = tmp.newFolder("cache")
        val file = writeCardListCsv(cache, "Contact Name,Address\n".toByteArray())
        assertEquals(File(cache, "exports"), file.parentFile)
        assertEquals("mycorrhizal-card-list.csv", file.name)
        assertEquals("Contact Name,Address\n", file.readText())
    }

    @Test
    fun `share is disabled while the download runs`() {
        coEvery { repository.cardListCsv("card") } coAnswers {
            kotlinx.coroutines.awaitCancellation()
        }
        renderActions()
        composeTestRule.onNodeWithText("Share as CSV").performClick()
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithText("Share as CSV").assertIsNotEnabled()
    }
}
