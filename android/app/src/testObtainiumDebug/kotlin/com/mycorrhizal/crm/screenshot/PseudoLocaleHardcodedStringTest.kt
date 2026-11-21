package com.mycorrhizal.crm.screenshot

import android.app.Application
import androidx.compose.runtime.Composable
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.junit4.v2.createComposeRule
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issue #2535: the hardcoded-string half of the pseudo-locale pass.
 *
 * Android's en-XA pseudo-locale wraps every resource string in `[`…`]` (and
 * lengthens/accentuates it). So under `en-XA`, any visible text that is *not*
 * bracketed bypassed `stringResource()` — unless it is user-entered fixture
 * data, which is listed narrowly in [PseudoLocaleFixtures]. The assertion names
 * the offending string so a failure is actionable.
 *
 * This lives in `:app`'s `testObtainiumDebug` variant source set for the same
 * reason as [PseudoLocaleScreenshotTest]: only the app's link step generates
 * `en-rXA`; in a library module the qualifier would resolve to plain English
 * and the check would pass without exercising anything. Obtainium is the
 * superset flavor and the resources are debug-only, so it runs there alone.
 */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(sdk = [35], application = Application::class, qualifiers = PSEUDO_LOCALE_QUALIFIERS)
class PseudoLocaleHardcodedStringTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun loginHasNoHardcodedStrings() = assertOnlyLocalizedText(
        screen = "Login",
        userContent = PseudoLocaleFixtures.LOGIN_USER_CONTENT
    ) { PseudoLoginScreen() }

    @Test
    fun contactListHasNoHardcodedStrings() = assertOnlyLocalizedText(
        screen = "ContactList",
        userContent = PseudoLocaleFixtures.CONTACT_LIST_USER_CONTENT
    ) { PseudoContactListScreen() }

    @Test
    fun contactDetailHasNoHardcodedStrings() = assertOnlyLocalizedText(
        screen = "ContactDetail",
        userContent = PseudoLocaleFixtures.CONTACT_DETAIL_USER_CONTENT
    ) { PseudoContactDetailScreen() }

    @Test
    fun settingsHaveNoHardcodedStrings() = assertOnlyLocalizedText(
        screen = "Settings",
        userContent = PseudoLocaleFixtures.SETTINGS_USER_CONTENT
    ) { PseudoSettingsScreen() }

    /**
     * Walks the unmerged semantics tree and asserts every [SemanticsProperties.Text]
     * value is either en-XA-bracketed or one of [userContent]'s user-data entries.
     */
    private fun assertOnlyLocalizedText(
        screen: String,
        userContent: Set<String>,
        content: @Composable () -> Unit
    ) {
        composeTestRule.setContent { content() }
        val visibleTexts = composeTestRule.onAllNodes(
            SemanticsMatcher("has visible Text semantics") { node ->
                node.config.getOrNull(SemanticsProperties.Text)?.isNotEmpty() == true
            },
            useUnmergedTree = true
        ).fetchSemanticsNodes()
            .flatMap { it.config.getOrNull(SemanticsProperties.Text).orEmpty() }
            .map { it.text.trim() }
            .filter { it.isNotEmpty() }
            .distinct()

        val offenders = visibleTexts.filter { text ->
            !text.isPseudoLocalized() && text !in userContent
        }

        // Guard against a silent no-op: an empty/unreadable tree, or an en-XA
        // resource table that fell back to English, would otherwise pass.
        assertTrue(
            "No Text semantics were found on $screen; the sweep did not render " +
                "anything (or read the wrong tree).",
            visibleTexts.isNotEmpty()
        )
        assertTrue(
            "No en-XA bracketed text was found on $screen; the pseudo locale did " +
                "not resolve (is isPseudoLocalesEnabled still true on :app debug?).",
            visibleTexts.any { it.isPseudoLocalized() }
        )

        assertTrue(
            "Hardcoded string(s) rendered under en-XA on $screen: " +
                offenders.joinToString { "'$it'" } +
                ". Move UI copy to a string resource in " +
                "android/core/ui/src/main/res/values*/strings.xml; only fixture " +
                "user content may be allowlisted in PseudoLocaleFixtures.",
            offenders.isEmpty()
        )
    }

    private fun String.isPseudoLocalized(): Boolean = startsWith("[") && endsWith("]")
}
