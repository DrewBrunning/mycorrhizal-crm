package com.mycorrhizal.crm.ui.theme

import androidx.compose.ui.graphics.Color
import kotlin.math.pow
import kotlin.math.sqrt
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * #2941: the share status chips (Active / Declined / Pending / Expired, `GrantStatusChip`) draw on
 * `tertiaryContainer`, `errorContainer`, `secondaryContainer` and `surfaceVariant`. In the light
 * theme two pairs were near-identical (the Material baseline pinks; secondaryContainer equal to
 * surfaceVariant), so different states looked the same. They must differ pairwise by at least
 * ΔE 10 (CIE76), and the chip text must keep 4.5:1 in both themes.
 */
class StatusChipColorsTest {

    private data class Chip(val name: String, val container: Color, val onContainer: Color)

    private fun chips(scheme: androidx.compose.material3.ColorScheme) = listOf(
        Chip("active", scheme.tertiaryContainer, scheme.onTertiaryContainer),
        Chip("declined", scheme.errorContainer, scheme.onErrorContainer),
        Chip("pending", scheme.secondaryContainer, scheme.onSecondaryContainer),
        Chip("expired", scheme.surfaceVariant, scheme.onSurfaceVariant)
    )

    private fun lin(c: Float): Double {
        val v = c.toDouble()
        return if (v <= 0.04045) v / 12.92 else ((v + 0.055) / 1.055).pow(2.4)
    }

    private fun luminance(c: Color) = 0.2126 * lin(
        c.red
    ) + 0.7152 * lin(c.green) + 0.0722 * lin(c.blue)

    private fun contrast(a: Color, b: Color): Double {
        val l1 = luminance(a)
        val l2 = luminance(b)
        return (maxOf(l1, l2) + 0.05) / (minOf(l1, l2) + 0.05)
    }

    private fun lab(c: Color): Triple<Double, Double, Double> {
        val r = lin(c.red)
        val g = lin(c.green)
        val b = lin(c.blue)
        val x = (0.4124 * r + 0.3576 * g + 0.1805 * b) / 0.95047
        val y = 0.2126 * r + 0.7152 * g + 0.0722 * b
        val z = (0.0193 * r + 0.1192 * g + 0.9505 * b) / 1.08883
        fun f(t: Double) = if (t > 0.008856) t.pow(1.0 / 3) else 7.787 * t + 16.0 / 116
        return Triple(116 * f(y) - 16, 500 * (f(x) - f(y)), 200 * (f(y) - f(z)))
    }

    private fun deltaE(a: Color, b: Color): Double {
        val (l1, a1, b1) = lab(a)
        val (l2, a2, b2) = lab(b)
        return sqrt((l1 - l2).pow(2) + (a1 - a2).pow(2) + (b1 - b2).pow(2))
    }

    @Test
    fun `light theme chip backgrounds differ pairwise by at least delta E 10`() {
        val chips = chips(LightColors)
        for (i in chips.indices) {
            for (j in i + 1 until chips.size) {
                val d = deltaE(chips[i].container, chips[j].container)
                assertTrue(
                    "${chips[i].name} vs ${chips[j].name}: ΔE ${"%.1f".format(d)} < 10",
                    d >= 10.0
                )
            }
        }
    }

    @Test
    fun `chip text keeps 4_5 to 1 contrast on its background in both themes`() {
        for ((theme, scheme) in listOf("light" to LightColors, "dark" to DarkColors)) {
            for (chip in chips(scheme)) {
                val ratio = contrast(chip.onContainer, chip.container)
                assertTrue(
                    "$theme ${chip.name}: ${"%.2f".format(ratio)}:1 < 4.5:1",
                    ratio >= 4.5
                )
            }
        }
    }
}
