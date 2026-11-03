package com.mycorrhizal.crm.ui.markdown

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** Mirrors `frontend/src/utils/markdown.test.ts` (issue #2134): same inputs, same expected structure. */
class MarkdownParserTest {

    @Test
    fun `sanitizeUrl rejects script, data, relative and file URLs`() {
        listOf(
            "javascript:alert(1)",
            "JaVaScRiPt:alert(1)",
            "java\tscript:alert(1)",
            " \u0001javascript:alert(1)",
            "data:text/html,<script>alert(1)</script>",
            "vbscript:msgbox(1)",
            "//evil.example/x",
            "/relative",
            "file:///etc/passwd",
            ""
        ).forEach { assertNull("should reject <$it>", sanitizeUrl(it)) }
    }

    @Test
    fun `sanitizeUrl allows http https mailto and tel unchanged`() {
        listOf("https://example.com/a?b=1", "http://example.com", "mailto:a@b.co", "tel:+15551234")
            .forEach { assertEquals(it, sanitizeUrl(it)) }
    }

    @Test
    fun `sanitizeUrl strips control characters and line separators before checking`() {
        assertEquals("https://example.com", sanitizeUrl("https://exa\tmple.com"))
        assertNull(sanitizeUrl("java\u2028script:alert(1)"))
        assertNull(sanitizeUrl("java\u2029script:alert(1)"))
        assertNull(sanitizeUrl("java\u0085script:alert(1)"))
    }

    @Test
    fun `plain text and raw HTML stay literal text`() {
        val src = "<img src=x onerror=alert(1)> <script>alert(1)</script> a < b"
        assertEquals(listOf(Inline.Text(src)), parseInlineMarkdown(src))
    }

    @Test
    fun `a javascript URL is not linked and its source stays visible`() {
        val out = parseInlineMarkdown("[x](javascript:alert(1))")
        assertFalse(out.any { it is Inline.Link })
        assertEquals("[x](javascript:alert(1))", out.joinToString("") { (it as Inline.Text).text })
    }

    @Test
    fun `image syntax stays literal text`() {
        assertEquals(
            listOf(Inline.Text("![a](https://e.com/x.png)")),
            parseInlineMarkdown("![a](https://e.com/x.png)")
        )
    }

    @Test
    fun `snake_case and math-like asterisks are not italicised`() {
        assertEquals(
            listOf(Inline.Text("snake_case_word 2 * 3 * 4")),
            parseInlineMarkdown("snake_case_word 2 * 3 * 4")
        )
    }

    @Test
    fun `bold italic code link and escapes parse`() {
        assertEquals(listOf(Inline.Strong(listOf(Inline.Text("b")))), parseInlineMarkdown("**b**"))
        assertEquals(listOf(Inline.Em(listOf(Inline.Text("i")))), parseInlineMarkdown("*i*"))
        assertEquals(listOf(Inline.Em(listOf(Inline.Text("i")))), parseInlineMarkdown("_i_"))
        assertEquals(listOf(Inline.Code("c")), parseInlineMarkdown("`c`"))
        assertEquals(
            listOf(Inline.Link("https://e.com", listOf(Inline.Text("go")))),
            parseInlineMarkdown("[go](https://e.com)")
        )
        assertEquals(listOf(Inline.Text("*not em*")), parseInlineMarkdown("\\*not em\\*"))
    }

    @Test
    fun `an empty link label shows the URL`() {
        assertEquals(
            listOf(Inline.Link("https://e.com", listOf(Inline.Text("https://e.com")))),
            parseInlineMarkdown("[](https://e.com)")
        )
    }

    @Test
    fun `unterminated markers stay as literal text and drop nothing`() {
        listOf(
            "**bold",
            "[a](",
            "[[[[",
            "`x",
            "_".repeat(50),
            "[".repeat(2000)
        ).forEach { src ->
            assertEquals(
                "unterminated <${src.take(20)}> must stay one literal run",
                listOf(Inline.Text(src)),
                parseInlineMarkdown(src)
            )
        }
    }

    @Test
    fun `crossed markers parse only the valid pair and keep the rest literal`() {
        // The leading "*" has no closer without trailing whitespace, so it stays text;
        // the "**...**" pair is real emphasis, and the unmatched "*" inside it stays text.
        assertEquals(
            listOf(Inline.Text("*a "), Inline.Strong(listOf(Inline.Text("b* c")))),
            parseInlineMarkdown("*a **b* c**")
        )
    }

    @Test
    fun `the deepest inline nesting the grammar allows parses completely`() {
        // Greedy first-closer matching means a marker type cannot nest in itself, so the deepest
        // possible tree is link > ** > _ > * (four levels), below MAX_INLINE_DEPTH (6), which is
        // therefore a defence-in-depth bound rather than a reachable limit. Nothing is dropped.
        assertEquals(
            listOf(
                Inline.Link(
                    "https://e.com",
                    listOf(
                        Inline.Strong(
                            listOf(Inline.Em(listOf(Inline.Em(listOf(Inline.Text("x"))))))
                        )
                    )
                )
            ),
            parseInlineMarkdown("[**_*x*_**](https://e.com)")
        )
    }

    @Test
    fun `parses the supported blocks`() {
        val blocks = parseMarkdown("# T\n\n- a\n- b\n\n1. x\n2. y\n\n> q\n\n---\n\n```\ncode\n```")
        assertEquals(
            listOf(
                Block.Heading::class,
                Block.ListBlock::class,
                Block.ListBlock::class,
                Block.Quote::class,
                Block.Rule::class,
                Block.Code::class
            ),
            blocks.map { it::class }
        )
    }

    @Test
    fun `plain text is one paragraph keeping its line breaks and blank-line gaps`() {
        assertEquals(
            listOf(
                Block.Paragraph(listOf("line one", "line two"), gap = 0),
                Block.Paragraph(listOf("after"), gap = 2)
            ),
            parseMarkdown("line one\nline two\n\n\nafter")
        )
    }

    @Test
    fun `an oversized source is a single plain paragraph`() {
        val big = "- x\n".repeat(MARKDOWN_MAX_LENGTH)
        val blocks = parseMarkdown(big)
        assertEquals(1, blocks.size)
        assertTrue(blocks[0] is Block.Paragraph)
    }

    @Test
    fun `nested quotes stop at the depth cap and keep the remaining markers as text`() {
        val src = "> ".repeat(500) + "x"
        var block: Block = parseMarkdown(src).single()
        var quotes = 0
        while (block is Block.Quote) {
            quotes += 1
            block = block.blocks.single()
        }
        assertEquals("five nested quotes (levels 0..MAX_QUOTE_DEPTH)", 5, quotes)
        // Past the cap the rest is one plain paragraph: 500 markers - 5 consumed, none dropped.
        assertEquals(Block.Paragraph(listOf("> ".repeat(495) + "x"), 0), block)
    }

    @Test
    fun `lists record ordering, start and items`() {
        val list = parseMarkdown("3. a\n4. b").single() as Block.ListBlock
        assertTrue(list.ordered)
        assertEquals(3, list.start)
        assertEquals(listOf("a", "b"), list.items)
    }

    @Test
    fun `an unclosed fence runs to the end`() {
        val code = parseMarkdown("```\na\nb").single() as Block.Code
        assertEquals("a\nb", code.text)
    }
}
