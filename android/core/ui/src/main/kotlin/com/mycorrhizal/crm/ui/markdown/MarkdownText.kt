package com.mycorrhizal.crm.ui.markdown

import androidx.compose.material3.LocalContentColor
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.isSpecified
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.LinkAnnotation
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextLinkStyles
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.text.withLink
import androidx.compose.ui.text.withStyle

/**
 * Issue #2134: renders the stored Markdown source of a note or activity description.
 *
 * One [Text] over a single [AnnotatedString], so a list row can still clamp it with [maxLines]
 * and an ellipsis exactly as the plain text it replaces. Links are [LinkAnnotation.Url]: the
 * system opens them and TalkBack announces the label. The source is never modified.
 */
@Composable
fun MarkdownText(
    source: String,
    modifier: Modifier = Modifier,
    style: TextStyle = MaterialTheme.typography.bodyLarge,
    color: Color = Color.Unspecified,
    maxLines: Int = Int.MAX_VALUE,
    overflow: TextOverflow = TextOverflow.Clip
) {
    val linkColor = MaterialTheme.colorScheme.primary
    val codeBackground = MaterialTheme.colorScheme.surfaceVariant
    val annotated = remember(source, linkColor, codeBackground) {
        markdownAnnotatedString(source, linkColor, codeBackground)
    }
    Text(
        text = annotated,
        modifier = modifier,
        style = style,
        color = if (color.isSpecified) color else LocalContentColor.current,
        maxLines = maxLines,
        overflow = overflow
    )
}

private const val RULE_TEXT = "────────"
private const val QUOTE_PREFIX = "│ "

/** Pure builder (no composition needed) so the rendering is unit-testable. */
fun markdownAnnotatedString(
    source: String,
    linkColor: Color = Color.Unspecified,
    codeBackground: Color = Color.Unspecified
): AnnotatedString = buildAnnotatedString {
    appendBlocks(parseMarkdown(source), "", linkColor, codeBackground)
}

private fun AnnotatedString.Builder.appendBlocks(
    blocks: List<Block>,
    prefix: String,
    linkColor: Color,
    codeBackground: Color,
    leadPrefix: Boolean = true
) {
    blocks.forEachIndexed { index, block ->
        if (index > 0) {
            // A gap of N is N blank lines: N+1 newlines, each followed by the quote prefix.
            repeat(block.gap + 1) {
                append('\n')
                append(prefix)
            }
        } else if (leadPrefix) {
            append(prefix)
        }
        when (block) {
            is Block.Paragraph -> block.lines.forEachIndexed { n, line ->
                if (n > 0) {
                    append('\n')
                    append(prefix)
                }
                appendInlines(parseInlineMarkdown(line), linkColor, codeBackground)
            }

            is Block.Heading -> withStyle(
                SpanStyle(fontWeight = FontWeight.Bold)
            ) { appendInlines(parseInlineMarkdown(block.text), linkColor, codeBackground) }

            is Block.ListBlock -> block.items.forEachIndexed { n, item ->
                if (n > 0) {
                    append('\n')
                    append(prefix)
                }
                append(if (block.ordered) "${block.start + n}. " else "• ")
                item.split('\n').forEachIndexed { k, line ->
                    if (k > 0) {
                        append('\n')
                        append(prefix)
                        append("   ")
                    }
                    appendInlines(parseInlineMarkdown(line), linkColor, codeBackground)
                }
            }

            is Block.Quote -> {
                append(QUOTE_PREFIX)
                appendBlocks(
                    block.blocks,
                    prefix + QUOTE_PREFIX,
                    linkColor,
                    codeBackground,
                    leadPrefix = false
                )
            }

            is Block.Rule -> append(RULE_TEXT)

            is Block.Code -> withStyle(
                SpanStyle(fontFamily = FontFamily.Monospace, background = codeBackground)
            ) {
                block.text.split('\n').forEachIndexed { n, line ->
                    if (n > 0) {
                        append('\n')
                        append(prefix)
                    }
                    append(line)
                }
            }
        }
    }
}

private fun AnnotatedString.Builder.appendInlines(
    inlines: List<Inline>,
    linkColor: Color,
    codeBackground: Color
) {
    inlines.forEach { inline ->
        when (inline) {
            is Inline.Text -> append(inline.text)

            is Inline.Code -> withStyle(
                SpanStyle(fontFamily = FontFamily.Monospace, background = codeBackground)
            ) { append(inline.text) }

            is Inline.Strong -> withStyle(SpanStyle(fontWeight = FontWeight.Bold)) {
                appendInlines(inline.children, linkColor, codeBackground)
            }

            is Inline.Em -> withStyle(SpanStyle(fontStyle = FontStyle.Italic)) {
                appendInlines(inline.children, linkColor, codeBackground)
            }

            is Inline.Link -> withLink(
                LinkAnnotation.Url(
                    url = inline.href,
                    styles = TextLinkStyles(
                        SpanStyle(color = linkColor, textDecoration = TextDecoration.Underline)
                    )
                )
            ) { appendInlines(inline.children, linkColor, codeBackground) }
        }
    }
}
