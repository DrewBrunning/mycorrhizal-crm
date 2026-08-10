package com.mycorrhizal.crm.ui.theme

import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Shapes
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

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
    surfaceContainerHighest = MycorrhizalColors.paper,
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
    surfaceContainerHighest = MycorrhizalColors.paperDark,
    error = MycorrhizalColors.russulaDark,
    onError = MycorrhizalColors.boneDark,
    outline = MycorrhizalColors.soilDark,
)

/**
 * Typography: AppBar title in a serif (EB Garamond is the target asset; the
 * system serif is the placeholder until the font files are bundled), body in
 * the system sans, monospace for data fields.
 */
object MycorrhizalTypography {
    val appBarTitle = TextStyle(
        fontFamily = FontFamily.Serif,
        fontSize = 22.sp,
        fontWeight = FontWeight.Normal,
    )
    val body = TextStyle(
        fontFamily = FontFamily.SansSerif,
        fontSize = 16.sp,
    )
    val mono = TextStyle(
        fontFamily = FontFamily.Monospace,
        fontSize = 14.sp,
    )
}

private val MycorrhizalShapes = Shapes(
    small = RoundedCornerShape(10.dp),
    medium = RoundedCornerShape(10.dp),
    large = RoundedCornerShape(10.dp),
)

@Composable
fun MycorrhizalTheme(
    darkTheme: Boolean = false,
    content: @Composable () -> Unit,
) {
    MaterialTheme(
        colorScheme = if (darkTheme) DarkColors else LightColors,
        shapes = MycorrhizalShapes,
        typography = androidx.compose.material3.Typography(
            titleLarge = MycorrhizalTypography.appBarTitle,
            bodyLarge = MycorrhizalTypography.body,
        ),
        content = content,
    )
}
