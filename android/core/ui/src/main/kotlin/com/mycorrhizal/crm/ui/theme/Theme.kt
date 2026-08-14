package com.mycorrhizal.crm.ui.theme

import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Shapes
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.mycorrhizal.crm.ui.R

/**
 * The web app's design tokens mapped to Material 3 color roles (ticket §4.4).
 * M3's tonal palette generation is bypassed — every color role is hand-pinned
 * to the exact hex values from the web theme.
 */
object MycorrhizalColors {
    // Light
    val bone = Color(0xFFFAF5EA)          // surface / background
    val parchment = Color(0xFFEFE7D9)     // surfaceVariant
    val paper = Color(0xFFFFFFFF)         // surfaceContainerHighest (dialogs)
    val mycelium = Color(0xFF3E543E)      // primary
    val lichen = Color(0xFF97A390)        // secondary
    val bark = Color(0xFF30271F)          // onSurface / onBackground
    val soil = Color(0xFF595148)          // onSurfaceVariant
    val moss = Color(0xFF0D844C)          // tertiary / success
    val chanterelle = Color(0xFFD3A563)   // warning
    val russula = Color(0xFFAD5349)       // error
    val laccaria = Color(0xFF7B6B98)      // info

    // Dark
    val boneDark = Color(0xFF1E1A13)
    val parchmentDark = Color(0xFF2B261B)
    val paperDark = Color(0xFF393226)
    val myceliumDark = Color(0xFF9EB698)
    val lichenDark = Color(0xFF9BAA94)
    val barkDark = Color(0xFFEAE4DA)
    val soilDark = Color(0xFFB5ADA2)
    val mossDark = Color(0xFF349D62)
    val chanterelleDark = Color(0xFFDDAE6C)
    val russulaDark = Color(0xFFC4675D)
    val laccariaDark = Color(0xFF9F8FBE)
}

private val LightColors = lightColorScheme(
    primary = MycorrhizalColors.mycelium,
    onPrimary = MycorrhizalColors.bone,
    primaryContainer = MycorrhizalColors.parchment,
    onPrimaryContainer = MycorrhizalColors.bark,
    secondary = MycorrhizalColors.lichen,
    onSecondary = MycorrhizalColors.bark,
    secondaryContainer = MycorrhizalColors.parchment,
    onSecondaryContainer = MycorrhizalColors.bark,
    tertiary = MycorrhizalColors.moss,
    background = MycorrhizalColors.bone,
    onBackground = MycorrhizalColors.bark,
    surface = MycorrhizalColors.bone,
    onSurface = MycorrhizalColors.bark,
    surfaceVariant = MycorrhizalColors.parchment,
    onSurfaceVariant = MycorrhizalColors.soil,
    // Cards render parchment (the web's background.paper), matching the 3-tier
    // bone/parchment/paper layering: bone page, parchment cards, paper dialogs.
    surfaceContainerHighest = MycorrhizalColors.parchment,
    surfaceContainerHigh = MycorrhizalColors.paper,
    surfaceContainer = MycorrhizalColors.paper,
    surfaceContainerLow = MycorrhizalColors.parchment,
    error = MycorrhizalColors.russula,
    onError = Color.White,
    outline = MycorrhizalColors.soil,
)

private val DarkColors = darkColorScheme(
    primary = MycorrhizalColors.myceliumDark,
    onPrimary = MycorrhizalColors.boneDark,
    primaryContainer = MycorrhizalColors.parchmentDark,
    onPrimaryContainer = MycorrhizalColors.barkDark,
    secondary = MycorrhizalColors.lichenDark,
    onSecondary = MycorrhizalColors.boneDark,
    secondaryContainer = MycorrhizalColors.parchmentDark,
    onSecondaryContainer = MycorrhizalColors.barkDark,
    tertiary = MycorrhizalColors.mossDark,
    background = MycorrhizalColors.boneDark,
    onBackground = MycorrhizalColors.barkDark,
    surface = MycorrhizalColors.boneDark,
    onSurface = MycorrhizalColors.barkDark,
    surfaceVariant = MycorrhizalColors.parchmentDark,
    onSurfaceVariant = MycorrhizalColors.soilDark,
    surfaceContainerHighest = MycorrhizalColors.parchmentDark,
    surfaceContainerHigh = MycorrhizalColors.paperDark,
    surfaceContainer = MycorrhizalColors.paperDark,
    surfaceContainerLow = MycorrhizalColors.parchmentDark,
    error = MycorrhizalColors.russulaDark,
    onError = MycorrhizalColors.boneDark,
    outline = MycorrhizalColors.soilDark,
)

/**
 * The bundled brand fonts:
 *  - IBM Plex Sans for UI and content.
 *  - IBM Plex Mono for data fields (phone numbers, emails, IDs).
 *
 * **No serif.** T99 dropped EB Garamond from Android entirely: a display serif
 * that reads well in a 1440px page header carries very differently at 22sp
 * over Material 3's metrics on a phone. This is a deliberate divergence from
 * web, which keeps Garamond on `typography.h5` and the persistent nav per T63
 * — not a parity gap to be re-filed.
 *
 * IBM Plex Sans is a variable font (weight axis); each `Font` entry pins a
 * weight so Compose picks the right instance. See THIRD_PARTY_SOURCES.md for
 * the SIL OFL notices.
 */
object MycorrhizalFonts {
    val sans = FontFamily(
        Font(R.font.ibm_plex_sans, weight = FontWeight.Normal),
        Font(R.font.ibm_plex_sans, weight = FontWeight.Medium),
        Font(R.font.ibm_plex_sans, weight = FontWeight.SemiBold),
    )
    val mono = FontFamily(
        Font(R.font.ibm_plex_mono_regular, weight = FontWeight.Normal),
        Font(R.font.ibm_plex_mono_medium, weight = FontWeight.Medium),
    )
}

