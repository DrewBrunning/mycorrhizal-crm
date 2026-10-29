package com.mycorrhizal.crm.ui.markdown

import java.net.URI
import java.net.URISyntaxException

// Small, deliberate CommonMark subset for notes and activity descriptions (issue #2134).
//
// A hand-written mirror of `frontend/src/utils/markdown.ts` (issue #2005): the same grammar,
// the same limits, the same expected trees (`MarkdownParserTest` mirrors `markdown.test.ts`).
// Keep the two in lockstep; a change to one is a change to both.
//
// This is a parser, not a renderer: it returns a plain tree that `MarkdownText` turns into an
// AnnotatedString. Nothing here produces HTML, so raw HTML in the source is just text. The
// stored source is never modified.
//
// Blocks: ATX headings, bullet/ordered lists (flat), blockquotes, horizontal rules, fenced code,
// paragraphs. Inline: `code`, **bold**, *italic* / _italic_, [text](url), backslash escapes.
// Images are NOT rendered (their source stays visible); bare URLs are not auto-linked, so
// existing plain text is unchanged.

sealed interface Inline {
    data class Text(val text: String) : Inline
    data class Code(val text: String) : Inline
    data class Strong(val children: List<Inline>) : Inline
    data class Em(val children: List<Inline>) : Inline
    data class Link(val href: String, val children: List<Inline>) : Inline
}

/** [gap] is the number of blank lines before the block (min 1 after the first block). */
sealed interface Block {
    val gap: Int

    data class Paragraph(val lines: List<String>, override val gap: Int = 0) : Block
    data class Heading(val level: Int, val text: String, override val gap: Int = 0) : Block
    data class ListBlock(
        val ordered: Boolean,
        val start: Int,
        val items: List<String>,
        override val gap: Int = 0
    ) : Block
    data class Quote(val blocks: List<Block>, override val gap: Int = 0) : Block
    data class Rule(override val gap: Int = 0) : Block
    data class Code(val text: String, override val gap: Int = 0) : Block
}

/** Past this size the source is one plain paragraph: bounds worst-case inline scanning. */
const val MARKDOWN_MAX_LENGTH = 50_000
private const val MAX_INLINE_DEPTH = 6
private const val MAX_QUOTE_DEPTH = 4

private val ALLOWED_SCHEMES = setOf("http", "https", "mailto", "tel")

/**
 * A safe href, or null when the URL is not allowed. Only http(s), mailto and tel pass;
 * javascript:, data:, vbscript:, relative and protocol-relative URLs are rejected. Platforms
 * ignore ASCII control characters and whitespace inside a scheme (`java\tscript:`), so they are
 * stripped before the check and the stripped form is what is returned.
 */
fun sanitizeUrl(raw: String): String? {
    val cleaned = buildString {
        raw.codePoints().forEach { code ->
            val strip = code <= 0x20 || code in 0x7f..0x9f || code == 0x2028 || code == 0x2029
            if (!strip) appendCodePoint(code)
        }
    }
    if (cleaned.isEmpty()) return null
    val uri = try {
        URI(cleaned)
    } catch (_: URISyntaxException) {
        return null
    }
    val scheme = uri.scheme?.lowercase() ?: return null
    if (scheme !in ALLOWED_SCHEMES) return null
    if ((scheme == "http" || scheme == "https") && uri.host.isNullOrEmpty()) return null
    return cleaned
}

private const val ESCAPABLE = "\\`*_{}[]()#+-.!>~|<"
private val IMAGE = Regex("^!\\[[^\\]]*]\\([^)]*\\)")

private fun parseInline(src: String, depth: Int = 0): List<Inline> {
    val out = mutableListOf<Inline>()
    val buf = StringBuilder()
    fun flush() {
        if (buf.isNotEmpty()) {
            out += Inline.Text(buf.toString())
            buf.clear()
        }
    }
    var i = 0
    while (i < src.length) {
        val ch = src[i]

        if (ch == '\\' && i + 1 < src.length && ESCAPABLE.indexOf(src[i + 1]) >= 0) {
            buf.append(src[i + 1])
            i += 2
            continue
        }

        if (ch == '`') {
            val end = src.indexOf('`', i + 1)
            if (end > i + 1) {
                flush()
                out += Inline.Code(src.substring(i + 1, end))
                i = end + 1
                continue
            }
        }

        // Image syntax is kept as literal text (no images from notes).
        if (ch == '!' && src.getOrNull(i + 1) == '[') {
            val m = IMAGE.find(src.substring(i))
            if (m != null) {
                buf.append(m.value)
                i += m.value.length
                continue
            }
        }

        if (ch == '[' && depth < MAX_INLINE_DEPTH) {
            val close = src.indexOf(']', i + 1)
            if (close > i && src.getOrNull(close + 1) == '(') {
                val paren = src.indexOf(')', close + 2)
                if (paren > close) {
                    val label = src.substring(i + 1, close)
                    // Drop an optional "title" after the URL.
                    val target = src.substring(close + 2, paren).trim().split(Regex("\\s+")).first()
                    val href = sanitizeUrl(target)
                    if (href != null) {
                        flush()
                        out += Inline.Link(
                            href,
                            if (label.isNotEmpty()) {
                                parseInline(
                                    label,
                                    depth + 1
                                )
                            } else {
                                listOf(Inline.Text(href))
                            }
                        )
                        i = paren + 1
                        continue
                    }
                }
            }
        }

        if ((ch == '*' || ch == '_') && depth < MAX_INLINE_DEPTH) {
            val double = src.getOrNull(i + 1) == ch
            val marker = if (double) "$ch$ch" else "$ch"
            val intraword =
                ch == '_' && i > 0 &&
                    src[i - 1].let { it in 'A'..'Z' || it in 'a'..'z' || it in '0'..'9' }
            val start = i + marker.length
            val end = if (intraword) -1 else src.indexOf(marker, start)
            if (end > start && !src[start].isWhitespace() && !src[end - 1].isWhitespace()) {
                flush()
                val children = parseInline(src.substring(start, end), depth + 1)
                out += if (double) Inline.Strong(children) else Inline.Em(children)
                i = end + marker.length
                continue
            }
        }

        buf.append(ch)
        i += 1
    }
    flush()
    return out
}

