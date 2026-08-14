package com.mycorrhizal.crm

import android.content.res.Configuration
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.SystemBarStyle
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.runtime.getValue
import androidx.core.splashscreen.SplashScreen.Companion.installSplashScreen
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.ui.theme.MycorrhizalColors
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import dagger.hilt.android.AndroidEntryPoint

private fun androidx.compose.ui.graphics.Color.toArgbCompat(): Int =
    android.graphics.Color.argb(
        (alpha * 255).toInt(),
        (red * 255).toInt(),
        (green * 255).toInt(),
        (blue * 255).toInt(),
    )

@AndroidEntryPoint
class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        // T106: must precede super.onCreate() -- required by the library, not stylistic.
        installSplashScreen()
        super.onCreate(savedInstanceState)

        // T97: isSystemInDarkTheme() isn't callable yet here (not in composition), so this
        // reads Configuration directly, once, purely to pick the correct scrims below so the
        // very first frame is right before any composable LaunchedEffect runs. The reactive
        // isSystemInDarkTheme() read in setContent below is the source of truth for
        // everything after that; this is an accepted, one-time duplication.
        val darkThemeAtLaunch = (resources.configuration.uiMode and Configuration.UI_MODE_NIGHT_MASK) ==
            Configuration.UI_MODE_NIGHT_YES

        enableEdgeToEdge(
            statusBarStyle = if (darkThemeAtLaunch) {
                // myceliumDark is light-toned (M3 dark-scheme accents are lighter than their
                // light-scheme counterpart) -> dark icons. SystemBarStyle.light always forces
                // dark icons regardless of system dark mode, so the second (darkScrim) param
                // is unused here; passed the same color for clarity.
                SystemBarStyle.light(
                    MycorrhizalColors.myceliumDark.toArgbCompat(),
                    MycorrhizalColors.myceliumDark.toArgbCompat(),
                )
            } else {
                SystemBarStyle.dark(MycorrhizalColors.mycelium.toArgbCompat())
            },
            navigationBarStyle = if (darkThemeAtLaunch) {
                SystemBarStyle.dark(MycorrhizalColors.boneDark.toArgbCompat())
            } else {
                SystemBarStyle.light(
                    MycorrhizalColors.bone.toArgbCompat(),
                    MycorrhizalColors.bone.toArgbCompat(),
                )
            },
        )

        setContent {
            val darkTheme = isSystemInDarkTheme()
            MycorrhizalTheme(darkTheme = darkTheme) {
                MycorrhizalApp(darkTheme = darkTheme)
            }
        }
    }
}