/**
 * Full Material 3 type scale over the brand fonts. Overriding the whole scale
 * (not just titleLarge/bodyLarge) keeps every M3 role — labels, body, buttons,
 * chips — on IBM Plex Sans instead of the default Roboto.
 *
 * Every role is IBM Plex Sans as of T99, which dropped EB Garamond from
 * Android — see MycorrhizalFonts above for why, and why that divergence from
 * web's T63 is deliberate.
 *
 * Kept for the next person who reaches for a display face here: T63's Android
 * port put the serif on `titleLarge` only, because that role is used
 * exclusively for TopAppBar titles and the contact-name heading, and M3's
 * `AlertDialog` title defaults to `headlineSmall` rather than `titleLarge`. An
 * earlier revision had the serif on `display*` and `headlineLarge/Medium/Small`
 * too, which put it on every AlertDialog title in the app. If a brand face ever
 * comes back, `titleLarge` is the one role it can safely occupy.
 */
val MycorrhizalTypography = Typography(
    // See titleLarge below for the brand-serif role -- display*/headline*
    // stay sans so AlertDialog titles (which default to headlineSmall)
    // don't inherit Garamond.
    displayLarge = TextStyle(fontFamily = MycorrhizalFonts.sans, fontSize = 57.sp, fontWeight = FontWeight.Normal),
    displayMedium = TextStyle(fontFamily = MycorrhizalFonts.sans, fontSize = 45.sp, fontWeight = FontWeight.Normal),
    displaySmall = TextStyle(fontFamily = MycorrhizalFonts.sans, fontSize = 36.sp, fontWeight = FontWeight.Normal),
    headlineLarge = TextStyle(fontFamily = MycorrhizalFonts.sans, fontSize = 32.sp, fontWeight = FontWeight.Normal),
    headlineMedium = TextStyle(fontFamily = MycorrhizalFonts.sans, fontSize = 28.sp, fontWeight = FontWeight.Normal),
    headlineSmall = TextStyle(fontFamily = MycorrhizalFonts.sans, fontSize = 24.sp, fontWeight = FontWeight.Normal),
    // App bars and page titles. Carried EB Garamond as T63's one safe
    // brand-serif role until T99 dropped the serif from Android; every role in
    // this scale is now sans.
    titleLarge = TextStyle(fontFamily = MycorrhizalFonts.sans, fontSize = 22.sp, fontWeight = FontWeight.Medium),
    titleMedium = TextStyle(fontFamily = MycorrhizalFonts.sans, fontSize = 16.sp, fontWeight = FontWeight.Medium),
    titleSmall = TextStyle(fontFamily = MycorrhizalFonts.sans, fontSize = 14.sp, fontWeight = FontWeight.Medium),
    // Body — IBM Plex Sans.
    bodyLarge = TextStyle(fontFamily = MycorrhizalFonts.sans, fontSize = 16.sp, fontWeight = FontWeight.Normal, lineHeight = 24.sp),
    bodyMedium = TextStyle(fontFamily = MycorrhizalFonts.sans, fontSize = 14.sp, fontWeight = FontWeight.Normal, lineHeight = 20.sp),
    bodySmall = TextStyle(fontFamily = MycorrhizalFonts.sans, fontSize = 12.sp, fontWeight = FontWeight.Normal, lineHeight = 16.sp),
    // Labels / buttons — IBM Plex Sans medium.
    labelLarge = TextStyle(fontFamily = MycorrhizalFonts.sans, fontSize = 14.sp, fontWeight = FontWeight.Medium, lineHeight = 20.sp),
    labelMedium = TextStyle(fontFamily = MycorrhizalFonts.sans, fontSize = 12.sp, fontWeight = FontWeight.Medium, lineHeight = 16.sp),
    labelSmall = TextStyle(fontFamily = MycorrhizalFonts.sans, fontSize = 11.sp, fontWeight = FontWeight.Medium, lineHeight = 16.sp),
)

/**
 * Named convenience styles for call sites that want to read their intent
 * rather than guess an M3 slot. `appBarTitle` was removed (T63 Android port)
 * once its one remaining consumer (`SettingsScreen`) migrated to
 * `MaterialTheme.typography.titleLarge`, the standard pattern every other
 * screen already used — prefer the M3 roles for new code.
 */
object AppTypography {
    /** Data fields (phones, emails, IDs) — IBM Plex Mono. */
    val mono = TextStyle(fontFamily = MycorrhizalFonts.mono, fontSize = 14.sp, fontWeight = FontWeight.Normal)
}

private val MycorrhizalShapes = Shapes(
    small = RoundedCornerShape(10.dp),
    medium = RoundedCornerShape(10.dp),
    large = RoundedCornerShape(10.dp),
    // NOT CircleShape: M3's AlertDialog also defaults its container to
    // shapes.extraLarge, so overriding it here made every AlertDialog in the
    // app render clipped to a circle/oval (title and buttons cut off) --
    // found during T97/T106 on-device verification. The brand FAB doesn't
    // need this token: BrandFab.kt already sets shape = CircleShape on
    // itself explicitly, so this can just be the same rounded shape as the
    // rest of the scale.
    extraLarge = RoundedCornerShape(10.dp),
)

@Composable
fun MycorrhizalTheme(
    darkTheme: Boolean = false,
    content: @Composable () -> Unit,
) {
    MaterialTheme(
        colorScheme = if (darkTheme) DarkColors else LightColors,
        shapes = MycorrhizalShapes,
        typography = MycorrhizalTypography,
        content = content,
    )
}