fun parseInlineMarkdown(src: String): List<Inline> = parseInline(src)

private val FENCE = Regex("^ {0,3}(```|~~~)")
private val HEADING = Regex("^ {0,3}(#{1,6})[ \\t]+(.*)$")
private val RULE_CANDIDATE = Regex("^ {0,3}[-*_][-*_ \\t]*$")
private val BULLET = Regex("^ {0,3}[-*+][ \\t]+(.*)$")
private val ORDERED = Regex("^ {0,3}(\\d{1,9})[.)][ \\t]+(.*)$")
private val QUOTE = Regex("^ {0,3}>[ \\t]?(.*)$")
private val HEADING_TRAILER = Regex("[ \\t]+#+[ \\t]*$")
private val CONTINUATION = Regex("^ {2,}\\S")

private fun isRule(line: String): Boolean {
    if (!RULE_CANDIDATE.matches(line)) return false
    val marks = line.replace(" ", "").replace("\t", "")
    return marks.length >= 3 && marks.all { it == marks[0] }
}

private fun startsBlock(line: String): Boolean =
    FENCE.containsMatchIn(line) || HEADING.matches(line) || isRule(line) ||
        BULLET.matches(line) || ORDERED.matches(line) || QUOTE.matches(line)

private fun parseBlocks(lines: List<String>, quoteDepth: Int): List<Block> {
    val blocks = mutableListOf<Block>()
    var blanks = 0
    fun gapNow() = if (blocks.isEmpty()) 0 else maxOf(blanks, 1)

    var i = 0
    while (i < lines.size) {
        val line = lines[i]
        if (line.isBlank()) {
            blanks += 1
            i += 1
            continue
        }

        val fence = FENCE.find(line)
        if (fence != null) {
            val marker = fence.groupValues[1]
            val body = mutableListOf<String>()
            val gap = gapNow()
            i += 1
            while (i < lines.size && !lines[i].trimStart().startsWith(marker)) {
                body += lines[i]
                i += 1
            }
            i += 1 // closing fence (or EOF: an unclosed fence runs to the end)
            blocks += Block.Code(body.joinToString("\n"), gap)
            blanks = 0
            continue
        }

        val heading = HEADING.matchEntire(line)
        if (heading != null) {
            blocks += Block.Heading(
                level = heading.groupValues[1].length,
                text = heading.groupValues[2].replace(HEADING_TRAILER, "").trim(),
                gap = gapNow()
            )
            blanks = 0
            i += 1
            continue
        }

        if (isRule(line)) {
            blocks += Block.Rule(gapNow())
            blanks = 0
            i += 1
            continue
        }

        if (QUOTE.matches(line)) {
            val gap = gapNow()
            val inner = mutableListOf<String>()
            while (i < lines.size && QUOTE.matches(lines[i])) {
                inner += QUOTE.matchEntire(lines[i])!!.groupValues[1]
                i += 1
            }
            blocks += Block.Quote(
                if (quoteDepth >= MAX_QUOTE_DEPTH) {
                    listOf(Block.Paragraph(inner, 0))
                } else {
                    parseBlocks(inner, quoteDepth + 1)
                },
                gap
            )
            blanks = 0
            continue
        }

        val bullet = BULLET.matchEntire(line)
        val ordered = ORDERED.matchEntire(line)
        if (bullet != null || ordered != null) {
            val gap = gapNow()
            val isOrdered = bullet == null
            val items = mutableListOf<String>()
            val start = ordered?.groupValues?.get(1)?.toIntOrNull() ?: 1
            while (i < lines.size) {
                val m = (if (isOrdered) ORDERED else BULLET).matchEntire(lines[i])
                // A rule like "* * *" matches BULLET; it ends the list.
                if (m == null || isRule(lines[i])) break
                val sb = StringBuilder(if (isOrdered) m.groupValues[2] else m.groupValues[1])
                i += 1
                // Indented continuation lines belong to the item.
                while (i < lines.size && CONTINUATION.containsMatchIn(lines[i]) &&
                    !startsBlock(lines[i].trimStart())
                ) {
                    sb.append('\n').append(lines[i].trim())
                    i += 1
                }
                items += sb.toString()
            }
            blocks += Block.ListBlock(isOrdered, start, items, gap)
            blanks = 0
            continue
        }

        val gap = gapNow()
        val para = mutableListOf(line)
        i += 1
        while (i < lines.size && lines[i].isNotBlank() && !startsBlock(lines[i])) {
            para += lines[i]
            i += 1
        }
        blocks += Block.Paragraph(para, gap)
        blanks = 0
    }
    return blocks
}

fun parseMarkdown(source: String): List<Block> {
    val normalized = source.replace(Regex("\r\n?"), "\n")
    if (normalized.length > MARKDOWN_MAX_LENGTH) {
        return listOf(Block.Paragraph(listOf(normalized), 0))
    }
    return parseBlocks(normalized.split('\n'), 0)
}
