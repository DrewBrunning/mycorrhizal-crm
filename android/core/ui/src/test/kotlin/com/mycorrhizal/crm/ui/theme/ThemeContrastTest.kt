package com.mycorrhizal.crm.ui.theme

import androidx.compose.ui.graphics.Color
import org.junit.Assert.assertTrue
import org.junit.Test
import kotlin.math.pow

/**
 * Issue #206: WCAG 1.4.3 (Minimum, AA) contrast over the actual token/surface
 * pairings the app renders — not just the palette in isolation. The three-tier
 * bone/parchment/paper layering (Theme.kt) puts the accent tokens on a
 * *different* ground than the web's verification covered, which is exactly how
 * moss #0D844C and laccaria #7B6B98 shipped at 3.87:1 on the audit badges.
 *
 * This is the second contrast pass the palette has needed (issue #200 was the
 * first), so the pairings are pinned here. Plain JUnit — the WCAG formula only
 * needs the hex literals.
 */
class ThemeContrastTest {

    private fun luminance(c: Color): Double {
        fun lin(channel: Float): Double {
            val v = channel.toDouble()
            return if (v <= 0.04045) v / 12.92 else ((v + 0.055) / 1.055).pow(2.4)
        }
        return 0.2126 * lin(c.red) + 0.7152 * lin(c.green) + 0.0722 * lin(c.blue)
    }

    private fun contrastRatio(fg: Color, bg: Color): Double {
        val l1 = luminance(fg)
        val l2 = luminance(bg)
        val lighter = maxOf(l1, l2)
        val darker = minOf(l1, l2)
        return (lighter + 0.05) / (darker + 0.05)
    }

    private fun assertContrast(fg: Color, bg: Color, minimum: Double, label: String) {
        val ratio = contrastRatio(fg, bg)
        assertTrue(
            "$label: contrast ${"%.2f".format(ratio)}:1 is below the " +
                "$minimum:1 minimum on ${bg}",
            ratio >= minimum,
        )
    }

    // The app's three light grounds (Theme.kt: bone page / parchment cards /
    // paper dialogs). Cards render surfaceContainerLow; dialogs render
    // surfaceContainerHigh.
    private val page = LightColors.surface          // bone
    private val card = LightColors.surfaceContainerLow   // parchment
    private val dialog = LightColors.surfaceContainerHigh // paper

    private val darkPage = DarkColors.surface            // boneDark
    private val darkCard = DarkColors.surfaceContainerLow   // parchmentDark
    private val darkDialog = DarkColors.surfaceContainerHigh // paperDark

    @Test
    fun `accent tokens clear AA as text on the surfaces they are drawn on (light)`() {
        // OperationBadge labels and the #200 overdue text now use onSurface.
        assertContrast(LightColors.onSurface, card, 4.5, "onSurface on cards")
        assertContrast(LightColors.onSurface, dialog, 4.5, "onSurface in dialogs")
        // Link/section-header text in primary.
        assertContrast(LightColors.primary, page, 4.5, "primary on the page")
        assertContrast(LightColors.primary, card, 4.5, "primary on cards")
        // Success/"on track" text in tertiary — #206 darkened moss for exactly
        // this pairing (was 3.87:1).
        assertContrast(LightColors.tertiary, page, 4.5, "tertiary on the page")
        assertContrast(LightColors.tertiary, card, 4.5, "tertiary on cards")
        // Error text in the error role.
        assertContrast(LightColors.error, page, 4.5, "error on the page")
        // Info token (no M3 slot; used directly) — #206 darkened laccaria.
        assertContrast(MycorrhizalColors.laccaria, card, 4.5, "laccaria on cards")
        // Secondary text.
        assertContrast(LightColors.onSurfaceVariant, page, 4.5, "onSurfaceVariant on the page")
        assertContrast(LightColors.onSurfaceVariant, card, 4.5, "onSurfaceVariant on cards")
    }

    @Test
    fun `accent tokens clear AA as text on the surfaces they are drawn on (dark)`() {
        assertContrast(DarkColors.onSurface, darkCard, 4.5, "onSurface on cards")
        assertContrast(DarkColors.onSurface, darkDialog, 4.5, "onSurface in dialogs")
        assertContrast(DarkColors.primary, darkPage, 4.5, "primary on the page")
        assertContrast(DarkColors.onSurfaceVariant, darkCard, 4.5, "onSurfaceVariant on cards")
        // #206 lightened mossDark and russulaDark for dark dialogs
        // (surfaceContainerHigh) — the worst pairings in the app.
        assertContrast(DarkColors.tertiary, darkDialog, 4.5, "tertiary in dialogs")
        assertContrast(DarkColors.error, darkDialog, 4.5, "error in dialogs")
    }

    @Test
    fun `non-text accent pairings clear 3 to 1 (borders and icon tints)`() {
        // #200: the warning foreground (overdue borders/icon tints) is the
        // darkened amber, not the chip's container amber.
        assertContrast(MycorrhizalColors.chanterelleForeground, page, 3.0, "warning border on the page")
        assertContrast(MycorrhizalColors.chanterelleForeground, card, 3.0, "warning border on cards")
        assertContrast(MycorrhizalColors.chanterelleDark, darkCard, 3.0, "warning border on dark cards")
        // #206: OperationBadge's 1dp border is the accent at full opacity — a
        // non-text 3:1 bar on the card ground in both themes.
        assertContrast(LightColors.tertiary, card, 3.0, "badge border (tertiary) on cards")
        assertContrast(LightColors.primary, card, 3.0, "badge border (primary) on cards")
        assertContrast(LightColors.error, card, 3.0, "badge border (error) on cards")
        assertContrast(DarkColors.tertiary, darkCard, 3.0, "badge border (tertiary) on dark cards")
        assertContrast(DarkColors.primary, darkCard, 3.0, "badge border (primary) on dark cards")
        assertContrast(DarkColors.error, darkCard, 3.0, "badge border (error) on dark cards")
    }
}
