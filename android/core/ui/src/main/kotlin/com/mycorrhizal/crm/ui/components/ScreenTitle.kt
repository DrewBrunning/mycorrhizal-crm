package com.mycorrhizal.crm.ui.components

import androidx.compose.foundation.focusable
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.runtime.withFrameNanos
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.style.TextAlign

/**
 * Issue #1922: a screen's title, carrying heading semantics and moving focus to
 * itself when the screen composes.
 *
 * Compose's focus restoration on navigation otherwise lands on the first
 * focusable node in the tree — the top bar's Back/Menu button — so TalkBack
 * announces "Back. Button" and never the screen (or, for a contact, which
 * contact). Requesting focus on the title after the first frame makes the
 * screen announce itself on route and deep-link entry.
 *
 * The wait for the next frame is deliberate: the focus node is attached during
 * layout, which can happen after the composition effect would otherwise run.
 */
@Composable
fun ScreenTitle(
    text: String,
    modifier: Modifier = Modifier,
    style: TextStyle = MaterialTheme.typography.titleLarge,
    color: Color = Color.Unspecified,
    textAlign: TextAlign? = null,
) {
    val focusRequester = remember { FocusRequester() }
    LaunchedEffect(Unit) {
        withFrameNanos { }
        focusRequester.requestFocus()
    }
    Text(
        text = text,
        modifier = modifier
            .focusRequester(focusRequester)
            .focusable()
            .semantics { heading() },
        style = style,
        color = color,
        textAlign = textAlign,
    )
}
