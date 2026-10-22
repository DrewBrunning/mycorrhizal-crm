package com.mycorrhizal.crm

import android.app.Application
import android.content.Context
import android.content.res.Configuration
import androidx.compose.material3.DrawerValue
import androidx.compose.material3.rememberDrawerState
import androidx.compose.material3.windowsizeclass.ExperimentalMaterial3WindowSizeClassApi
import androidx.compose.material3.windowsizeclass.WindowSizeClass
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.DpSize
import androidx.compose.ui.unit.dp
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import java.util.Locale
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issue #1776: the rail has a fixed 96dp column, so every primary label and
 * "More" must fit at font scale 1.0 in all five shipped locales -- the audit saw
 * "Data suggest…" and "Import conta…" ellipsised. Rendered with a real locale
 * and [Density] fontScale 1.0, then each label's actual text layout is checked:
 * every line must be narrower than the slot and at most two lines, so nothing is
 * ellipsised or clipped -- not just that the string exists.
 */
@OptIn(ExperimentalMaterial3WindowSizeClassApi::class)
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class RailLabelTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val expanded = WindowSizeClass.calculateFromSize(DpSize(1000.dp, 800.dp))

    private fun localized(locale: Locale): Context {
        val base = ApplicationProvider.getApplicationContext<Application>()
        val config = Configuration(base.resources.configuration).apply { setLocale(locale) }
        return base.createConfigurationContext(config)
    }

    private fun setRail(locale: Locale) {
        val context = localized(locale)
        composeTestRule.setContent {
            CompositionLocalProvider(
                LocalContext provides context,
                LocalConfiguration provides context.resources.configuration,
                LocalDensity provides Density(density = 1f, fontScale = 1f),
            ) {
                MycorrhizalTheme {
                    MainNavScaffold(
                        currentRoute = "home",
                        windowSizeClass = expanded,
                        drawerState = rememberDrawerState(initialValue = DrawerValue.Closed),
                        onDestinationClick = {},
                    ) {}
                }
            }
        }
    }

    private fun assertLabelFits(locale: Locale) {
        setRail(locale)
        val context = localized(locale)
        val labels = railDestinations.map { it.route to context.getString(it.labelRes) } +
            ("more" to context.getString(R.string.nav_more))
        labels.forEach { (route, label) ->
            val matches = composeTestRule.onAllNodes(
                hasText(label) and hasAnyAncestor(hasTestTag("rail-$route")),
                useUnmergedTree = true,
            ).fetchSemanticsNodes()
            assertTrue("expected one \"$label\" in rail-$route [$locale], got ${matches.size}", matches.size == 1)
            val node = matches.first()
            val results = mutableListOf<androidx.compose.ui.text.TextLayoutResult>()
            val action = if (node.config.contains(SemanticsActions.GetTextLayoutResult)) {
                node.config[SemanticsActions.GetTextLayoutResult]
            } else {
                null
            }
            action?.action?.invoke(results)
            assertTrue("no text layout for \"$label\" [$locale]", results.isNotEmpty())
            val layout = results.first()
            val maxWidth = layout.layoutInput.constraints.maxWidth
            val lineWidths = (0 until layout.lineCount).map { layout.multiParagraph.getLineWidth(it) }
            // hasVisualOverflow is not usable here: Compose rounds size.width up to
            // an Int and then compares it to the fractional paragraph width, so a
            // 57.93px label is reported as overflowing a 96px slot. Assert the
            // actual line widths instead: every line must fit inside the slot.
            assertFalse(
                "\"$label\" [$locale] did not fit its rail slot at font scale 1.0: " +
                    "lineWidths=$lineWidths, maxWidth=$maxWidth",
                lineWidths.any { it >= maxWidth },
            )
            assertTrue("\"$label\" [$locale] wrapped past two lines", layout.lineCount in 1..2)
            assertFalse("\"$label\" [$locale] overflowed vertically", layout.didOverflowHeight)
        }
    }

    @Test
    fun `english rail labels fit at font scale one`() = assertLabelFits(Locale.ENGLISH)

    @Test
    fun `german rail labels fit at font scale one`() = assertLabelFits(Locale.GERMAN)

    @Test
    fun `spanish rail labels fit at font scale one`() = assertLabelFits(Locale("es"))

    @Test
    fun `french rail labels fit at font scale one`() = assertLabelFits(Locale.FRENCH)

    @Test
    fun `italian rail labels fit at font scale one`() = assertLabelFits(Locale.ITALIAN)

    @Test
    fun `the localized More label is not English for the non-English locales`() {
        assertNotEquals("More", localized(Locale.GERMAN).getString(R.string.nav_more))
        assertNotEquals("More", localized(Locale("es")).getString(R.string.nav_more))
        assertNotEquals("More", localized(Locale.FRENCH).getString(R.string.nav_more))
        assertNotEquals("More", localized(Locale.ITALIAN).getString(R.string.nav_more))
    }

    @Test
    fun `the rail renders the localized primary destinations`() {
        val locale = Locale.FRENCH
        setRail(locale)
        val context = localized(locale)

        railDestinations.forEach { destination ->
            val label = context.getString(destination.labelRes)
            val matches = composeTestRule.onAllNodes(
                hasText(label) and hasAnyAncestor(hasTestTag("rail-${destination.route}")),
                useUnmergedTree = true,
            ).fetchSemanticsNodes()
            assertTrue("missing localized ${destination.route} label ($label)", matches.size == 1)
        }
        val more = context.getString(R.string.nav_more)
        assertTrue(
            "missing localized More label ($more)",
            composeTestRule.onAllNodes(
                hasText(more) and hasAnyAncestor(hasTestTag("rail-more")),
                useUnmergedTree = true,
            ).fetchSemanticsNodes().size == 1,
        )
    }
}
