package com.mycorrhizal.crm.ui.components

import androidx.compose.ui.graphics.Color
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import com.mycorrhizal.crm.ui.theme.MycorrhizalColors
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
 * Issue #1628: the band -> color/label mapping (#383/ADR-0023) and the
 * deceased-dot color (#1193). Color alone must never carry band meaning, so
 * the word-based label is pinned alongside the tint.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class HealthScoreBadgeTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun `each known band maps to its own theme color and localized label`() {
        // Mirrors the backend's BandMoss/BandChanterelle/BandRussula constants
        // (backend/internal/scoring/scoring.go). This list must stay in sync
        // with the backend band enum.
        val colors = mutableMapOf<String, Color>()
        val labels = mutableMapOf<String, String>()
        composeTestRule.setContent {
            MycorrhizalTheme {
                colors["moss"] = healthBandColor("moss")
                labels["moss"] = healthBandLabel("moss")
                colors["chanterelle"] = healthBandColor("chanterelle")
                labels["chanterelle"] = healthBandLabel("chanterelle")
                colors["russula"] = healthBandColor("russula")
                labels["russula"] = healthBandLabel("russula")
            }
        }

        // Each band gets its own role: tertiary (moss), the warning foreground
        // (chanterelle), error (russula) -- never one shared token.
        assertEquals(MycorrhizalColors.moss, colors["moss"])
        assertEquals(MycorrhizalColors.chanterelleForeground, colors["chanterelle"])
        assertEquals(MycorrhizalColors.russula, colors["russula"])
        assertEquals(3, colors.values.toSet().size)

        assertEquals("Healthy", labels["moss"])
        assertEquals("Needs attention", labels["chanterelle"])
        assertEquals("At risk", labels["russula"])
    }

    @Test
    fun `an unknown band falls back to the muted role and the raw token`() {
        var color: Color? = null
        var label: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                color = healthBandColor("boletus")
                label = healthBandLabel("boletus")
            }
        }

        assertEquals(MycorrhizalColors.soil, color)
        assertEquals("boletus", label)
    }

    @Test
    fun `the badge names the band in words for TalkBack`() {
        composeTestRule.setContent {
            MycorrhizalTheme { HealthScoreBadge(score = 72, band = "moss", onClick = {}) }
        }

        composeTestRule
            .onNodeWithContentDescription("Relationship health score 72 out of 100: Healthy")
            .assertIsDisplayed()
    }

    @Test
    fun `an unknown band badge exposes the raw token as its TalkBack name`() {
        composeTestRule.setContent {
            MycorrhizalTheme { HealthScoreBadge(score = 7, band = "boletus", onClick = {}) }
        }

        composeTestRule
            .onNodeWithContentDescription("Relationship health score 7 out of 100: boletus")
            .assertIsDisplayed()
    }

    @Test
    fun `the deceased dot uses the muted deceased color, never a band color`() {
        var deceased: Color? = null
        val bandColors = mutableListOf<Color>()
        composeTestRule.setContent {
            MycorrhizalTheme {
                deceased = deceasedColor()
                bandColors += healthBandColor("moss")
                bandColors += healthBandColor("chanterelle")
                bandColors += healthBandColor("russula")
            }
        }

        // Issue #1193: DeceasedDot's background is deceasedColor(), a muted
        // "present but quiet" role -- not a health verdict.
        assertEquals(MycorrhizalColors.soil, deceased)
        for (band in bandColors) {
            assertNotEquals(deceased, band)
        }
    }
}
