package com.mycorrhizal.crm.ui.theme

import android.content.Context
import androidx.compose.ui.graphics.Color
import androidx.test.core.app.ApplicationProvider
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
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
        // The four bundled brand-font files must resolve as raw resources; a
        // missing font file would resolve to 0 (and font rendering would fall
        // back to the system font).
        assertNotEquals(0, context.resources.getIdentifier("font/eb_garamond", "raw", context.packageName))
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
}
