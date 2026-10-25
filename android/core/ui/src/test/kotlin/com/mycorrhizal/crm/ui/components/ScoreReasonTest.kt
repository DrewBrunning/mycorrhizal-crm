package com.mycorrhizal.crm.ui.components

import androidx.compose.material3.Text
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import com.mycorrhizal.crm.model.network.ScoreReasonCode
import com.mycorrhizal.crm.model.network.ScoreReasonParams
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * v1.5.0 review: the score breakdown renders its explanations from the
 * server's reason_code + reason_params in the UI language (it used to show the
 * English `reason`, "… 3 day(s) ago", in every locale).
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ScoreReasonTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun render(code: String?, params: ScoreReasonParams = ScoreReasonParams()): String {
        var text = ""
        composeTestRule.setContent {
            MycorrhizalTheme {
                text = scoreReasonText(code, params, FALLBACK)
                Text(text)
            }
        }
        composeTestRule.waitForIdle()
        return text
    }

    @Test
    fun `recency uses real plurals for both numbers`() {
        assertEquals(
            "Last qualifying interaction 1 day ago (target: every day)",
            render(ScoreReasonCode.RECENCY_DAYS, ScoreReasonParams(days = 1, intervalDays = 1))
        )
    }

    @Test
    fun `recency plural other`() {
        assertEquals(
            "Last qualifying interaction 5 days ago (target: every 14 days)",
            render(ScoreReasonCode.RECENCY_DAYS, ScoreReasonParams(days = 5, intervalDays = 14))
        )
    }

    @Test
    fun `frequency interpolates the count, window and rounded expectation`() {
        assertEquals(
            "2 qualifying interactions in the last 180 days (about 12.9 expected at this cadence)",
            render(
                ScoreReasonCode.FREQUENCY_COUNT,
                ScoreReasonParams(count = 2, windowDays = 180, expected = 12.857)
            )
        )
    }

    @Test
    fun `closeness uses the translated relation and hop plurals`() {
        assertEquals(
            "Directly connected as Spouse",
            render(ScoreReasonCode.CLOSENESS_DIRECT, ScoreReasonParams(relation = "spouse_of"))
        )
    }

    @Test
    fun `hops plural one`() {
        assertEquals(
            "Reachable 1 hop away in your relationship graph",
            render(ScoreReasonCode.CLOSENESS_HOPS, ScoreReasonParams(hops = 1))
        )
    }

    @Test
    fun `target tier names the translated tier and interval`() {
        assertEquals(
            "Recency and frequency score 30 against the Casual target (every 45 days)",
            render(
                ScoreReasonCode.TARGET_TIER,
                ScoreReasonParams(score = 30, intervalDays = 45, tier = "casual")
            )
        )
    }

    @Test
    fun `an unknown code, a missing param and an older server fall back to the English reason`() {
        assertEquals(FALLBACK, render("some_future_code"))
    }

    @Test
    fun `a missing param falls back`() {
        assertEquals(FALLBACK, render(ScoreReasonCode.RECENCY_DAYS, ScoreReasonParams(days = 3)))
    }

    @Test
    fun `a server without reason codes falls back`() {
        assertEquals(FALLBACK, render(null))
    }

    @Test
    fun `every mirrored code renders without falling back`() {
        val all = ScoreReasonParams(
            days = 2, intervalDays = 14, hops = 2, count = 2, windowDays = 180,
            expected = 3.0, score = 50, relation = "friend_of", tier = "close"
        )
        val rendered = mutableMapOf<String, String>()
        composeTestRule.setContent {
            MycorrhizalTheme {
                ScoreReasonCode.ALL.forEach { code ->
                    rendered[code] = scoreReasonText(
                        code,
                        all,
                        FALLBACK
                    )
                }
            }
        }
        composeTestRule.waitForIdle()
        ScoreReasonCode.ALL.forEach { code -> assertNotEquals(code, FALLBACK, rendered[code]) }
    }

    @Test
    fun `a parameterised code falls back when any of its params is missing`() {
        // No params at all, then each multi-param code with only one of its
        // params set: neither may render a half-filled template.
        val partial = ScoreReasonParams(days = 2, score = 50, count = 2)
        val noParams = listOf(
            ScoreReasonCode.CLOSENESS_DIRECT,
            ScoreReasonCode.CLOSENESS_HOPS,
            ScoreReasonCode.RECENCY_DAYS,
            ScoreReasonCode.FREQUENCY_COUNT,
            ScoreReasonCode.LAST_UPDATED_DAYS,
            ScoreReasonCode.TARGET_CADENCE_POLICY,
            ScoreReasonCode.TARGET_TIER
        )
        val partialParams = listOf(
            ScoreReasonCode.RECENCY_DAYS,
            ScoreReasonCode.FREQUENCY_COUNT,
            ScoreReasonCode.TARGET_CADENCE_POLICY,
            ScoreReasonCode.TARGET_TIER
        )
        val rendered = mutableListOf<Pair<String, String>>()
        composeTestRule.setContent {
            MycorrhizalTheme {
                rendered.clear()
                noParams.forEach {
                    rendered += it to scoreReasonText(
                        it,
                        ScoreReasonParams(),
                        FALLBACK
                    )
                }
                partialParams.forEach {
                    rendered += "$it (partial)" to scoreReasonText(
                        it,
                        partial,
                        FALLBACK
                    )
                }
            }
        }
        composeTestRule.waitForIdle()
        assertEquals(noParams.size + partialParams.size, rendered.size)
        rendered.forEach { (label, text) -> assertEquals(label, FALLBACK, text) }
    }

    @Test
    @Config(qualifiers = "de")
    fun `renders in the UI language with its plurals`() {
        assertEquals(
            "Letzte zählende Interaktion vor 1 Tag (Ziel: alle 30 Tage)",
            render(ScoreReasonCode.RECENCY_DAYS, ScoreReasonParams(days = 1, intervalDays = 30))
        )
        composeTestRule.onNodeWithText(
            "Letzte zählende Interaktion vor 1 Tag (Ziel: alle 30 Tage)"
        ).assertExists()
    }

    private companion object {
        const val FALLBACK = "ENGLISH FALLBACK"
    }
}
