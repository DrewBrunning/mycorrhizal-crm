package com.mycorrhizal.crm.testing

import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.SemanticsNodeInteraction
import androidx.compose.ui.text.TextLayoutResult
import androidx.compose.ui.unit.Density

/** Renders [content] with the system font size scaled by [fontScale] (2.0 = the 200% setting). */
@Composable
fun WithFontScale(fontScale: Float, content: @Composable () -> Unit) {
    val density = LocalDensity.current
    CompositionLocalProvider(LocalDensity provides Density(density.density, fontScale)) {
        content()
    }
}

/**
 * Asserts the node's text never wraps inside a word: every line but the last must end at
 * whitespace (#2948: "Organisa/tions", "Discar/d new"). The node must be a text node, so find
 * it with `useUnmergedTree = true`.
 */
fun SemanticsNodeInteraction.assertNoMidWordBreak(): SemanticsNodeInteraction {
    val node = fetchSemanticsNode()
    val results = mutableListOf<TextLayoutResult>()
    val getLayout = node.config.getOrNull(SemanticsActions.GetTextLayoutResult)?.action
    check(getLayout != null && getLayout(results) && results.isNotEmpty()) {
        "Node has no text layout; query a text node with useUnmergedTree = true"
    }
    val layout = results.first()
    val text = layout.layoutInput.text.text
    for (line in 0 until layout.lineCount - 1) {
        val end = layout.getLineEnd(line)
        check(end > 0 && text[end - 1].isWhitespace()) {
            "\"$text\" breaks inside a word at line ${line + 1} (…${text.substring(
                0,
                end
            ).takeLast(8)}|…)"
        }
    }
    return this
}
