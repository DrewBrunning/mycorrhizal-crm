package com.mycorrhizal.crm.ui.theme

import android.content.Context
import androidx.compose.ui.graphics.Color
import androidx.test.core.app.ApplicationProvider
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertSame
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ThemeTest {

    @Test
    fun `brand fonts are bundled as font resources`() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        // The three bundled brand-font files must resolve as raw resources; a
        // missing font file would resolve to 0 (and font rendering would fall
        // back to the system font). T99 removed a fourth, eb_garamond.
        assertEquals(0, context.resources.getIdentifier("font/eb_garamond", "raw", context.packageName))
        assertNotEquals(0, context.resources.getIdentifier("font/ibm_plex_sans", "raw", context.packageName))
        assertNotEquals(0, context.resources.getIdentifier("font/ibm_plex_mono_regular", "raw", context.packageName))
        assertNotEquals(0, context.resources.getIdentifier("font/ibm_plex_mono_medium", "raw", context.packageName))
    }

    @Test
    fun `color palette carries the web tokens exactly`() {
        assertEquals(Color(0xFF3E543E), MycorrhizalColors.mycelium)  // primary
        assertEquals(Color(0xFFFAF5EA), MycorrhizalColors.bone)      // surface
        assertEquals(Color(0xFF30271F), MycorrhizalColors.bark)      // onSurface
        assertEquals(Color(0xFFAD5349), MycorrhizalColors.russula)   // error
        assertEquals(Color(0xFF0D844C), MycorrhizalColors.moss)      // tertiary/success
    }

    @Test
    fun `dark palette carries the web tokens exactly`() {
        assertEquals(Color(0xFF9EB698), MycorrhizalColors.myceliumDark)
        assertEquals(Color(0xFF1E1A13), MycorrhizalColors.boneDark)
        assertEquals(Color(0xFFEAE4DA), MycorrhizalColors.barkDark)
    }

    // T99 dropped EB Garamond from Android, so titleLarge -- T63's one safe
    // brand-serif role -- is sans like everything else. The assertion is kept
    // rather than deleted: it is what stops a serif drifting back onto a role
    // that has no serif to carry.
    @Test
    fun `titleLarge is sans since T99 dropped the brand serif`() {
        assertSame(MycorrhizalFonts.sans, MycorrhizalTypography.titleLarge.fontFamily)
    }

    // headlineSmall must stay sans specifically because M3's AlertDialog
    // defaults its title slot to headlineSmall -- serif there was a real,
    // shipping bug (every dialog title rendered in Garamond) this test pins
    // against regressing, and would pin again if a brand face returned.
    @Test
    fun `headlineSmall stays sans so AlertDialog titles don't inherit serif`() {
        assertSame(MycorrhizalFonts.sans, MycorrhizalTypography.headlineSmall.fontFamily)
    }

    @Test
    fun `display and headline roles stay sans`() {
        assertSame(MycorrhizalFonts.sans, MycorrhizalTypography.displayLarge.fontFamily)
        assertSame(MycorrhizalFonts.sans, MycorrhizalTypography.displayMedium.fontFamily)
        assertSame(MycorrhizalFonts.sans, MycorrhizalTypography.displaySmall.fontFamily)
        assertSame(MycorrhizalFonts.sans, MycorrhizalTypography.headlineLarge.fontFamily)
        assertSame(MycorrhizalFonts.sans, MycorrhizalTypography.headlineMedium.fontFamily)
    }

    // labelLarge is also M3's default Button-label and Snackbar-action-label
    // font -- it must never carry a brand font or a size override globally.
    // The nav drawer's own 16sp bump (T100) is applied per-call-site in
    // MycorrhizalApp.kt for exactly this reason.
    @Test
    fun `labelLarge stays sans`() {
        assertSame(MycorrhizalFonts.sans, MycorrhizalTypography.labelLarge.fontFamily)
    }
}
