package com.mycorrhizal.crm

import java.io.File
import javax.xml.parsers.DocumentBuilderFactory
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.w3c.dom.Element

/**
 * Issue #1920: API 26 cannot render dark navigation-bar icons on the bone bar
 * (android:windowLightNavigationBar is API 27+) and the splash window did not
 * publish a status-bar color, so it fell back to the platform's pink. The
 * runtime policy and the resource fallbacks are both pinned here — the policy
 * because SystemBarsEffect would otherwise repaint the bone bar at runtime,
 * and the resources because the pre-Compose launch/splash window never reaches
 * that code.
 */
class Api26SystemBarsTest {

    private fun styleItems(file: String, styleName: String): Map<String, String> {
        val doc = DocumentBuilderFactory.newInstance()
            .newDocumentBuilder()
            .parse(File("src/main/res/$file"))
        val styles = doc.getElementsByTagName("style")
        for (i in 0 until styles.length) {
            val style = styles.item(i) as Element
            if (style.getAttribute("name") != styleName) continue
            val items = style.getElementsByTagName("item")
            return (0 until items.length).associate { index ->
                val item = items.item(index) as Element
                item.getAttribute("name") to item.textContent.trim()
            }
        }
        error("Style '$styleName' not found in $file")
    }

    // --- Runtime policy -----------------------------------------------------------------------

    @Test
    fun `API 26 uses light navigation icons in both themes`() {
        assertFalse(navigationBarLightAppearance(darkTheme = false, sdkInt = 26))
        assertFalse(navigationBarLightAppearance(darkTheme = true, sdkInt = 26))
    }

    @Test
    fun `API 27 and later use dark icons only on the light bone bar`() {
        assertTrue(navigationBarLightAppearance(darkTheme = false, sdkInt = 27))
        assertFalse(navigationBarLightAppearance(darkTheme = true, sdkInt = 27))
        assertTrue(navigationBarLightAppearance(darkTheme = false, sdkInt = 35))
    }

    @Test
    fun `API 26 light theme paints the dark primary bar`() {
        val surface = 0x222222
        val primary = 0x111111
        assertEquals(primary, navigationBarBackground(false, 26, surface, primary))
        assertEquals(surface, navigationBarBackground(false, 27, surface, primary))
        // The night bar is already dark, so it keeps the surface color.
        assertEquals(surface, navigationBarBackground(true, 26, surface, primary))
    }

    // --- Resource fallbacks (the launch and splash windows) -----------------------------------

    @Test
    fun `the API 26 base theme uses a dark navigation bar`() {
        val base = styleItems("values/themes.xml", "Base.Theme.Mycorrhizal")
        assertEquals("#3E543E", base["android:navigationBarColor"])
        assertFalse(base.containsKey("android:windowLightNavigationBar"))
    }

    @Test
    fun `the API 27 override restores the bone bar with dark icons`() {
        val v27 = styleItems("values-v27/themes.xml", "Theme.Mycorrhizal")
        assertEquals("#FAF5EA", v27["android:navigationBarColor"])
        assertEquals("true", v27["android:windowLightNavigationBar"])
    }

    @Test
    fun `the night navigation bar is already dark on API 26`() {
        val night = styleItems("values-night/themes.xml", "Base.Theme.Mycorrhizal")
        assertEquals("#1E1A13", night["android:navigationBarColor"])
    }

    @Test
    fun `the light splash status bar matches the splash background`() {
        val splash = styleItems("values/themes.xml", "Theme.Mycorrhizal.Splash")
        assertEquals("#FAF5EA", splash["android:statusBarColor"])
        assertEquals("true", splash["android:windowLightStatusBar"])
    }

    @Test
    fun `the night splash status bar matches the dark splash background`() {
        val splash = styleItems("values-night/themes.xml", "Theme.Mycorrhizal.Splash")
        assertEquals("#1E1A13", splash["android:statusBarColor"])
        assertEquals("false", splash["android:windowLightStatusBar"])
    }
}
