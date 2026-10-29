package com.mycorrhizal.crm.ui.markdown

import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.text.LinkAnnotation
import androidx.compose.ui.text.font.FontWeight
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/** Issue #2134: rendering of the Markdown subset and the plain-text-unchanged guarantee. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class MarkdownTextTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun render(source: String) = markdownAnnotatedString(source)

    @Test
    fun `the composable shows the same text it builds`() {
        val source = "# Hi\n\n- a\n\nplain\n\n\nend"
        composeTestRule.setContent {
            MycorrhizalTheme { MarkdownText(source = source, modifier = Modifier.testTag("md")) }
        }
        val shown = composeTestRule.onNodeWithTag("md").fetchSemanticsNode()
            .config.getOrNull(SemanticsProperties.Text)?.joinToString("") { it.text }
        assertEquals(render(source).text, shown)
    }

    @Test
    fun `plain text renders exactly as stored, including single breaks and blank-line gaps`() {
        val source = "line one\nline two\n\n\nafter"
        assertEquals(source, render(source).text)
    }

    @Test
    fun `bold italic and code carry styles`() {
        val annotated = render("**b** *i* `c`")
        assertEquals("b i c", annotated.text)
        assertTrue(annotated.spanStyles.any { it.item.fontWeight == FontWeight.Bold })
        assertTrue(annotated.spanStyles.any { it.item.fontStyle != null })
        assertTrue(annotated.spanStyles.any { it.item.fontFamily != null })
    }

    @Test
    fun `lists headings quotes and rules are drawn as text`() {
        val text = render("# Title\n\n- a\n- b\n\n1. x\n\n> q\n\n---").text
        assertTrue(text.contains("Title"))
        assertTrue(text.contains("• a"))
        assertTrue(text.contains("• b"))
        assertTrue(text.contains("1. x"))
        assertTrue(text.contains("q"))
    }

    @Test
    fun `a link exposes its label as the accessible name and its URL`() {
        val annotated = render("see [the docs](https://example.com/docs) now")
        val link = annotated.getLinkAnnotations(0, annotated.length).single()
        assertEquals("https://example.com/docs", (link.item as LinkAnnotation.Url).url)
        assertEquals("the docs", annotated.text.substring(link.start, link.end))
    }

    @Test
    fun `an empty link label falls back to the URL as the accessible name`() {
        val annotated = render("[](https://example.com)")
        val link = annotated.getLinkAnnotations(0, annotated.length).single()
        assertEquals("https://example.com", annotated.text.substring(link.start, link.end))
    }

    @Test
    fun `unsafe links render as literal text with no link annotation`() {
        for (source in listOf(
            "[x](javascript:alert(1))",
            "[x](data:text/html,hi)",
            "[x](/relative)",
            "[x](//host/p)"
        )) {
            val annotated = render(source)
            assertEquals(source, annotated.text)
            assertTrue(annotated.getLinkAnnotations(0, annotated.length).isEmpty())
        }
    }
}
