package com.mycorrhizal.crm.feature.occasions

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.domain.repository.OccasionObligationRepository
import com.mycorrhizal.crm.model.AppVersion
import com.mycorrhizal.crm.model.network.ErrorReason
import com.mycorrhizal.crm.model.network.ErrorReasonCode
import com.mycorrhizal.crm.model.network.GiftShoppingItem
import com.mycorrhizal.crm.model.network.OccasionCardListSkippedReasons
import com.mycorrhizal.crm.model.network.OccasionCardListSummary
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.ui.LocalServerVersion
import com.mycorrhizal.crm.ui.LocalToday
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import java.io.File
import java.time.LocalDate
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

private val TODAY: LocalDate = LocalDate.of(2026, 11, 13)

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
        date = TODAY.plusDays(days.toLong()).toString(),
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
            CompositionLocalProvider(LocalToday provides TODAY) {
                MycorrhizalTheme {
                    GiftShoppingListScreen(
                        onBack = {},
                        onOpenContact = onOpenContact,
                        viewModel = vm
                    )
                }
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
        onOpenShoppingList: () -> Unit = {},
        serverVersion: AppVersion? = null
    ): OccasionCardListViewModel {
        // Most tests are about the CSV share; the summary is stubbed as a plain success unless a
        // test arranged its own.
        if (!summaryStubbed) {
            coEvery { repository.cardListSummary("card") } returns
                Result.success(OccasionCardListSummary(total = 1, included = 1))
        }
        val vm = OccasionCardListViewModel(repository)
        composeTestRule.setContent {
            CompositionLocalProvider(LocalServerVersion provides serverVersion) {
                MycorrhizalTheme {
                    OccasionListsActions(
                        onOpenShoppingList = onOpenShoppingList,
                        onShareCsv = onShare,
                        viewModel = vm
                    )
                }
            }
        }
        composeTestRule.waitForIdle()
        return vm
    }

    private var summaryStubbed = false

    private fun stubSummary(result: Result<OccasionCardListSummary>) {
        summaryStubbed = true
        coEvery { repository.cardListSummary("card") } returns result
    }

    private fun summaryOf(total: Int, included: Int, noAddress: Int = 0, privateOnly: Int = 0) =
        OccasionCardListSummary(
            total = total,
            included = included,
            skipped = noAddress + privateOnly,
            skippedReasons = OccasionCardListSkippedReasons(noAddress, privateOnly)
        )

    @Test
    fun `the summary names how many recipients are included and why others are skipped`() {
        stubSummary(Result.success(summaryOf(12, 9, noAddress = 2, privateOnly = 1)))
        renderActions()
        composeTestRule
            .onNodeWithText(
                "9 recipients; 3 skipped (no postal address: 2, private address only: 1)"
            )
            .assertIsDisplayed()
        composeTestRule.onNodeWithText("Share as CSV").assertIsDisplayed()
    }

    @Test
    fun `only the skip reasons that apply are listed`() {
        stubSummary(Result.success(summaryOf(4, 3, privateOnly = 1)))
        renderActions()
        composeTestRule
            .onNodeWithText("3 recipients; 1 skipped (private address only: 1)")
            .assertIsDisplayed()
    }

    @Test
    fun `a summary with nothing skipped is just the recipient count, singular for one`() {
        stubSummary(Result.success(summaryOf(1, 1)))
        renderActions()
        composeTestRule.onNodeWithText("1 recipient").assertIsDisplayed()
    }

    @Test
    fun `the summary is a polite live region`() {
        stubSummary(Result.success(summaryOf(2, 2)))
        renderActions()
        composeTestRule
            .onNode(
                SemanticsMatcher.expectValue(
                    SemanticsProperties.LiveRegion,
                    LiveRegionMode.Polite
                )
            )
            .assertTextContains("2 recipients")
    }

    @Test
    fun `no recipients with an address replaces the share action with a note`() {
        stubSummary(Result.success(summaryOf(2, 0, noAddress = 2)))
        renderActions()
        composeTestRule.onNodeWithText("No recipients have a postal address.").assertIsDisplayed()
        composeTestRule.onNodeWithTag("occasion-card-list-share").assertDoesNotExist()
    }

    @Test
    fun `no active card occasions says so and offers no share`() {
        stubSummary(Result.success(summaryOf(0, 0)))
        renderActions()
        composeTestRule.onNodeWithText("No active card occasions yet", substring = true)
            .assertIsDisplayed()
        composeTestRule.onNodeWithText("No recipients have a postal address.").assertDoesNotExist()
        composeTestRule.onNodeWithTag("occasion-card-list-share").assertDoesNotExist()
    }

    @Test
    fun `while the summary loads a checking line shows and share stays available`() {
        stubSummary(Result.success(summaryOf(1, 1)))
        coEvery { repository.cardListSummary("card") } coAnswers {
            kotlinx.coroutines.awaitCancellation()
        }
        renderActions()
        composeTestRule.onNodeWithText("Checking who is on the list…").assertIsDisplayed()
        composeTestRule.onNodeWithTag("occasion-card-list-share").assertIsDisplayed()
    }

    @Test
    fun `a failed summary is worded from its reason code and retry reloads it`() {
        stubSummary(Result.failure(reasonCoded()))
        renderActions()
        composeTestRule.onNodeWithText("Organisation, place or pet not found").assertIsDisplayed()
        composeTestRule.onNodeWithText("RAW SERVER TEXT").assertDoesNotExist()
        composeTestRule.onNodeWithTag("occasion-card-list-share").assertIsDisplayed()

        coEvery { repository.cardListSummary("card") } returns
            Result.success(summaryOf(2, 2))
        composeTestRule.onNodeWithText("Retry").performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("2 recipients").assertIsDisplayed()
        composeTestRule.onNodeWithText("Organisation, place or pet not found").assertDoesNotExist()
        coVerify(exactly = 2) { repository.cardListSummary("card") }
    }

    @Test
    fun `a v1_6 server is never asked for the summary and keeps the plain share`() {
        renderActions(serverVersion = AppVersion(1, 6, 0))
        composeTestRule.onNodeWithTag("occasion-card-list-share").assertIsDisplayed()
        coVerify(exactly = 0) { repository.cardListSummary(any()) }
    }

    @Test
    fun `a v1_7 server is asked for the summary`() {
        renderActions(serverVersion = AppVersion(1, 7, 0))
        coVerify(exactly = 1) { repository.cardListSummary("card") }
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

    @Test
    fun `a shopping list load failure is worded from its reason code, never the server text`() {
        renderShopping(Result.failure(reasonCoded()))
        composeTestRule.onNodeWithText("Organisation, place or pet not found").assertIsDisplayed()
        composeTestRule.onNodeWithText("RAW SERVER TEXT").assertDoesNotExist()
    }

    @Test
    fun `a card list download failure is worded from its reason code, never the server text`() {
        coEvery { repository.cardListCsv("card") } returns Result.failure(reasonCoded())
        renderActions()
        composeTestRule.onNodeWithText("Share as CSV").performClick()
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithText("Organisation, place or pet not found").assertIsDisplayed()
        composeTestRule.onNodeWithText("RAW SERVER TEXT").assertDoesNotExist()
    }

    private fun reasonCoded() = ApiError.Client(
        409,
        "RAW SERVER TEXT",
        reason = ErrorReason(ErrorReasonCode.ENTITY_NOT_FOUND, emptyMap())
    )

    @Test
    fun `the countdown follows the device date, not the server's days_until`() {
        // Server zone already rolled over: it says 0 ("Today") for a date that is tomorrow here.
        renderShopping(
            Result.success(
                listOf(item("a").copy(date = TODAY.plusDays(1).toString(), daysUntil = 0))
            )
        )
        composeTestRule.onNodeWithText("in 1 day").assertIsDisplayed()
        composeTestRule.onNodeWithText("Today").assertDoesNotExist()
    }
}
